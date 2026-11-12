// Package walletsigtest signs messages the way NEAR, EVM and Solana wallets
// do, for tests. It is written separately from walletsig's verifiers, so a
// signature it makes that walletsig accepts is evidence both follow the
// wallet formats. Keys are deterministic test keys; never use them for real
// funds.
package walletsigtest

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"strconv"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
	"github.com/mr-tron/base58"
	"golang.org/x/crypto/sha3"
)

// Ed25519Key returns the ed25519 key whose 32-byte seed is filled with seed.
func Ed25519Key(seed byte) ed25519.PrivateKey {
	s := make([]byte, ed25519.SeedSize)
	for i := range s {
		s[i] = seed
	}
	return ed25519.NewKeyFromSeed(s)
}

// NEARPublicKey is the "ed25519:<base58>" form of key's public key.
func NEARPublicKey(key ed25519.PrivateKey) string {
	return "ed25519:" + base58.Encode(key.Public().(ed25519.PublicKey))
}

// ImplicitAccount is the NEAR implicit account of key: hex of its public key.
func ImplicitAccount(key ed25519.PrivateKey) string {
	return hex.EncodeToString(key.Public().(ed25519.PublicKey))
}

// SolanaAddress is the base58 public key of key, as Phantom shows it.
func SolanaAddress(key ed25519.PrivateKey) string {
	return base58.Encode(key.Public().(ed25519.PublicKey))
}

// SignNEP413 returns what a NEAR wallet's signMessage({message, nonce,
// recipient}) returns as signature: base64 of the ed25519 signature of
// sha256(borsh(2^31 + 413) ‖ borsh({message, nonce, recipient, None})).
func SignNEP413(key ed25519.PrivateKey, message string, nonce [32]byte, recipient string) string {
	var b []byte
	b = binary.LittleEndian.AppendUint32(b, 1<<31+413)
	b = binary.LittleEndian.AppendUint32(b, uint32(len(message)))
	b = append(b, message...)
	b = append(b, nonce[:]...)
	b = binary.LittleEndian.AppendUint32(b, uint32(len(recipient)))
	b = append(b, recipient...)
	b = append(b, 0)
	h := sha256.Sum256(b)
	return base64.StdEncoding.EncodeToString(ed25519.Sign(key, h[:]))
}

// SignSolana returns what Phantom's signMessage returns, base58 encoded:
// the ed25519 signature of the message bytes.
func SignSolana(key ed25519.PrivateKey, message string) string {
	return base58.Encode(ed25519.Sign(key, []byte(message)))
}

// EVMKey is a secp256k1 key for EIP-191 signatures.
type EVMKey struct{ priv *secp256k1.PrivateKey }

// NewEVMKey returns the key with the 32-byte big-endian scalar n (n > 0).
func NewEVMKey(n byte) EVMKey {
	b := make([]byte, 32)
	b[31] = n
	return EVMKey{priv: secp256k1.PrivKeyFromBytes(b)}
}

// Address is the lowercase 0x address of the key.
func (k EVMKey) Address() string {
	pub := k.priv.PubKey().SerializeUncompressed()
	return "0x" + hex.EncodeToString(keccak256(pub[1:])[12:])
}

// SignEIP191 returns what personal_sign returns: 0x hex of r ‖ s ‖ v with v
// 27 or 28.
func (k EVMKey) SignEIP191(message string) string {
	h := keccak256([]byte("\x19Ethereum Signed Message:\n" + strconv.Itoa(len(message)) + message))
	compact := ecdsa.SignCompact(k.priv, h, false) // v ‖ r ‖ s
	out := append(append([]byte(nil), compact[1:]...), compact[0])
	return "0x" + hex.EncodeToString(out)
}

func keccak256(b []byte) []byte {
	h := sha3.NewLegacyKeccak256()
	h.Write(b)
	return h.Sum(nil)
}
