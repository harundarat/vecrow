// Package mail sends plain-text emails: over SMTP (smtp.go), or only to the
// log in development (log.go). Queue (queue.go) sends them in the background.
//
// Every sender has the method Send(ctx, to, subject, body), which is also the
// shape of usecase.Mailer, so this package does not depend on the use cases.
package mail

import (
	"context"
	"errors"
	"fmt"
	netmail "net/mail"
	"strings"
)

// Name is the component name used in logs.
const Name = "mail"

// Sender sends one plain-text email.
type Sender interface {
	Send(ctx context.Context, to, subject, body string) error
}

// ErrInvalidMessage is returned for a recipient or subject that cannot be
// sent safely, such as one with a line break.
var ErrInvalidMessage = errors.New("mail: invalid message")

// parseAddress accepts a bare address ("bob@example.com"), without a display
// name, and returns it.
func parseAddress(s string) (string, error) {
	if strings.ContainsAny(s, "\r\n") {
		return "", fmt.Errorf("%w: recipient contains a line break", ErrInvalidMessage)
	}
	addr, err := netmail.ParseAddress(s)
	if err != nil || addr.Name != "" || addr.Address != s {
		return "", fmt.Errorf("%w: recipient is not a bare email address", ErrInvalidMessage)
	}
	return addr.Address, nil
}

// checkSubject rejects subjects that could inject header lines.
func checkSubject(s string) error {
	if strings.ContainsAny(s, "\r\n") {
		return fmt.Errorf("%w: subject contains a line break", ErrInvalidMessage)
	}
	return nil
}

// maskAddress hides the local part of an address for logs: "b***@example.com".
func maskAddress(addr string) string {
	local, domain, ok := strings.Cut(addr, "@")
	if !ok || local == "" {
		return "***"
	}
	return local[:1] + "***@" + domain
}
