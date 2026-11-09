// Package walletsig verifies off-chain message signatures from the wallets
// Vecrow supports: EVM (EIP-191), Solana (ed25519) and NEAR (NEP-413).
//
// It is pure computation with no I/O. Proving that a NEAR public key belongs
// to an account needs an RPC call and is done by the caller.
//
// Inputs use each wallet's native encoding so the frontend can pass them on
// unchanged: hex for EVM signatures, base58 for Solana, base64 for NEAR.
package walletsig

import "errors"

// ErrInvalidSignature means the input is well-formed but the signature does
// not match the message and signer.
var ErrInvalidSignature = errors.New("walletsig: invalid signature")

// MalformedError reports input with the wrong encoding or length. Its message
// is safe to show to clients.
type MalformedError struct {
	Msg string
}

func (e *MalformedError) Error() string { return e.Msg }

func malformed(msg string) error { return &MalformedError{Msg: msg} }
