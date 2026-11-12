package escrow

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/harundarat/vecrow/backend/internal/domain"
	"github.com/harundarat/vecrow/backend/internal/walletsig"
)

// The backend checks every signature before it sends anything, so invalid
// payloads cost no gas; the contract checks them again and is what decides.

// verifyFreelancerTerms checks the freelancer's NEP-413 signature over the
// terms, the key rules of D-18, and that the key is a full access key of the
// freelancer's account: a NEAR account ID is a name, and the contract can
// only check the signature against the key given with it (D-16).
func (s *Service) verifyFreelancerTerms(ctx context.Context, t domain.EscrowTerms, message, publicKey, signature string) error {
	if publicKey == "" {
		return domain.InvalidArgument("public_key is required for the freelancer's signature")
	}
	pub, err := domain.ParseNEARPublicKey(publicKey)
	if err != nil {
		return err
	}
	if err := t.CheckFreelancerKey(pub); err != nil {
		return err
	}
	nonce := domain.TermsNonce(message)
	if err := signatureError(walletsig.VerifyNEP413(message, nonce, s.ContractID(), publicKey, signature)); err != nil {
		return err
	}
	return s.checkFullAccess(ctx, t.Freelancer, publicKey, pub)
}

// verifyApproverTerms checks the approver's signature over the terms. A NEAR
// approver must sign with the key named in the terms, and it must be a full
// access key of the approver's account.
func (s *Service) verifyApproverTerms(ctx context.Context, a domain.Approver, message, signature string) error {
	switch a.Scheme {
	case domain.SchemeNEP413:
		nonce := domain.TermsNonce(message)
		if err := signatureError(walletsig.VerifyNEP413(message, nonce, s.ContractID(), a.PublicKey, signature)); err != nil {
			return err
		}
		pub, err := domain.ParseNEARPublicKey(a.PublicKey)
		if err != nil {
			return err
		}
		return s.checkFullAccess(ctx, a.AccountID, a.PublicKey, pub)
	case domain.SchemeEIP191:
		return signatureError(walletsig.VerifyEIP191(a.Address, message, signature))
	case domain.SchemeEd25519:
		return signatureError(walletsig.VerifySolana(a.Address, message, signature))
	default:
		return domain.InvalidArgument("unsupported approver scheme %q", a.Scheme)
	}
}

// verifyApproverAction checks the approver's signature over an action
// message, against the approver in the terms, as the contract will.
func (s *Service) verifyApproverAction(a domain.Approver, message string, nonce [domain.NonceSize]byte, signature string) error {
	switch a.Scheme {
	case domain.SchemeNEP413:
		return signatureError(walletsig.VerifyNEP413(message, nonce, s.ContractID(), a.PublicKey, signature))
	case domain.SchemeEIP191:
		return signatureError(walletsig.VerifyEIP191(a.Address, message, signature))
	case domain.SchemeEd25519:
		return signatureError(walletsig.VerifySolana(a.Address, message, signature))
	default:
		return domain.InvalidArgument("unsupported approver scheme %q", a.Scheme)
	}
}

// checkFullAccess requires publicKey to be a full access key of account. An
// implicit account that is not on chain yet is accepted when it is the hex of
// the key, so parties with a NEAR balance of 0 can sign.
func (s *Service) checkFullAccess(ctx context.Context, account, publicKey string, pub []byte) error {
	status, err := s.Keys.Status(ctx, account, publicKey)
	if err != nil {
		return fmt.Errorf("escrow: check NEAR access key: %w", err)
	}
	switch status {
	case domain.AccessKeyFullAccess:
		return nil
	case domain.AccountMissing:
		if hex.EncodeToString(pub) == account {
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
		return fmt.Errorf("escrow: verify signature: %w", err)
	}
}
