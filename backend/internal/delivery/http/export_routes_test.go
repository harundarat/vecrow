package http_test

import (
	"bytes"
	"encoding/csv"
	"math/big"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/harundarat/vecrow/backend/internal/domain"
	"github.com/harundarat/vecrow/backend/internal/usecase/escrow"
)

func TestExportCSV(t *testing.T) {
	srv := newTestServer(t, fakeIssuer{})
	at := time.Date(2026, 11, 13, 9, 30, 5, 0, time.FixedZone("WIB", 7*3600))
	srv.escrows.payouts = []escrow.Payout{
		{Time: at, EscrowID: testEscrowID, Milestones: []uint32{0}, Amount: big.NewInt(5_000_000),
			Recipient: "treasury.near", TxHash: "HASH1", Status: escrow.PayoutPaid},
		{Time: at, EscrowID: testEscrowID, Milestones: []uint32{0}, Amount: big.NewInt(696_500_000),
			Recipient: "alice.near", TxHash: "HASH1", Status: escrow.PayoutPaid},
		{Time: at, EscrowID: testEscrowID, Milestones: []uint32{1, 2}, Amount: big.NewInt(1),
			Recipient: "client.near", TxHash: "HASH2", Status: escrow.PayoutPaid},
		// Cells a spreadsheet would run as formulas, and a field to quote.
		{Time: at, EscrowID: "-escrow", Amount: big.NewInt(298_500_000),
			Recipient: `=HYPERLINK("http://evil.example","x, y")`, Status: escrow.PayoutPending},
		{EscrowID: testEscrowID, Amount: big.NewInt(0), Recipient: "@sum", TxHash: "+1"},
		{EscrowID: "\tx", Amount: big.NewInt(0), Recipient: "＝cmd", TxHash: "\nx"},
	}

	rec := srv.do(http.MethodGet, "/api/v1/escrows/"+testEscrowID+"/export.csv", "", "Authorization", bearer)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if srv.escrows.method != "Payouts" || srv.escrows.id != testEscrowID || srv.escrows.requester.AccountID != "acct-1" {
		t.Fatalf("call %s(%s, %+v)", srv.escrows.method, srv.escrows.id, srv.escrows.requester)
	}
	h := rec.Header()
	if ct := h.Get("Content-Type"); ct != "text/csv; charset=utf-8" {
		t.Errorf("Content-Type = %q", ct)
	}
	if cd := h.Get("Content-Disposition"); cd != `attachment; filename="vecrow-escrow-`+testEscrowID+`.csv"` {
		t.Errorf("Content-Disposition = %q", cd)
	}
	if cc := h.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q", cc)
	}

	body := rec.Body.Bytes()
	// RFC 4180: every line ends with CRLF, including the last one.
	if !bytes.HasSuffix(body, []byte("\r\n")) || bytes.Count(body, []byte("\n")) != bytes.Count(body, []byte("\r\n")) {
		t.Fatalf("line ends are not CRLF: %q", body)
	}
	r := csv.NewReader(bytes.NewReader(body))
	r.FieldsPerRecord = 7
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("not valid CSV: %v\n%s", err, body)
	}
	want := [][]string{
		{"date", "escrow", "milestone", "USDC amount", "recipient", "tx hash", "status"},
		{"2026-11-13 02:30:05", testEscrowID, "1", "5.000000", "treasury.near", "HASH1", "paid"},
		{"2026-11-13 02:30:05", testEscrowID, "1", "696.500000", "alice.near", "HASH1", "paid"},
		{"2026-11-13 02:30:05", testEscrowID, "2, 3", "0.000001", "client.near", "HASH2", "paid"},
		{"2026-11-13 02:30:05", "'-escrow", "", "298.500000", `'=HYPERLINK("http://evil.example","x, y")`, "", "pending"},
		{"", testEscrowID, "", "0.000000", "'@sum", "'+1", ""},
		{"", "'\tx", "", "0.000000", "'＝cmd", "'\nx", ""},
	}
	if len(records) != len(want) {
		t.Fatalf("records = %q", records)
	}
	for i := range want {
		if !slices.Equal(records[i], want[i]) {
			t.Errorf("record %d = %q, want %q", i, records[i], want[i])
		}
	}
	if !strings.Contains(string(body), `"'=HYPERLINK(""http://evil.example"",""x, y"")"`) {
		t.Errorf("quoted field not escaped per RFC 4180: %s", body)
	}

	// A guest token is passed on as the requester.
	srv.do(http.MethodGet, "/api/v1/escrows/"+testEscrowID+"/export.csv", "", "X-Guest-Token", validGuestToken)
	if g := srv.escrows.requester.Guest; g == nil || *g != testGuest || srv.escrows.requester.AccountID != "" {
		t.Errorf("guest requester = %+v", srv.escrows.requester)
	}
}

func TestExportCSV_HeaderOnlyAndErrors(t *testing.T) {
	srv := newTestServer(t, fakeIssuer{})
	rec := srv.do(http.MethodGet, "/api/v1/escrows/"+testEscrowID+"/export.csv", "", "Authorization", bearer)
	if rec.Code != http.StatusOK || rec.Body.String() != "date,escrow,milestone,USDC amount,recipient,tx hash,status\r\n" {
		t.Fatalf("no payouts: status %d %q", rec.Code, rec.Body)
	}

	for _, c := range []struct {
		err    error
		status int
	}{
		{domain.Unauthorized("a session or a guest token for this escrow is required"), http.StatusUnauthorized},
		{domain.Forbidden("only the parties of the escrow can export its payouts"), http.StatusForbidden},
		{domain.NotFound("escrow not found"), http.StatusNotFound},
	} {
		srv.escrows.err = c.err
		rec := srv.do(http.MethodGet, "/api/v1/escrows/"+testEscrowID+"/export.csv", "")
		if rec.Code != c.status || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
			t.Errorf("%v: status %d, Content-Type %q", c.err, rec.Code, rec.Header().Get("Content-Type"))
		}
	}
	srv.escrows.err = nil

	if rec := srv.do(http.MethodGet, "/api/v1/escrows/"+testEscrowID+"/export.csv", "", "X-Guest-Token", "expired"); rec.Code != http.StatusUnauthorized {
		t.Errorf("invalid guest token: status %d", rec.Code)
	}
	if rec := srv.do(http.MethodGet, "/api/v1/escrows/"+testEscrowID+"/export.csv", "", "Authorization", recoveryBearer); rec.Code != http.StatusForbidden {
		t.Errorf("recovery session: status %d", rec.Code)
	}
}
