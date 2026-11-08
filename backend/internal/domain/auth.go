package domain

import (
	"regexp"
	"strings"
	"time"
)

// WalletScheme is the signature scheme a wallet signs with.
type WalletScheme string

const (
	SchemeNEP413  WalletScheme = "nep413"  // NEAR wallets
	SchemeEIP191  WalletScheme = "eip191"  // EVM wallets (MetaMask, ...)
	SchemeEd25519 WalletScheme = "ed25519" // Solana wallets (Phantom, ...)
)

// NonceSize is the length in bytes of a login nonce. NEP-413 requires 32.
const NonceSize = 32

var (
	// NEAR named account IDs: lowercase parts separated by dots, each part
	// using - or _ only between alphanumerics; 2–64 characters in total.
	nearNamedAccount = regexp.MustCompile(`^(([a-z\d]+[-_])*[a-z\d]+\.)*([a-z\d]+[-_])*[a-z\d]+$`)
	// NEAR implicit accounts: 64 lowercase hex characters.
	nearImplicitAccount = regexp.MustCompile(`^[0-9a-f]{64}$`)
	evmAddress          = regexp.MustCompile(`^0x[0-9a-fA-F]{40}$`)
	// Solana addresses: base58-encoded 32-byte public keys.
	solanaAddress = regexp.MustCompile(`^[1-9A-HJ-NP-Za-km-z]{32,44}$`)
)

// ParseWalletScheme validates s.
func ParseWalletScheme(s string) (WalletScheme, error) {
	switch scheme := WalletScheme(strings.ToLower(strings.TrimSpace(s))); scheme {
	case SchemeNEP413, SchemeEIP191, SchemeEd25519:
		return scheme, nil
	default:
		return "", InvalidArgument("scheme must be one of nep413, eip191, ed25519")
	}
}

// Wallet is a validated wallet address together with its scheme.
type Wallet struct {
	Scheme  WalletScheme
	Address string
}

// NewWallet validates address for scheme and normalises it (EVM addresses
// are lowercased so the same wallet always maps to the same value).
func NewWallet(scheme WalletScheme, address string) (Wallet, error) {
	address = strings.TrimSpace(address)
	switch scheme {
	case SchemeNEP413:
		if !isNEARAccount(address) {
			return Wallet{}, InvalidArgument("address is not a valid NEAR account ID")
		}
	case SchemeEIP191:
		if !evmAddress.MatchString(address) {
			return Wallet{}, InvalidArgument("address is not a valid EVM address")
		}
		address = strings.ToLower(address)
	case SchemeEd25519:
		if !solanaAddress.MatchString(address) || !isBase58Of32Bytes(address) {
			return Wallet{}, InvalidArgument("address is not a valid Solana address")
		}
	default:
		return Wallet{}, InvalidArgument("unsupported wallet scheme %q", scheme)
	}
	return Wallet{Scheme: scheme, Address: address}, nil
}

func isNEARAccount(s string) bool {
	if len(s) < 2 || len(s) > 64 {
		return false
	}
	return nearImplicitAccount.MatchString(s) || nearNamedAccount.MatchString(s)
}

const base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

// isBase58Of32Bytes reports whether s (already checked to use only base58
// characters) decodes to exactly 32 bytes, the size of an ed25519 public key.
func isBase58Of32Bytes(s string) bool {
	// Leading '1's encode leading zero bytes.
	zeros := 0
	for zeros < len(s) && s[zeros] == '1' {
		zeros++
	}
	// Big-endian base-256 accumulator, at most 32 bytes.
	var buf []byte
	for i := zeros; i < len(s); i++ {
		carry := strings.IndexByte(base58Alphabet, s[i])
		if carry < 0 {
			return false
		}
		for j := len(buf) - 1; j >= 0; j-- {
			carry += int(buf[j]) * 58
			buf[j] = byte(carry)
			carry >>= 8
		}
		for carry > 0 {
			buf = append([]byte{byte(carry)}, buf...)
			carry >>= 8
		}
		if zeros+len(buf) > 32 {
			return false
		}
	}
	return zeros+len(buf) == 32
}

// Nonce is a single-use login challenge bound to one wallet.
type Nonce struct {
	Value     string // hex-encoded, NonceSize bytes
	Wallet    Wallet
	ExpiresAt time.Time
}
