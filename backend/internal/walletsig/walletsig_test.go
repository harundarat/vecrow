package walletsig_test

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/mr-tron/base58"

	"github.com/harundarat/vecrow/backend/internal/walletsig"
)

// testdata/vectors.json was generated with independent implementations:
// ethers v6 (EIP-191), node:crypto ed25519 + bs58 (Solana) and the borsh JS
// package with the NEP-413 schema (NEAR). Keys are fixed test keys.
type vectors struct {
	EVM struct {
		Address, Message, Signature string
	} `json:"evm"`
	Solana struct {
		Address, Message, Signature string
	} `json:"solana"`
	NEP413 struct {
		AccountID string `json:"account_id"`
		PublicKey string `json:"public_key"`
		Message   string `json:"message"`
		Nonce     string `json:"nonce"`
		Recipient string `json:"recipient"`
		Signature string `json:"signature"`
	} `json:"nep413"`
}

func loadVectors(t *testing.T) vectors {
	t.Helper()
	b, err := os.ReadFile("testdata/vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var v vectors
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func isMalformed(err error) bool {
	var me *walletsig.MalformedError
	return errors.As(err, &me)
}

// flipHexByte returns sigHex with byte i flipped.
func flipHexByte(sigHex string, i int) string {
	b, _ := hex.DecodeString(strings.TrimPrefix(sigHex, "0x"))
	b[i] ^= 0x01
	return "0x" + hex.EncodeToString(b)
}

func TestVerifyEIP191(t *testing.T) {
	v := loadVectors(t).EVM
	raw, _ := hex.DecodeString(strings.TrimPrefix(v.Signature, "0x"))
	vZeroOne := append([]byte{}, raw...)
	vZeroOne[64] -= 27

	tests := []struct {
		name                       string
		address, msg, sig          string
		wantInvalid, wantMalformed bool
	}{
		{name: "valid, checksummed address", address: v.Address, msg: v.Message, sig: v.Signature},
		{name: "valid, lowercase address", address: strings.ToLower(v.Address), msg: v.Message, sig: v.Signature},
		{name: "valid without 0x prefix", address: v.Address, msg: v.Message, sig: strings.TrimPrefix(v.Signature, "0x")},
		{name: "valid with V as 0/1", address: v.Address, msg: v.Message, sig: "0x" + hex.EncodeToString(vZeroOne)},
		{name: "tampered message", address: v.Address, msg: v.Message + " ", sig: v.Signature, wantInvalid: true},
		{name: "other address", address: "0x70997970c51812dc3a010c7d01b50e0d17dc79c8", msg: v.Message, sig: v.Signature, wantInvalid: true},
		{name: "tampered S", address: v.Address, msg: v.Message, sig: flipHexByte(v.Signature, 40), wantInvalid: true},
		{name: "not hex", address: v.Address, msg: v.Message, sig: "0xzz", wantMalformed: true},
		{name: "64 bytes", address: v.Address, msg: v.Message, sig: v.Signature[:len(v.Signature)-2], wantMalformed: true},
		{name: "bad recovery id", address: v.Address, msg: v.Message, sig: v.Signature[:len(v.Signature)-2] + "05", wantMalformed: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := walletsig.VerifyEIP191(tt.address, tt.msg, tt.sig)
			switch {
			case tt.wantMalformed:
				if !isMalformed(err) {
					t.Fatalf("error = %v, want MalformedError", err)
				}
			case tt.wantInvalid:
				if !errors.Is(err, walletsig.ErrInvalidSignature) {
					t.Fatalf("error = %v, want ErrInvalidSignature", err)
				}
			default:
				if err != nil {
					t.Fatalf("error = %v, want nil", err)
				}
			}
		})
	}
}

func TestVerifySolana(t *testing.T) {
	v := loadVectors(t).Solana
	sig, _ := base58.Decode(v.Signature)
	sig[0] ^= 0x01
	tampered := base58.Encode(sig)

	if err := walletsig.VerifySolana(v.Address, v.Message, v.Signature); err != nil {
		t.Fatalf("valid signature: %v", err)
	}
	for name, tc := range map[string][3]string{
		"tampered message":   {v.Address, v.Message + " ", v.Signature},
		"tampered signature": {v.Address, v.Message, tampered},
		"other address":      {"9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM", v.Message, v.Signature},
	} {
		if err := walletsig.VerifySolana(tc[0], tc[1], tc[2]); !errors.Is(err, walletsig.ErrInvalidSignature) {
			t.Errorf("%s: error = %v, want ErrInvalidSignature", name, err)
		}
	}
	for name, tc := range map[string][3]string{
		"signature not base58":  {v.Address, v.Message, "0OIl"},
		"signature too short":   {v.Address, v.Message, base58.Encode(sig[:63])},
		"signature hex encoded": {v.Address, v.Message, hex.EncodeToString(sig)},
		"address not base58":    {"0x00", v.Message, v.Signature},
	} {
		if err := walletsig.VerifySolana(tc[0], tc[1], tc[2]); !isMalformed(err) {
			t.Errorf("%s: error = %v, want MalformedError", name, err)
		}
	}
}

func TestVerifyNEP413(t *testing.T) {
	v := loadVectors(t).NEP413
	var nonce [32]byte
	if _, err := hex.Decode(nonce[:], []byte(v.Nonce)); err != nil {
		t.Fatal(err)
	}
	otherNonce := nonce
	otherNonce[0] ^= 0x01
	sig, _ := base64.StdEncoding.DecodeString(v.Signature)

	if err := walletsig.VerifyNEP413(v.Message, nonce, v.Recipient, v.PublicKey, v.Signature); err != nil {
		t.Fatalf("valid signature: %v", err)
	}

	invalid := map[string]func() error{
		"tampered message": func() error {
			return walletsig.VerifyNEP413(v.Message+" ", nonce, v.Recipient, v.PublicKey, v.Signature)
		},
		"other nonce": func() error {
			return walletsig.VerifyNEP413(v.Message, otherNonce, v.Recipient, v.PublicKey, v.Signature)
		},
		"other recipient": func() error {
			return walletsig.VerifyNEP413(v.Message, nonce, "evil.example", v.PublicKey, v.Signature)
		},
		"other key": func() error {
			return walletsig.VerifyNEP413(v.Message, nonce, v.Recipient, "ed25519:9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM", v.Signature)
		},
	}
	for name, call := range invalid {
		if err := call(); !errors.Is(err, walletsig.ErrInvalidSignature) {
			t.Errorf("%s: error = %v, want ErrInvalidSignature", name, err)
		}
	}

	malformed := map[string][2]string{
		"key without prefix":   {strings.TrimPrefix(v.PublicKey, "ed25519:"), v.Signature},
		"secp256k1 key":        {"secp256k1:" + strings.TrimPrefix(v.PublicKey, "ed25519:"), v.Signature},
		"signature not base64": {v.PublicKey, "***"},
		"signature too short":  {v.PublicKey, base64.StdEncoding.EncodeToString(sig[:63])},
		"signature base58":     {v.PublicKey, base58.Encode(sig)},
	}
	for name, tc := range malformed {
		if err := walletsig.VerifyNEP413(v.Message, nonce, v.Recipient, tc[0], tc[1]); !isMalformed(err) {
			t.Errorf("%s: error = %v, want MalformedError", name, err)
		}
	}
}

func TestParseNEARPublicKey(t *testing.T) {
	v := loadVectors(t).NEP413
	pub, err := walletsig.ParseNEARPublicKey(v.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	// The implicit account ID is the hex public key.
	if got := hex.EncodeToString(pub); got != v.AccountID {
		t.Errorf("public key hex = %s, want %s", got, v.AccountID)
	}
}
