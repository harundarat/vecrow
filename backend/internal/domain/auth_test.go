package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/harundarat/vecrow/backend/internal/domain"
)

func TestParseWalletScheme(t *testing.T) {
	tests := []struct {
		in      string
		want    domain.WalletScheme
		wantErr bool
	}{
		{in: "nep413", want: domain.SchemeNEP413},
		{in: " EIP191 ", want: domain.SchemeEIP191},
		{in: "ed25519", want: domain.SchemeEd25519},
		{in: "", wantErr: true},
		{in: "secp256k1", wantErr: true},
	}
	for _, tt := range tests {
		got, err := domain.ParseWalletScheme(tt.in)
		if tt.wantErr {
			if !errors.Is(err, domain.ErrInvalidArgument) {
				t.Errorf("ParseWalletScheme(%q) error = %v, want ErrInvalidArgument", tt.in, err)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("ParseWalletScheme(%q) = %q, %v; want %q", tt.in, got, err, tt.want)
		}
	}
}

func TestNewWallet(t *testing.T) {
	tests := []struct {
		name    string
		scheme  domain.WalletScheme
		address string
		want    string
		wantErr bool
	}{
		{name: "near named", scheme: domain.SchemeNEP413, address: "alice.near", want: "alice.near"},
		{name: "near sub-account", scheme: domain.SchemeNEP413, address: "escrow_v1.vecrow-app.testnet", want: "escrow_v1.vecrow-app.testnet"},
		{name: "near implicit", scheme: domain.SchemeNEP413, address: strings.Repeat("ab", 32), want: strings.Repeat("ab", 32)},
		{name: "near trims spaces", scheme: domain.SchemeNEP413, address: "  bob.near ", want: "bob.near"},
		{name: "near uppercase", scheme: domain.SchemeNEP413, address: "Alice.near", wantErr: true},
		{name: "near double dot", scheme: domain.SchemeNEP413, address: "alice..near", wantErr: true},
		{name: "near too short", scheme: domain.SchemeNEP413, address: "a", wantErr: true},
		{name: "near too long", scheme: domain.SchemeNEP413, address: strings.Repeat("a", 65), wantErr: true},
		{name: "evm lowercased", scheme: domain.SchemeEIP191, address: "0xAbCdEf0123456789abcdef0123456789ABCDEF01", want: "0xabcdef0123456789abcdef0123456789abcdef01"},
		{name: "evm too short", scheme: domain.SchemeEIP191, address: "0xabc", wantErr: true},
		{name: "evm missing prefix", scheme: domain.SchemeEIP191, address: "abcdef0123456789abcdef0123456789abcdef01", wantErr: true},
		{name: "solana", scheme: domain.SchemeEd25519, address: "9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM", want: "9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM"},
		{name: "solana invalid base58 char", scheme: domain.SchemeEd25519, address: "0WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM", wantErr: true},
		{name: "solana system program (all zero bytes)", scheme: domain.SchemeEd25519, address: strings.Repeat("1", 32), want: strings.Repeat("1", 32)},
		{name: "solana decodes to fewer than 32 bytes", scheme: domain.SchemeEd25519, address: "2" + strings.Repeat("1", 31), wantErr: true},
		{name: "solana decodes to more than 32 bytes", scheme: domain.SchemeEd25519, address: strings.Repeat("z", 44), wantErr: true},
		{name: "unknown scheme", scheme: "foo", address: "alice.near", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := domain.NewWallet(tt.scheme, tt.address)
			if tt.wantErr {
				if !errors.Is(err, domain.ErrInvalidArgument) {
					t.Fatalf("error = %v, want ErrInvalidArgument", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Address != tt.want || got.Scheme != tt.scheme {
				t.Fatalf("got %+v, want address %q scheme %q", got, tt.want, tt.scheme)
			}
		})
	}
}
