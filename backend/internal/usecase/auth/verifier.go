package auth

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/harundarat/vecrow/backend/internal/domain"
	"github.com/harundarat/vecrow/backend/internal/walletsig"
)

// WalletVerifier verifies login signatures for every supported scheme.
type WalletVerifier struct {
	recipient string
	nearKeys  NEARAccessKeys
}

var _ SignatureVerifier = (*WalletVerifier)(nil)

// NewWalletVerifier returns a verifier that accepts NEP-413 signatures made
// for recipient and checks NEAR keys with nearKeys.
func NewWalletVerifier(recipient string, nearKeys NEARAccessKeys) *WalletVerifier {
	return &WalletVerifier{recipient: recipient, nearKeys: nearKeys}
}

func (v *WalletVerifier) Verify(ctx context.Context, n domain.Nonce, p Proof) error {
	switch n.Wallet.Scheme {
	case domain.SchemeEIP191:
		return signatureError(walletsig.VerifyEIP191(n.Wallet.Address, n.Message, p.Signature))
	case domain.SchemeEd25519:
		return signatureError(walletsig.VerifySolana(n.Wallet.Address, n.Message, p.Signature))
	case domain.SchemeNEP413:
		return v.verifyNEAR(ctx, n, p)
	default:
		return domain.InvalidArgument("unsupported wallet scheme %q", n.Wallet.Scheme)
	}
}

// verifyNEAR checks the signature, then that the signing key is a full access
// key of the account: a NEAR account ID is a name, not a public key.
func (v *WalletVerifier) verifyNEAR(ctx context.Context, n domain.Nonce, p Proof) error {
	if p.PublicKey == "" {
		return domain.InvalidArgument("public_key is required for NEAR wallets")
	}
	nonce, err := domain.DecodeNonce(n.Value)
	if err != nil {
		return err
	}
	if err := signatureError(walletsig.VerifyNEP413(n.Message, nonce, v.recipient, p.PublicKey, p.Signature)); err != nil {
		return err
	}

	account := n.Wallet.Address
	status, err := v.nearKeys.Status(ctx, account, p.PublicKey)
	if err != nil {
		return fmt.Errorf("auth: check NEAR access key: %w", err)
	}
	switch status {
	case AccessKeyFullAccess:
		return nil
	case AccountMissing:
		// A new implicit account (hex of its public key) exists on chain only
		// once it is funded; users with a NEAR balance of 0 must still log in.
		if pub, err := walletsig.ParseNEARPublicKey(p.PublicKey); err == nil && hex.EncodeToString(pub) == account {
			return nil
		}
		return domain.Unauthorized("NEAR account %s does not exist", account)
	default:
		return domain.Unauthorized("public_key is not a full access key of %s", account)
	}
}

// signatureError maps walletsig errors to domain errors.
func signatureError(err error) error {
	var malformed *walletsig.MalformedError
	switch {
	case err == nil:
		return nil
	case errors.As(err, &malformed):
		return domain.InvalidArgument("%s", malformed.Msg)
	case errors.Is(err, walletsig.ErrInvalidSignature):
		return domain.Unauthorized("signature is invalid")
	default:
		return fmt.Errorf("auth: verify signature: %w", err)
	}
}

// proveWallet consumes the nonce of p and verifies the signature, returning
// the wallet that signed. The nonce is consumed even when the signature is
// wrong, so each nonce allows a single attempt.
func proveWallet(ctx context.Context, nonces NonceStore, verifier SignatureVerifier, p Proof) (domain.Wallet, error) {
	if _, err := domain.DecodeNonce(p.Nonce); err != nil {
		return domain.Wallet{}, err
	}
	n, err := nonces.Consume(ctx, p.Nonce)
	if errors.Is(err, ErrNonceNotFound) {
		return domain.Wallet{}, domain.Unauthorized("nonce is invalid, expired or already used")
	}
	if err != nil {
		return domain.Wallet{}, fmt.Errorf("auth: consume nonce: %w", err)
	}
	if err := verifier.Verify(ctx, n, p); err != nil {
		return domain.Wallet{}, err
	}
	return n.Wallet, nil
}
