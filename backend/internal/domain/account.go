package domain

import (
	"regexp"
	"strings"
	"time"
)

// Account is a backend identity: a username and the wallets proven to belong
// to it. It grants visibility only, never rights in the contract (D-08).
type Account struct {
	ID        string
	Username  string
	CreatedAt time.Time
	Wallets   []LinkedWallet
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
