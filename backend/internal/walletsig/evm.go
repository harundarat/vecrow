package walletsig

import (
	"encoding/hex"
	"strconv"
	"strings"

	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
	"golang.org/x/crypto/sha3"
)

// VerifyEIP191 checks an EIP-191 personal_sign signature (65 bytes R‖S‖V,
// hex with optional 0x prefix) of message by the EVM address.
// Smart-contract wallets (EIP-1271) are not supported.
func VerifyEIP191(address, message, signature string) error {
	sig, err := hex.DecodeString(strings.TrimPrefix(signature, "0x"))
	if err != nil || len(sig) != 65 {
		return malformed("signature must be hex of 65 bytes")
	}
	// Wallets return V as 27/28 (legacy) or 0/1.
	v := sig[64]
	if v >= 27 {
		v -= 27
	}
	if v > 1 {
		return malformed("signature has an invalid recovery id")
	}

	// decred expects <27+recovery id><R><S>.
	compact := make([]byte, 65)
	compact[0] = 27 + v
	copy(compact[1:], sig[:64])
	pub, _, err := ecdsa.RecoverCompact(compact, eip191Hash(message))
	if err != nil {
		return ErrInvalidSignature
	}
	if !strings.EqualFold(evmAddress(pub.SerializeUncompressed()), address) {
		return ErrInvalidSignature
	}
	return nil
}

// eip191Hash is keccak256("\x19Ethereum Signed Message:\n" + len(message) + message).
func eip191Hash(message string) []byte {
	return keccak256([]byte("\x19Ethereum Signed Message:\n" + strconv.Itoa(len(message)) + message))
}

// evmAddress derives the address from an uncompressed public key (0x04‖X‖Y):
// the last 20 bytes of keccak256(X‖Y).
func evmAddress(uncompressed []byte) string {
	return "0x" + hex.EncodeToString(keccak256(uncompressed[1:])[12:])
}

func keccak256(b []byte) []byte {
	h := sha3.NewLegacyKeccak256()
	h.Write(b)
	return h.Sum(nil)
}
