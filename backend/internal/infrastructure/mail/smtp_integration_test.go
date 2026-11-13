//go:build integration

package mail_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"io"
	"log/slog"
	"math/big"
	"mime/quotedprintable"
	"net"
	netmail "net/mail"
	"net/textproto"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/harundarat/vecrow/backend/internal/infrastructure/mail"
)

const (
	smtpUser     = "vecrow"
	smtpPassword = "s3cret-smtp-password"
	sender       = "Vecrow <no-reply@vecrow.test>"
)

// received is one email accepted by the fake server.
type received struct {
	from, to string
	data     string
	tls      bool
	authUser string
}

// fakeSMTP is a minimal SMTP server: EHLO, STARTTLS, AUTH PLAIN, MAIL, RCPT,
// DATA and QUIT, enough for net/smtp.
type fakeSMTP struct {
	t        *testing.T
	ln       net.Listener
	tls      *tls.Config
	starttls bool // offer STARTTLS on plain connections
	implicit bool // the listener is TLS from the start
	silent   bool // accept connections but never greet

	mu        sync.Mutex
	messages  []received
	authSeen  bool
	plainAuth bool // AUTH was received over a plain connection
}

func startFakeSMTP(t *testing.T, cert tls.Certificate, configure func(*fakeSMTP)) *fakeSMTP {
	t.Helper()
	s := &fakeSMTP{t: t, tls: &tls.Config{Certificates: []tls.Certificate{cert}}, starttls: true}
	if configure != nil {
		configure(s)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	if s.implicit {
		ln = tls.NewListener(ln, s.tls)
	}
	s.ln = ln
	t.Cleanup(func() { _ = ln.Close() })
	go s.serve()
	return s
}

func (s *fakeSMTP) port() int { return s.ln.Addr().(*net.TCPAddr).Port }

func (s *fakeSMTP) serve() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.handle(conn)
	}
}

func (s *fakeSMTP) handle(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	if s.silent {
		_, _ = io.Copy(io.Discard, conn)
		return
	}
	tp := textproto.NewConn(conn)
	isTLS := s.implicit
	var msg received
	reply := func(format string, args ...any) { _ = tp.PrintfLine(format, args...) }
	reply("220 fake.smtp ESMTP")
	for {
		line, err := tp.ReadLine()
		if err != nil {
			return
		}
		verb, arg, _ := strings.Cut(line, " ")
		switch strings.ToUpper(verb) {
		case "EHLO", "HELO":
			ext := []string{"fake.smtp"}
			if s.starttls && !isTLS {
				ext = append(ext, "STARTTLS")
			}
			ext = append(ext, "AUTH PLAIN")
			for i, e := range ext {
				sep := "-"
				if i == len(ext)-1 {
					sep = " "
				}
				reply("250%s%s", sep, e)
			}
		case "STARTTLS":
			reply("220 ready to start TLS")
			tc := tls.Server(conn, s.tls)
			if err := tc.Handshake(); err != nil {
				return
			}
			conn, tp, isTLS = tc, textproto.NewConn(tc), true
		case "AUTH":
			s.mu.Lock()
			s.authSeen = true
			s.plainAuth = s.plainAuth || !isTLS
			s.mu.Unlock()
			mech, payload, _ := strings.Cut(arg, " ")
			b, _ := base64.StdEncoding.DecodeString(payload)
			parts := strings.Split(string(b), "\x00")
			if mech != "PLAIN" || len(parts) != 3 || parts[1] != smtpUser || parts[2] != smtpPassword {
				reply("535 5.7.8 authentication failed")
				continue
			}
			msg.authUser = parts[1]
			reply("235 2.7.0 authenticated")
		case "MAIL":
			msg.from = arg
			reply("250 ok")
		case "RCPT":
			msg.to = arg
			reply("250 ok")
		case "DATA":
			reply("354 end with <CRLF>.<CRLF>")
			data, err := tp.ReadDotBytes()
			if err != nil {
				return
			}
			msg.data, msg.tls = string(data), isTLS
			s.mu.Lock()
			s.messages = append(s.messages, msg)
			s.mu.Unlock()
			msg = received{}
			reply("250 queued")
		case "QUIT":
			reply("221 bye")
			return
		default:
			reply("502 command not implemented")
		}
	}
}

func (s *fakeSMTP) received() []received {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]received(nil), s.messages...)
}

// selfSigned returns a certificate for 127.0.0.1 and a pool that trusts it.
func selfSigned(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "fake smtp"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}, pool
}

func newSender(t *testing.T, s *fakeSMTP, pool *x509.CertPool, mode mail.TLSMode, password string) *mail.SMTP {
	t.Helper()
	m, err := mail.NewSMTP(mail.SMTPConfig{
		Host: "127.0.0.1", Port: s.port(),
		Username: smtpUser, Password: password,
		From: sender, TLS: mode, Timeout: 5 * time.Second, RootCAs: pool,
	})
	if err != nil {
		t.Fatalf("NewSMTP: %v", err)
	}
	return m
}

