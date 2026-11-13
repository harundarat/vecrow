package domain

import (
	"net/mail"
	"regexp"
	"strings"
	"time"
)

// Account is a backend identity: a username and the wallets proven to belong
// to it. It grants visibility only, never rights in the contract (D-08).
type Account struct {
	ID       string
	Username string
	// Email is the verified email, or "". Only a verified email matches
	// invitations (R-15) and allows recovery logins (R-22).
	Email string
	// PendingEmail is an email waiting for its one-time code, or "".
	PendingEmail string
	CreatedAt    time.Time
	Wallets      []LinkedWallet
}

// LinkedWallet is a wallet linked to an account.
type LinkedWallet struct {
	ID        string
	Wallet    Wallet
	CreatedAt time.Time
}

// Session is an authenticated login of an account.
type Session struct {
	AccountID string
	ExpiresAt time.Time
	// Recovery marks a session opened with an email code (R-22). It can only
	// view the account and link or unlink wallets; everything else needs a
	// session opened with a wallet signature.
	Recovery bool
}

// Usernames: 3–30 characters, lowercase letters, digits and underscores,
// starting with a letter. Generated usernames ("user_xxxxxxxx") match too.
var usernamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{2,29}$`)

// ParseUsername validates s and normalises it to lowercase, so usernames are
// unique regardless of case.
func ParseUsername(s string) (string, error) {
	u := strings.ToLower(strings.TrimSpace(s))
	if !usernamePattern.MatchString(u) {
		return "", InvalidArgument("username must be 3-30 characters of a-z, 0-9 or _, starting with a letter")
	}
	return u, nil
}

// maxEmailLen is the longest address SMTP can carry (RFC 5321 path limit).
const maxEmailLen = 254

// ParseEmail validates a bare email address ("bob@example.com", no display
// name) and normalises it to lowercase, so each address matches one account.
func ParseEmail(s string) (string, error) {
	e := strings.ToLower(strings.TrimSpace(s))
	invalid := InvalidArgument("email must be a valid address such as name@example.com")
	if e == "" || len(e) > maxEmailLen || strings.ContainsFunc(e, func(r rune) bool { return r <= ' ' || r == 0x7f }) {
		return "", invalid
	}
	addr, err := mail.ParseAddress(e)
	if err != nil || addr.Name != "" || addr.Address != e {
		return "", invalid
	}
	local, host, ok := strings.Cut(e, "@")
	if !ok || local == "" || !strings.Contains(host, ".") || strings.HasPrefix(host, "[") {
		return "", invalid
	}
	return e, nil
}
