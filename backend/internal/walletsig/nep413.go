package walletsig

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"strings"

	"github.com/mr-tron/base58"
)

// nep413Tag prefixes every NEP-413 payload (2^31 + 413) so a signed message
// can never be a valid transaction.
const nep413Tag = 1<<31 + 413

const nearKeyPrefix = "ed25519:"

// ParseNEARPublicKey decodes a NEAR public key in the form "ed25519:<base58>".
func ParseNEARPublicKey(s string) (ed25519.PublicKey, error) {
	enc, ok := strings.CutPrefix(s, nearKeyPrefix)
	if !ok {
		return nil, malformed("public_key must be an ed25519 key (ed25519:<base58>)")
	}
	pub, err := base58.Decode(enc)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return nil, malformed("public_key must be an ed25519 key (ed25519:<base58>)")
	}
	return pub, nil
}

// VerifyNEP413 checks a NEP-413 signMessage signature (base64, 64 bytes) by
// publicKey. The signed payload has no callbackUrl. It only checks the
// cryptography: whether publicKey belongs to the NEAR account is up to the
// caller.
func VerifyNEP413(message string, nonce [32]byte, recipient, publicKey, signature string) error {
	pub, err := ParseNEARPublicKey(publicKey)
	if err != nil {
		return err
	}
	sig, err := base64.StdEncoding.DecodeString(signature)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return malformed("signature must be base64 of 64 bytes")
	}
	hash := nep413Hash(message, nonce, recipient)
	if !ed25519.Verify(pub, hash[:], sig) {
		return ErrInvalidSignature
	}
	return nil
}

// nep413Hash is sha256 of the Borsh-serialised payload
// { tag: u32, message: string, nonce: [u8; 32], recipient: string, callbackUrl: Option<string> }.
func nep413Hash(message string, nonce [32]byte, recipient string) [32]byte {
	b := make([]byte, 0, 4+4+len(message)+32+4+len(recipient)+1)
	b = binary.LittleEndian.AppendUint32(b, nep413Tag)
	b = appendBorshString(b, message)
	b = append(b, nonce[:]...)
	b = appendBorshString(b, recipient)
	b = append(b, 0) // callbackUrl: None
	return sha256.Sum256(b)
}

// appendBorshString appends a u32 little-endian byte length and the bytes.
func appendBorshString(b []byte, s string) []byte {
	b = binary.LittleEndian.AppendUint32(b, uint32(len(s)))
	return append(b, s...)
}
