package handler

import (
	"bytes"
	"encoding/csv"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/harundarat/vecrow/backend/internal/delivery/http/middleware"
	"github.com/harundarat/vecrow/backend/internal/delivery/http/response"
	"github.com/harundarat/vecrow/backend/internal/domain"
	"github.com/harundarat/vecrow/backend/internal/usecase/escrow"
)

// csvHeader is the header row of the CSV export (R-12).
var csvHeader = []string{"date", "escrow", "milestone", "USDC amount", "recipient", "tx hash", "status"}

// csvDateLayout is how dates are written, in UTC: a layout spreadsheets read
// as a date and time without cleanup.
const csvDateLayout = "2006-01-02 15:04:05"

// ExportCSV serves GET /escrows/:id/export.csv: the escrow's payouts, one row
// per payout, the platform fee as its own row with the treasury as
// recipient, pending payouts included (R-12, R-20). Only the parties of the
// escrow can download it.
func (h *EscrowHandler) ExportCSV(c *gin.Context) {
	id := c.Param("id")
	payouts, err := h.escrows.Payouts(c.Request.Context(), id, middleware.RequesterFrom(c.Request.Context()))
	if err != nil {
		response.Error(c, err)
		return
	}
	body, err := payoutsCSV(payouts)
	if err != nil {
		response.Error(c, err)
		return
	}
	c.Header("Content-Disposition", `attachment; filename="vecrow-escrow-`+id+`.csv"`)
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, "text/csv; charset=utf-8", body)
}

// payoutsCSV writes payouts as RFC 4180 CSV: CRLF line ends, fields quoted
// when needed, amounts as decimal USDC, and every cell guarded against
// formula injection.
func payoutsCSV(payouts []escrow.Payout) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	w.UseCRLF = true
	if err := w.Write(csvHeader); err != nil {
		return nil, err
	}
	for _, p := range payouts {
		date := ""
		if !p.Time.IsZero() {
			date = p.Time.UTC().Format(csvDateLayout)
		}
		record := []string{
			date, p.EscrowID, milestoneCell(p.Milestones), domain.FormatUSDC(p.Amount),
			p.Recipient, p.TxHash, p.Status,
		}
		for i, cell := range record {
			record[i] = csvSafe(cell)
		}
		if err := w.Write(record); err != nil {
			return nil, err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// milestoneCell numbers milestones from 1, as people count them; a refund of
// several milestones lists them all, e.g. "2, 3".
func milestoneCell(milestones []uint32) string {
	parts := make([]string, len(milestones))
	for i, m := range milestones {
		parts[i] = strconv.FormatUint(uint64(m)+1, 10)
	}
	return strings.Join(parts, ", ")
}

// csvSafe keeps a spreadsheet from reading a cell as a formula (OWASP CSV
// injection): a cell that starts with =, +, -, @, a tab, a line break, or
// their full-width forms gets a leading apostrophe.
func csvSafe(cell string) string {
	if cell == "" {
		return cell
	}
	switch cell[0] {
	case '=', '+', '-', '@', '\t', '\r', '\n':
		return "'" + cell
	}
	for _, prefix := range []string{"＝", "＋", "－", "＠"} {
		if strings.HasPrefix(cell, prefix) {
			return "'" + cell
		}
	}
	return cell
}
