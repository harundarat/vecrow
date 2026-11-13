package mail

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"mime"
	"mime/quotedprintable"
	netmail "net/mail"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	testCode = "482915"
	testBody = "Your Vecrow code is " + testCode + ".\nIt expires in 10 minutes."
)

func TestLogSender_NeverLogsTheBody(t *testing.T) {
	var buf bytes.Buffer
	s := NewLogSender(slog.New(slog.NewJSONHandler(&buf, nil)))
	if err := s.Send(context.Background(), "bob@example.com", "Your Vecrow code", testBody); err != nil {
		t.Fatalf("Send: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, testCode) {
		t.Fatalf("log contains the code: %s", out)
	}
	if strings.Contains(out, "bob@example.com") {
		t.Fatalf("log contains the full recipient: %s", out)
	}
	for _, want := range []string{`"subject":"Your Vecrow code"`, `"to":"b***@example.com"`} {
		if !strings.Contains(out, want) {
			t.Errorf("log %s misses %s", out, want)
		}
	}
}

func TestSenders_RejectHeaderInjection(t *testing.T) {
	smtpSender, err := NewSMTP(SMTPConfig{Host: "127.0.0.1", Port: 1, From: "no-reply@vecrow.test", TLS: TLSNone})
	if err != nil {
		t.Fatalf("NewSMTP: %v", err)
	}
	queue := NewQueue(&recordingSender{}, slog.New(slog.DiscardHandler), QueueConfig{Size: 1})
	defer func() { _ = queue.Close() }()
	senders := map[string]Sender{
		"log":   NewLogSender(slog.New(slog.DiscardHandler)),
		"smtp":  smtpSender,
		"queue": queue,
	}
	cases := []struct{ to, subject string }{
		{"bob@example.com\r\nBcc: eve@example.com", "Hello"},
		{"Bob <bob@example.com>", "Hello"},
		{"not an address", "Hello"},
		{"bob@example.com", "Hello\r\nBcc: eve@example.com"},
	}
	for name, s := range senders {
		for _, tc := range cases {
			err := s.Send(context.Background(), tc.to, tc.subject, "body")
			if !errors.Is(err, ErrInvalidMessage) {
				t.Errorf("%s: Send(%q, %q) error = %v, want ErrInvalidMessage", name, tc.to, tc.subject, err)
			}
		}
	}
}

func TestNewSMTP_Validates(t *testing.T) {
	valid := SMTPConfig{Host: "smtp.example.com", Port: 587, From: "Vecrow <no-reply@vecrow.test>", TLS: TLSStartTLS}
	if _, err := NewSMTP(valid); err != nil {
		t.Fatalf("NewSMTP(valid): %v", err)
	}
	local := SMTPConfig{Host: "localhost", Port: 1025, Username: "u", Password: "p", From: "a@b.c", TLS: TLSNone}
	if _, err := NewSMTP(local); err != nil {
		t.Fatalf("NewSMTP(plain text to localhost): %v", err)
	}
	for name, mutate := range map[string]func(*SMTPConfig){
		"no host":       func(c *SMTPConfig) { c.Host = "" },
		"host and port": func(c *SMTPConfig) { c.Host = "smtp.example.com:587" },
		"port":          func(c *SMTPConfig) { c.Port = 0 },
		"tls mode":      func(c *SMTPConfig) { c.TLS = "ssl" },
		"from":          func(c *SMTPConfig) { c.From = "vecrow" },
		"from newline":  func(c *SMTPConfig) { c.From = "a@b.c\r\nBcc: e@f.g" },
		"password in plain text": func(c *SMTPConfig) {
			c.TLS, c.Username, c.Password = TLSNone, "user", "password"
		},
	} {
		cfg := valid
		mutate(&cfg)
		if _, err := NewSMTP(cfg); err == nil {
			t.Errorf("%s: NewSMTP accepted %+v", name, cfg)
		}
	}
}

func TestParseTLSMode(t *testing.T) {
	for in, want := range map[string]TLSMode{"starttls": TLSStartTLS, " TLS ": TLSImplicit, "none": TLSNone} {
		if got, err := ParseTLSMode(in); err != nil || got != want {
			t.Errorf("ParseTLSMode(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := ParseTLSMode("ssl"); err == nil {
		t.Error("ParseTLSMode(ssl) succeeded")
	}
}

func TestSMTP_Compose(t *testing.T) {
	s, err := NewSMTP(SMTPConfig{Host: "smtp.example.com", Port: 587, From: "Vecrow <no-reply@vecrow.test>", TLS: TLSStartTLS})
	if err != nil {
		t.Fatalf("NewSMTP: %v", err)
	}
	now := time.Date(2026, 11, 13, 9, 30, 0, 0, time.UTC)
	raw, err := s.compose("bob@example.com", "Código Vecrow", "Line one — ünïcode\nLine two", now)
	if err != nil {
		t.Fatalf("compose: %v", err)
	}
	msg, err := netmail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	subject, err := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject"))
	if err != nil || subject != "Código Vecrow" {
		t.Errorf("Subject = %q, %v", subject, err)
	}
	if got := msg.Header.Get("From"); got != `"Vecrow" <no-reply@vecrow.test>` {
		t.Errorf("From = %q", got)
	}
	if got := msg.Header.Get("To"); got != "bob@example.com" {
		t.Errorf("To = %q", got)
	}
	if got := msg.Header.Get("Date"); got != "Fri, 13 Nov 2026 09:30:00 +0000" {
		t.Errorf("Date = %q", got)
	}
	if got := msg.Header.Get("Message-ID"); !strings.HasSuffix(got, "@vecrow.test>") {
		t.Errorf("Message-ID = %q", got)
	}
	body, err := io.ReadAll(quotedprintable.NewReader(msg.Body))
	if err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if got := strings.ReplaceAll(string(body), "\r\n", "\n"); got != "Line one — ünïcode\nLine two" {
		t.Errorf("body = %q", got)
	}
}

// recordingSender records emails; it blocks while gate is non-nil and open.
type recordingSender struct {
	mu   sync.Mutex
	sent []string
	gate chan struct{}
	err  error
}

func (r *recordingSender) Send(_ context.Context, to, _, body string) error {
	if r.gate != nil {
		<-r.gate
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sent = append(r.sent, to+": "+body)
	return r.err
}

func (r *recordingSender) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.sent)
}

func TestQueue_SendsInBackgroundAndDrainsOnClose(t *testing.T) {
	rec := &recordingSender{gate: make(chan struct{})}
	q := NewQueue(rec, slog.New(slog.DiscardHandler), QueueConfig{Size: 2, Workers: 1, DrainTimeout: 5 * time.Second})

	// The worker takes the first email and blocks on the gate; two more fill
	// the queue, and a fourth does not fit.
	if err := q.Send(context.Background(), "a@example.com", "s", "1"); err != nil {
		t.Fatalf("Send 1: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for len(q.jobs) != 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	for i, body := range []string{"2", "3"} {
		if err := q.Send(context.Background(), "a@example.com", "s", body); err != nil {
			t.Fatalf("Send %d: %v", i+2, err)
		}
	}
	if err := q.Send(context.Background(), "a@example.com", "s", "4"); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("Send 4 error = %v, want ErrQueueFull", err)
	}
	if rec.count() != 0 {
		t.Fatalf("sent %d emails before the gate opened", rec.count())
	}

	close(rec.gate)
	if err := q.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if rec.count() != 3 {
		t.Fatalf("sent %d emails, want 3", rec.count())
	}
	if err := q.Send(context.Background(), "a@example.com", "s", "5"); !errors.Is(err, ErrQueueClosed) {
		t.Fatalf("Send after Close error = %v, want ErrQueueClosed", err)
	}
}

func TestQueue_LogsFailuresWithoutTheBody(t *testing.T) {
	var buf bytes.Buffer
	var mu sync.Mutex
	log := slog.New(slog.NewJSONHandler(lockedWriter{&buf, &mu}, nil))
	rec := &recordingSender{err: errors.New("smtp: 451 try later")}
	q := NewQueue(rec, log, QueueConfig{Size: 1})
	if err := q.Send(context.Background(), "bob@example.com", "Your Vecrow code", testBody); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if err := q.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	mu.Lock()
	out := buf.String()
	mu.Unlock()
	if !strings.Contains(out, "email not sent") || !strings.Contains(out, "451 try later") {
		t.Fatalf("failure not logged: %s", out)
	}
	if strings.Contains(out, testCode) {
		t.Fatalf("log contains the code: %s", out)
	}
}

func TestQueue_CloseGivesUpAfterDrainTimeout(t *testing.T) {
	rec := &recordingSender{gate: make(chan struct{})}
	defer close(rec.gate)
	q := NewQueue(rec, slog.New(slog.DiscardHandler), QueueConfig{Size: 1, DrainTimeout: 50 * time.Millisecond})
	if err := q.Send(context.Background(), "a@example.com", "s", "1"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if err := q.Close(); err == nil {
		t.Fatal("Close returned nil while an email was stuck")
	}
}

type lockedWriter struct {
	w  io.Writer
	mu *sync.Mutex
}

func (l lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}
