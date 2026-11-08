package database

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
)

// slowQueryThreshold is the duration above which a successful query is logged
// at warn level.
const slowQueryThreshold = 500 * time.Millisecond

// queryTracer logs queries through slog. Query arguments are never logged:
// they may contain personal data or secrets.
//
//   - slow query:  warn
//   - other query: debug (including failed ones: the error is returned and
//     logged once by the boundary that handles it)
type queryTracer struct {
	log *slog.Logger
}

type traceKey struct{}

type traceData struct {
	start time.Time
	sql   string
}

func newQueryTracer(log *slog.Logger) *queryTracer {
	return &queryTracer{log: log}
}

func (t *queryTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	return context.WithValue(ctx, traceKey{}, traceData{start: time.Now(), sql: data.SQL})
}

func (t *queryTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	td, ok := ctx.Value(traceKey{}).(traceData)
	if !ok {
		return
	}
	elapsed := time.Since(td.start)

	var (
		level = slog.LevelDebug
		msg   = "database query"
	)
	switch {
	case data.Err != nil:
		msg = "database query failed"
	case elapsed >= slowQueryThreshold:
		level, msg = slog.LevelWarn, "slow database query"
	}
	if !t.log.Enabled(ctx, level) {
		return
	}

	attrs := []slog.Attr{
		slog.String("sql", td.sql),
		slog.Float64("duration_ms", float64(elapsed.Microseconds())/1000),
	}
	if data.Err != nil {
		attrs = append(attrs, slog.String("error", data.Err.Error()))
	} else {
		attrs = append(attrs, slog.Int64("rows_affected", data.CommandTag.RowsAffected()))
	}
	t.log.LogAttrs(ctx, level, msg, attrs...)
}