func TestSMTP_SendsOverSTARTTLSWithAuth(t *testing.T) {
	cert, pool := selfSigned(t)
	srv := startFakeSMTP(t, cert, nil)
	m := newSender(t, srv, pool, mail.TLSStartTLS, smtpPassword)

	body := "Your Vecrow code is 482915.\nIt expires in 10 minutes."
	if err := m.Send(context.Background(), "bob@example.com", "Your Vecrow code", body); err != nil {
		t.Fatalf("Send: %v", err)
	}
	msgs := srv.received()
	if len(msgs) != 1 {
		t.Fatalf("server received %d emails, want 1", len(msgs))
	}
	got := msgs[0]
	if !got.tls || got.authUser != smtpUser {
		t.Errorf("tls = %v, auth user = %q; want TLS and %q", got.tls, got.authUser, smtpUser)
	}
	if got.from != "FROM:<no-reply@vecrow.test>" || got.to != "TO:<bob@example.com>" {
		t.Errorf("envelope = %q -> %q", got.from, got.to)
	}
	msg, err := netmail.ReadMessage(strings.NewReader(got.data))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if msg.Header.Get("Subject") != "Your Vecrow code" || msg.Header.Get("To") != "bob@example.com" {
		t.Errorf("headers = %v", msg.Header)
	}
	decoded, err := io.ReadAll(quotedprintable.NewReader(msg.Body))
	if err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if strings.TrimRight(strings.ReplaceAll(string(decoded), "\r\n", "\n"), "\n") != body {
		t.Errorf("body = %q, want %q", decoded, body)
	}
	if srv.plainAuth {
		t.Error("credentials were sent before STARTTLS")
	}
}

func TestSMTP_ImplicitTLS(t *testing.T) {
	cert, pool := selfSigned(t)
	srv := startFakeSMTP(t, cert, func(s *fakeSMTP) { s.implicit, s.starttls = true, false })
	m := newSender(t, srv, pool, mail.TLSImplicit, smtpPassword)
	if err := m.Send(context.Background(), "bob@example.com", "Hello", "Hi"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if msgs := srv.received(); len(msgs) != 1 || !msgs[0].tls {
		t.Fatalf("received = %+v", msgs)
	}
}

func TestSMTP_RefusesServerWithoutSTARTTLS(t *testing.T) {
	cert, pool := selfSigned(t)
	srv := startFakeSMTP(t, cert, func(s *fakeSMTP) { s.starttls = false })
	m := newSender(t, srv, pool, mail.TLSStartTLS, smtpPassword)
	err := m.Send(context.Background(), "bob@example.com", "Hello", "Hi")
	if err == nil || !strings.Contains(err.Error(), "STARTTLS") {
		t.Fatalf("Send error = %v, want a STARTTLS error", err)
	}
	if srv.authSeen || len(srv.received()) != 0 {
		t.Fatal("credentials or the email reached a server without TLS")
	}
}

func TestSMTP_PlainWithoutCredentials(t *testing.T) {
	cert, _ := selfSigned(t)
	srv := startFakeSMTP(t, cert, func(s *fakeSMTP) { s.starttls = false })
	m, err := mail.NewSMTP(mail.SMTPConfig{
		Host: "127.0.0.1", Port: srv.port(), From: sender, TLS: mail.TLSNone, Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewSMTP: %v", err)
	}
	if err := m.Send(context.Background(), "bob@example.com", "Hello", "Hi"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if msgs := srv.received(); len(msgs) != 1 || msgs[0].tls {
		t.Fatalf("received = %+v", msgs)
	}
}

func TestSMTP_RejectsWrongPasswordAndUntrustedCertificate(t *testing.T) {
	cert, pool := selfSigned(t)
	srv := startFakeSMTP(t, cert, nil)

	err := newSender(t, srv, pool, mail.TLSStartTLS, "wrong").Send(context.Background(), "bob@example.com", "Hello", "Hi")
	if err == nil || !strings.Contains(err.Error(), "535") {
		t.Fatalf("wrong password: Send error = %v, want 535", err)
	}
	if err := newSender(t, srv, nil, mail.TLSStartTLS, smtpPassword).Send(context.Background(), "bob@example.com", "Hello", "Hi"); err == nil {
		t.Fatal("Send trusted a self-signed certificate")
	}
	if n := len(srv.received()); n != 0 {
		t.Fatalf("server received %d emails, want 0", n)
	}
}

func TestSMTP_TimesOutOnSilentServer(t *testing.T) {
	cert, _ := selfSigned(t)
	srv := startFakeSMTP(t, cert, func(s *fakeSMTP) { s.silent = true })
	m, err := mail.NewSMTP(mail.SMTPConfig{
		Host: "127.0.0.1", Port: srv.port(), From: sender, TLS: mail.TLSStartTLS, Timeout: 200 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewSMTP: %v", err)
	}
	start := time.Now()
	if err := m.Send(context.Background(), "bob@example.com", "Hello", "Hi"); err == nil {
		t.Fatal("Send succeeded against a silent server")
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Fatalf("Send took %s, want about the 200ms timeout", d)
	}
}

func TestQueue_DeliversThroughSMTP(t *testing.T) {
	cert, pool := selfSigned(t)
	srv := startFakeSMTP(t, cert, nil)
	var logs bytes.Buffer
	q := mail.NewQueue(newSender(t, srv, pool, mail.TLSStartTLS, smtpPassword),
		slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
		mail.QueueConfig{Size: 10, Workers: 2, DrainTimeout: 10 * time.Second})
	for i := range 3 {
		if err := q.Send(context.Background(), "user"+strconv.Itoa(i)+"@example.com", "Hello", "code 482915"); err != nil {
			t.Fatalf("Send %d: %v", i, err)
		}
	}
	if err := q.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if n := len(srv.received()); n != 3 {
		t.Fatalf("server received %d emails, want 3", n)
	}
	if strings.Contains(logs.String(), "482915") || strings.Contains(logs.String(), smtpPassword) {
		t.Fatalf("logs contain the body or the password: %s", logs.String())
	}
}
