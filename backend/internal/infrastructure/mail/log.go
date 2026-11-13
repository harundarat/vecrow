package mail

import (
	"context"
	"log/slog"
)

// LogSender does not send anything: it writes one log line per email, for
// development without an SMTP server. The line holds the masked recipient and
// the subject, never the body, because bodies carry one-time codes, which must
// never reach the logs (spec.md §8).
type LogSender struct {
	log *slog.Logger
}

var _ Sender = (*LogSender)(nil)

func NewLogSender(log *slog.Logger) *LogSender {
	return &LogSender{log: log.With(slog.String("component", Name))}
}

func (s *LogSender) Send(ctx context.Context, to, subject, body string) error {
	rcpt, err := parseAddress(to)
	if err != nil {
		return err
	}
	if err := checkSubject(subject); err != nil {
		return err
	}
	s.log.InfoContext(ctx, "email not sent: no SMTP server is configured",
		slog.String("to", maskAddress(rcpt)),
		slog.String("subject", subject),
		slog.Int("body_bytes", len(body)))
	return nil
}
