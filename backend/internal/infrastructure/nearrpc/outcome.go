package nearrpc

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// Outcome is a transaction's final execution outcome as send_tx and tx
// return it.
type Outcome struct {
	Transaction struct {
		Hash       string `json:"hash"`
		SignerID   string `json:"signer_id"`
		ReceiverID string `json:"receiver_id"`
	} `json:"transaction"`
	TransactionOutcome ExecutionOutcome   `json:"transaction_outcome"`
	ReceiptsOutcome    []ExecutionOutcome `json:"receipts_outcome"`
}

// ExecutionOutcome is the outcome of the transaction or of one receipt.
type ExecutionOutcome struct {
	ID      string `json:"id"`
	Outcome struct {
		Logs       []string        `json:"logs"`
		ReceiptIDs []string        `json:"receipt_ids"`
		Status     ExecutionStatus `json:"status"`
		ExecutorID string          `json:"executor_id"`
	} `json:"outcome"`
}

// ExecutionStatus is one of SuccessValue (base64), SuccessReceiptId, Failure
// or a pending state.
type ExecutionStatus struct {
	SuccessValue     *string
	SuccessReceiptID string
	Failure          json.RawMessage
}

func (s *ExecutionStatus) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '"' {
		return nil // "Unknown" or "Pending": neither success nor failure yet
	}
	var v struct {
		SuccessValue     *string         `json:"SuccessValue"`
		SuccessReceiptID string          `json:"SuccessReceiptId"`
		Failure          json.RawMessage `json:"Failure"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*s = ExecutionStatus{SuccessValue: v.SuccessValue, SuccessReceiptID: v.SuccessReceiptID, Failure: v.Failure}
	return nil
}

func (s ExecutionStatus) failed() bool { return len(s.Failure) > 0 }

// Event is a NEP-297 event logged by a contract.
type Event struct {
	ReceiptID string
	// LogIndex is the position of the log in the receipt's logs.
	LogIndex int
	Standard string
	Version  string
	Event    string
	Data     json.RawMessage
}

// CallResult is what a function call on a contract did.
type CallResult struct {
	Hash      string
	Succeeded bool
	// Failure explains a failed call, e.g. the contract's panic message.
	Failure string
	// Value is the method's return value (JSON), after following the
	// promises it returned.
	Value []byte
	// Events are the NEP-297 events of standard logged by the contract in
	// receipts that succeeded.
	Events []Event
}

// eventPrefix starts every NEP-297 log.
const eventPrefix = "EVENT_JSON:"

// CallResult finds the receipt that ran the call on contractID, directly or
// through a delegate action, and reports its result and the events of
// standard the contract logged. A delegate transaction's own status says
// nothing about the inner call, so the receipts are inspected instead.
func (o *Outcome) CallResult(contractID, standard string) CallResult {
	res := CallResult{Hash: o.Transaction.Hash}
	receipts := make(map[string]*ExecutionOutcome, len(o.ReceiptsOutcome))
	for i := range o.ReceiptsOutcome {
		receipts[o.ReceiptsOutcome[i].ID] = &o.ReceiptsOutcome[i]
	}

	txo := o.TransactionOutcome.Outcome
	if txo.Status.failed() {
		res.Failure = FailureMessage(txo.Status.Failure)
		return res
	}
	if len(txo.ReceiptIDs) == 0 {
		res.Failure = "transaction produced no receipt"
		return res
	}
	r := receipts[txo.ReceiptIDs[0]]
	// Walk down through a delegate receipt to the receipt on the contract.
	for depth := 0; r != nil && r.Outcome.ExecutorID != contractID; depth++ {
		if r.Outcome.Status.failed() {
			res.Failure = FailureMessage(r.Outcome.Status.Failure)
			return res
		}
		var next *ExecutionOutcome
		for _, id := range r.Outcome.ReceiptIDs {
			if c := receipts[id]; c != nil && c.Outcome.ExecutorID == contractID {
				next = c
				break
			}
		}
		if depth > 4 {
			next = nil
		}
		r = next
	}
	if r == nil {
		res.Failure = "no receipt reached " + contractID
		return res
	}

	// Follow the promises the method returned to its final value.
	for depth := 0; ; depth++ {
		st := r.Outcome.Status
		switch {
		case st.failed():
			res.Failure = FailureMessage(st.Failure)
		case st.SuccessValue != nil:
			v, err := base64.StdEncoding.DecodeString(*st.SuccessValue)
			if err != nil {
				res.Failure = "undecodable return value"
				break
			}
			res.Succeeded, res.Value = true, v
		case st.SuccessReceiptID != "" && receipts[st.SuccessReceiptID] != nil && depth < 8:
			r = receipts[st.SuccessReceiptID]
			continue
		default:
			res.Failure = "call outcome is not final"
		}
		break
	}

	for _, ro := range o.ReceiptsOutcome {
		if ro.Outcome.ExecutorID != contractID || ro.Outcome.Status.failed() {
			continue
		}
		for i, line := range ro.Outcome.Logs {
			res.Events = append(res.Events, parseEvents(ro.ID, i, line, standard)...)
		}
	}
	return res
}

// parseEvents parses a NEP-297 log line. Data may be an object or an array.
func parseEvents(receiptID string, index int, line, standard string) []Event {
	body, ok := strings.CutPrefix(line, eventPrefix)
	if !ok {
		return nil
	}
	var ev struct {
		Standard string          `json:"standard"`
		Version  string          `json:"version"`
		Event    string          `json:"event"`
		Data     json.RawMessage `json:"data"`
	}
	if json.Unmarshal([]byte(body), &ev) != nil || ev.Standard != standard || ev.Event == "" {
		return nil
	}
	data := ev.Data
	if len(data) == 0 {
		data = json.RawMessage("null")
	}
	return []Event{{ReceiptID: receiptID, LogIndex: index, Standard: ev.Standard, Version: ev.Version, Event: ev.Event, Data: data}}
}

// FailureMessage turns a TxExecutionError into a short readable message: the
// panic message for contract panics, otherwise the error's JSON.
func FailureMessage(failure json.RawMessage) string {
	var v struct {
		ActionError *struct {
			Kind json.RawMessage `json:"kind"`
		} `json:"ActionError"`
	}
	if json.Unmarshal(failure, &v) == nil && v.ActionError != nil {
		var kind struct {
			FunctionCallError *struct {
				ExecutionError string `json:"ExecutionError"`
			} `json:"FunctionCallError"`
		}
		if json.Unmarshal(v.ActionError.Kind, &kind) == nil && kind.FunctionCallError != nil &&
			kind.FunctionCallError.ExecutionError != "" {
			return strings.TrimPrefix(kind.FunctionCallError.ExecutionError, "Smart contract panicked: ")
		}
		return compact(v.ActionError.Kind)
	}
	return compact(failure)
}

func compact(b json.RawMessage) string {
	var buf bytes.Buffer
	if json.Compact(&buf, b) != nil {
		return fmt.Sprintf("%.300s", b)
	}
	s := buf.String()
	if len(s) > 300 {
		s = s[:300] + "…"
	}
	return s
}
