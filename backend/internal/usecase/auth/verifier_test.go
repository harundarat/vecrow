package auth_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/harundarat/vecrow/backend/internal/domain"
	"github.com/harundarat/vecrow/backend/internal/usecase/auth"
)

// Vectors shared with the walletsig tests: real signatures over login
// messages for recipient localhost:5173 and nonce "ab" * 32.
type sigVectors struct {
	EVM    struct{ Address, Message, Signature string } `json:"evm"`
	Solana struct{ Address, Message, Signature string } `json:"solana"`
	NEP413 struct {
		AccountID string `json:"account_id"`
		PublicKey string `json:"public_key"`
		Message   string `json:"message"`
		Signature string `json:"signature"`
	} `json:"nep413"`
}

const vectorNonce = "abababababababababababababababababababababababababababababababab"

func loadSigVectors(t *testing.T) sigVectors {
	t.Helper()
	b, err := os.ReadFile("../../walletsig/testdata/vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var v sigVectors
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

type fakeNEARKeys struct {
	status auth.AccessKeyStatus
	err    error
	calls  int
}

func (f *fakeNEARKeys) Status(context.Context, string, string) (auth.AccessKeyStatus, error) {
	f.calls++
	return f.status, f.err
}

func TestWalletVerifier_EVMAndSolana(t *testing.T) {
	v := loadSigVectors(t)
	verifier := auth.NewWalletVerifier("localhost:5173", &fakeNEARKeys{})
	ctx := context.Background()

	evm := domain.Nonce{
		Value:   vectorNonce,
		Wallet:  domain.Wallet{Scheme: domain.SchemeEIP191, Address: strings.ToLower(v.EVM.Address)},
		Message: v.EVM.Message,
	}
	if err := verifier.Verify(ctx, evm, auth.Proof{Signature: v.EVM.Signature}); err != nil {
		t.Errorf("EVM: %v", err)
	}
	evm.Message += "!"
	if err := verifier.Verify(ctx, evm, auth.Proof{Signature: v.EVM.Signature}); !errors.Is(err, domain.ErrUnauthorized) {
		t.Errorf("EVM tampered: error = %v, want ErrUnauthorized", err)
	}

	sol := domain.Nonce{
		Value:   vectorNonce,
		Wallet:  domain.Wallet{Scheme: domain.SchemeEd25519, Address: v.Solana.Address},
		Message: v.Solana.Message,
	}
	if err := verifier.Verify(ctx, sol, auth.Proof{Signature: v.Solana.Signature}); err != nil {
		t.Errorf("Solana: %v", err)
	}
	if err := verifier.Verify(ctx, sol, auth.Proof{Signature: "not base58 0OIl"}); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Errorf("Solana malformed: error = %v, want ErrInvalidArgument", err)
	}
}

func TestWalletVerifier_NEAR(t *testing.T) {
	v := loadSigVectors(t)
	implicit := domain.Nonce{
		Value:   vectorNonce,
		Wallet:  domain.Wallet{Scheme: domain.SchemeNEP413, Address: v.NEP413.AccountID},
		Message: v.NEP413.Message,
	}
	// A named account signing the same message with the same key (the key
	// cannot be derived from a name, so it must be checked on chain).
	named := implicit
	named.Wallet.Address = "alice.testnet"
	proof := auth.Proof{Signature: v.NEP413.Signature, PublicKey: v.NEP413.PublicKey}
	rpcDown := errors.New("nearrpc: query: request failed: timeout")

	tests := []struct {
		name      string
		nonce     domain.Nonce
		proof     auth.Proof
		recipient string
		keys      fakeNEARKeys
		wantErr   error // nil, a domain kind, or rpcDown
		wantRPC   bool
	}{
		{name: "full access key", nonce: implicit, proof: proof, keys: fakeNEARKeys{status: auth.AccessKeyFullAccess}, wantRPC: true},
		{name: "function-call key", nonce: implicit, proof: proof, keys: fakeNEARKeys{status: auth.AccessKeyLimited}, wantErr: domain.ErrUnauthorized, wantRPC: true},
		{name: "key not on account", nonce: implicit, proof: proof, keys: fakeNEARKeys{status: auth.AccessKeyMissing}, wantErr: domain.ErrUnauthorized, wantRPC: true},
		{name: "unfunded implicit account of the key", nonce: implicit, proof: proof, keys: fakeNEARKeys{status: auth.AccountMissing}, wantRPC: true},
		{name: "missing named account", nonce: named, proof: proof, keys: fakeNEARKeys{status: auth.AccountMissing}, wantErr: domain.ErrUnauthorized, wantRPC: true},
		{name: "rpc error is internal", nonce: implicit, proof: proof, keys: fakeNEARKeys{err: rpcDown}, wantErr: rpcDown, wantRPC: true},
		{name: "missing public key", nonce: implicit, proof: auth.Proof{Signature: v.NEP413.Signature}, wantErr: domain.ErrInvalidArgument},
		{name: "signed for another recipient", nonce: implicit, proof: proof, recipient: "evil.example", wantErr: domain.ErrUnauthorized},
		{name: "malformed signature", nonce: implicit, proof: auth.Proof{Signature: "***", PublicKey: v.NEP413.PublicKey}, wantErr: domain.ErrInvalidArgument},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recipient := tt.recipient
			if recipient == "" {
				recipient = "localhost:5173"
			}
			keys := tt.keys
			err := auth.NewWalletVerifier(recipient, &keys).Verify(context.Background(), tt.nonce, tt.proof)
			if tt.wantErr == nil && err != nil || tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == rpcDown {
				var de *domain.Error
				if errors.As(err, &de) {
					t.Error("RPC failures must not be reported as client errors")
				}
			}
			if (keys.calls > 0) != tt.wantRPC {
				t.Errorf("RPC calls = %d, want called = %v", keys.calls, tt.wantRPC)
			}
		})
	}
}
