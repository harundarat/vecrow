package mail

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	netmail "net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

// TLSMode says how the connection to the SMTP server is encrypted.
type TLSMode string

const (
	// TLSStartTLS connects in plain text and requires the server to upgrade
	// the connection with STARTTLS before anything else is sent (port 587).
	TLSStartTLS TLSMode = "starttls"
	// TLSImplicit connects over TLS from the start (port 465).
	TLSImplicit TLSMode = "tls"
	// TLSNone sends in plain text. Meant for a mail catcher on the same
	// machine; net/smtp refuses to send a password over it to any other host.
	TLSNone TLSMode = "none"
)

// ParseTLSMode validates s.
func ParseTLSMode(s string) (TLSMode, error) {
	switch m := TLSMode(strings.ToLower(strings.TrimSpace(s))); m {
	case TLSStartTLS, TLSImplicit, TLSNone:
		return m, nil
	default:
		return "", fmt.Errorf("mail: TLS mode must be starttls, tls or none, got %q", s)
	}
}

const defaultTimeout = 10 * time.Second

// SMTPConfig configures an SMTP server.
type SMTPConfig struct {
	Host string
	Port int
	// Username and Password are sent with AUTH PLAIN when Username is set,
	// only over an encrypted connection. The password is never logged.
	Username string
	Password string
	// From is the sender, a bare address or "Name <address>".
	From string
	TLS  TLSMode
	// Timeout bounds one email, from connecting to QUIT. 0 means 10s.
	Timeout time.Duration
	// RootCAs verifies the server's certificate; nil uses the system pool.
	RootCAs *x509.CertPool
}

// SMTP sends emails through an SMTP server, one connection per email.
type SMTP struct {
	cfg  SMTPConfig
	from *netmail.Address
	addr string
}

var _ Sender = (*SMTP)(nil)

// NewSMTP validates cfg. It does not connect.
func NewSMTP(cfg SMTPConfig) (*SMTP, error) {
	if cfg.Host == "" || strings.ContainsAny(cfg.Host, " \r\n/:") {
		return nil, fmt.Errorf("mail: invalid SMTP host %q", cfg.Host)
	}
	if cfg.Port < 1 || cfg.Port > 65535 {
		return nil, fmt.Errorf("mail: SMTP port must be between 1 and 65535, got %d", cfg.Port)
	}
	if _, err := ParseTLSMode(string(cfg.TLS)); err != nil {
		return nil, err
	}
	if strings.ContainsAny(cfg.From, "\r\n") {
		return nil, errors.New("mail: sender address contains a line break")
	}
	from, err := netmail.ParseAddress(cfg.From)
	if err != nil {
		return nil, fmt.Errorf("mail: invalid sender address %q", cfg.From)
	}
	if cfg.TLS == TLSNone && cfg.Username != "" && !isLoopback(cfg.Host) {
		return nil, errors.New("mail: a password is sent in plain text with TLS mode none; it is allowed for localhost only")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	return &SMTP{cfg: cfg, from: from, addr: net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))}, nil
}

// Send delivers one plain-text email to one recipient.
func (s *SMTP) Send(ctx context.Context, to, subject, body string) error {
	rcpt, err := parseAddress(to)
	if err != nil {
		return err
	}
	if err := checkSubject(subject); err != nil {
		return err
	}
	msg, err := s.compose(rcpt, subject, body, time.Now())
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, s.cfg.Timeout)
	defer cancel()
	conn, err := s.dial(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	// net/smtp has no context support: the deadline bounds every read and
	// write, and cancelling ctx expires it at once.
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Now()) })
	defer stop()

	c, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		return fmt.Errorf("mail: smtp greeting: %w", err)
	}
	defer func() { _ = c.Close() }()

	if s.cfg.TLS == TLSStartTLS {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return errors.New("mail: the SMTP server does not offer STARTTLS")
		}
		if err := c.StartTLS(s.tlsConfig()); err != nil {
			return fmt.Errorf("mail: starttls: %w", err)
		}
	}
	if s.cfg.Username != "" {
		if ok, _ := c.Extension("AUTH"); !ok {
			return errors.New("mail: the SMTP server does not offer AUTH")
		}
		if err := c.Auth(smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)); err != nil {
			return fmt.Errorf("mail: smtp auth: %w", err)
		}
	}
	if err := c.Mail(s.from.Address); err != nil {
		return fmt.Errorf("mail: smtp MAIL FROM: %w", err)
	}
	if err := c.Rcpt(rcpt); err != nil {
		return fmt.Errorf("mail: smtp RCPT TO: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("mail: smtp DATA: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("mail: write message: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("mail: smtp end of DATA: %w", err)
	}
	if err := c.Quit(); err != nil {
		return fmt.Errorf("mail: smtp QUIT: %w", err)
	}
	return nil
}

// isLoopback matches the hosts net/smtp sends a password to without TLS.
func isLoopback(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func (s *SMTP) tlsConfig() *tls.Config {
	return &tls.Config{ServerName: s.cfg.Host, RootCAs: s.cfg.RootCAs, MinVersion: tls.VersionTLS12}
}

func (s *SMTP) dial(ctx context.Context) (net.Conn, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", s.addr)
	if err != nil {
		return nil, fmt.Errorf("mail: connect to SMTP server: %w", err)
	}
	if s.cfg.TLS != TLSImplicit {
		return conn, nil
	}
	tc := tls.Client(conn, s.tlsConfig())
	if err := tc.HandshakeContext(ctx); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("mail: tls handshake: %w", err)
	}
	return tc, nil
}

// compose builds the message: headers, then the body as quoted-printable
// UTF-8 text.
func (s *SMTP) compose(rcpt, subject, body string, now time.Time) ([]byte, error) {
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return nil, fmt.Errorf("mail: message id: %w", err)
	}
	_, domain, _ := strings.Cut(s.from.Address, "@")

	var b bytes.Buffer
	header := func(name, value string) { b.WriteString(name + ": " + value + "\r\n") }
	header("From", s.from.String())
	header("To", rcpt)
	header("Subject", mime.QEncoding.Encode("utf-8", subject))
	header("Date", now.UTC().Format(time.RFC1123Z))
	header("Message-ID", "<"+hex.EncodeToString(id)+"@"+domain+">")
	header("MIME-Version", "1.0")
	header("Content-Type", `text/plain; charset="utf-8"`)
	header("Content-Transfer-Encoding", "quoted-printable")
	b.WriteString("\r\n")

	qp := quotedprintable.NewWriter(&b)
	if _, err := qp.Write([]byte(strings.ReplaceAll(body, "\r\n", "\n"))); err != nil {
		return nil, fmt.Errorf("mail: encode body: %w", err)
	}
	if err := qp.Close(); err != nil {
		return nil, fmt.Errorf("mail: encode body: %w", err)
	}
	return b.Bytes(), nil
}
