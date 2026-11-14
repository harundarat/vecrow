package domain_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/harundarat/vecrow/backend/internal/domain"
)

func TestParseUsername(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "alice", want: "alice"},
		{in: "  Alice_99 ", want: "alice_99"},
		{in: "user_k3x9q2ab", want: "user_k3x9q2ab"},
		{in: "abc", want: "abc"},
		{in: strings.Repeat("a", 30), want: strings.Repeat("a", 30)},
		{in: "ab", wantErr: true},
		{in: strings.Repeat("a", 31), wantErr: true},
		{in: "9lives", wantErr: true},
		{in: "_alice", wantErr: true},
		{in: "alice.near", wantErr: true},
		{in: "al ice", wantErr: true},
		{in: "", wantErr: true},
	}
	for _, tt := range tests {
		got, err := domain.ParseUsername(tt.in)
		if tt.wantErr {
			if !errors.Is(err, domain.ErrInvalidArgument) {
				t.Errorf("ParseUsername(%q) error = %v, want ErrInvalidArgument", tt.in, err)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("ParseUsername(%q) = %q, %v; want %q", tt.in, got, err, tt.want)
		}
	}
}

func TestParseEmail(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "bob@example.com", want: "bob@example.com"},
		{in: "  Bob.Smith+vecrow@Example.COM ", want: "bob.smith+vecrow@example.com"},
		{in: "", wantErr: true},
		{in: "bob", wantErr: true},
		{in: "bob@localhost", wantErr: true},
		{in: "@example.com", wantErr: true},
		{in: "Bob <bob@example.com>", wantErr: true},
		{in: "bob@example.com\r\nbcc: eve@example.com", wantErr: true},
		{in: "bob smith@example.com", wantErr: true},
		{in: "bob@[127.0.0.1]", wantErr: true},
		{in: strings.Repeat("a", 250) + "@x.io", wantErr: true},
	}
	for _, tt := range tests {
		got, err := domain.ParseEmail(tt.in)
		if tt.wantErr {
			if !errors.Is(err, domain.ErrInvalidArgument) {
				t.Errorf("ParseEmail(%q) = %q, %v; want ErrInvalidArgument", tt.in, got, err)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("ParseEmail(%q) = %q, %v; want %q", tt.in, got, err, tt.want)
		}
	}
}

func TestDecodeNonce(t *testing.T) {
	got, err := domain.DecodeNonce(strings.Repeat("ab", 32))
	if err != nil {
		t.Fatalf("DecodeNonce: %v", err)
	}
	for _, b := range got {
		if b != 0xab {
			t.Fatalf("decoded = %x", got)
		}
	}
	for _, bad := range []string{"", "abc", strings.Repeat("AB", 32), strings.Repeat("zz", 32), strings.Repeat("ab", 33)} {
		if _, err := domain.DecodeNonce(bad); !errors.Is(err, domain.ErrInvalidArgument) {
			t.Errorf("DecodeNonce(%q) error = %v, want ErrInvalidArgument", bad, err)
		}
	}
}

func TestLoginMessage(t *testing.T) {
	n := domain.Nonce{
		Value:  strings.Repeat("ab", 32),
		Wallet: domain.Wallet{Scheme: domain.SchemeEIP191, Address: "0xf39fd6e51aad88f6f4ce6ab8827279cfffb92266"},
		// Sub-second precision and a non-UTC zone must not change the text.
		ExpiresAt: time.Date(2026, 11, 8, 19, 5, 0, 123456789, time.FixedZone("WIB", 7*3600)),
	}
	want := "localhost:5173 wants you to sign in with your wallet.\n\n" +
		"Wallet: 0xf39fd6e51aad88f6f4ce6ab8827279cfffb92266\n" +
		"Nonce: " + strings.Repeat("ab", 32) + "\n" +
		"Expires: 2026-11-08T12:05:00Z\n\n" +
		"Signing is free and does not send a transaction."
	if got := domain.LoginMessage("localhost:5173", n); got != want {
		t.Errorf("LoginMessage =\n%s\nwant\n%s", got, want)
	}
}
