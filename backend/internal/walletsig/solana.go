package walletsig

import (
	"crypto/ed25519"

	"github.com/mr-tron/base58"
)

// VerifySolana checks an ed25519 signature (base58, 64 bytes) of the raw
// message bytes by the Solana address (base58 public key), as produced by
// signMessage in Phantom and other Solana wallets.
func VerifySolana(address, message, signature string) error {
	pub, err := base58.Decode(address)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return malformed("address is not a base58 ed25519 public key")
	}
	sig, err := base58.Decode(signature)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return malformed("signature must be base58 of 64 bytes")
	}
	if !ed25519.Verify(pub, []byte(message), sig) {
		return ErrInvalidSignature
	}
	return nil
}
