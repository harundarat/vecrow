package domain

import (
	"math/big"
	"strings"
)

// decodeBase58 decodes s with the Bitcoin alphabet NEAR and Solana use.
func decodeBase58(s string) ([]byte, bool) {
	if s == "" {
		return nil, false
	}
	zeros := 0
	for zeros < len(s) && s[zeros] == '1' {
		zeros++
	}
	n := new(big.Int)
	radix := big.NewInt(58)
	for i := zeros; i < len(s); i++ {
		d := strings.IndexByte(base58Alphabet, s[i])
		if d < 0 {
			return nil, false
		}
		n.Mul(n, radix)
		n.Add(n, big.NewInt(int64(d)))
	}
	out := make([]byte, zeros, zeros+len(n.Bytes()))
	return append(out, n.Bytes()...), true
}

// encodeBase58 encodes b with the Bitcoin alphabet; it is the canonical form
// the contract writes (one leading '1' per leading zero byte).
func encodeBase58(b []byte) string {
	zeros := 0
	for zeros < len(b) && b[zeros] == 0 {
		zeros++
	}
	n := new(big.Int).SetBytes(b[zeros:])
	radix := big.NewInt(58)
	mod := new(big.Int)
	var digits []byte
	for n.Sign() > 0 {
		n.DivMod(n, radix, mod)
		digits = append(digits, base58Alphabet[mod.Int64()])
	}
	out := make([]byte, 0, zeros+len(digits))
	for range zeros {
		out = append(out, '1')
	}
	for i := len(digits) - 1; i >= 0; i-- {
		out = append(out, digits[i])
	}
	return string(out)
}

// decodeEd25519Base58 decodes a base58 ed25519 public key (32 bytes).
func decodeEd25519Base58(s string) ([]byte, bool) {
	b, ok := decodeBase58(s)
	if !ok || len(b) != 32 {
		return nil, false
	}
	return b, true
}

// ParseNEARPublicKey decodes "ed25519:<base58>" into the 32 key bytes.
func ParseNEARPublicKey(s string) ([]byte, error) {
	rest, ok := strings.CutPrefix(s, "ed25519:")
	if !ok {
		return nil, InvalidArgument("public_key must be ed25519:<base58>")
	}
	key, ok := decodeEd25519Base58(rest)
	if !ok {
		return nil, InvalidArgument("public_key must be ed25519:<base58> of 32 bytes")
	}
	return key, nil
}

// FormatNEARPublicKey is the canonical "ed25519:<base58>" form of key.
func FormatNEARPublicKey(key []byte) string { return "ed25519:" + encodeBase58(key) }
