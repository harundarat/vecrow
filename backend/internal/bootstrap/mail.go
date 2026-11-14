package bootstrap

import (
	"log/slog"
	"time"

	"github.com/harundarat/vecrow/backend/internal/config"
	"github.com/harundarat/vecrow/backend/internal/infrastructure/mail"
	"github.com/harundarat/vecrow/backend/internal/usecase"
)

// Emails wait in a queue and are sent in the background (mail.Queue).
const (
	mailQueueSize    = 256
	mailQueueWorkers = 4
	mailDrainTimeout = 10 * time.Second
)

// newMailer returns the SMTP sender when SMTP_HOST is set, otherwise the
// log-only sender (development; config.RequireMail rejects it in
// production), behind a queue that is drained on close.
func newMailer(cfg config.Config, log *slog.Logger, c *closers) (usecase.Mailer, error) {
	var sender mail.Sender
	if cfg.Mail.SMTPHost == "" {
		log.Warn("SMTP_HOST is not set: emails are logged without their body, not sent",
			slog.String("component", mail.Name))
		sender = mail.NewLogSender(log)
	} else {
		mode, err := mail.ParseTLSMode(cfg.Mail.SMTPTLS)
		if err != nil {
			return nil, err
		}
		smtp, err := mail.NewSMTP(mail.SMTPConfig{
			Host:     cfg.Mail.SMTPHost,
			Port:     cfg.Mail.SMTPPort,
			Username: cfg.Mail.SMTPUsername,
			Password: cfg.Mail.SMTPPassword,
			From:     cfg.Mail.From,
			TLS:      mode,
			Timeout:  cfg.Mail.SMTPTimeout,
		})
		if err != nil {
			return nil, err
		}
		log.Info("emails are sent over SMTP", slog.String("component", mail.Name),
			slog.String("host", cfg.Mail.SMTPHost), slog.Int("port", cfg.Mail.SMTPPort),
			slog.String("tls", string(mode)))
		sender = smtp
	}
	q := mail.NewQueue(sender, log, mail.QueueConfig{
		Size: mailQueueSize, Workers: mailQueueWorkers, DrainTimeout: mailDrainTimeout,
	})
	c.add(mail.Name, q.Close)
	return q, nil
}
