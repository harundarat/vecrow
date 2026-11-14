package walletsigtest_test

import (
	"testing"

	"github.com/harundarat/vecrow/backend/internal/walletsig"
	"github.com/harundarat/vecrow/backend/internal/walletsig/walletsigtest"
)

func TestSignaturesVerify(t *testing.T) {
	const msg = "Vecrow approve_milestone\nContract: escrow.test.near"
	var nonce [32]byte
	nonce[0] = 7

	near := walletsigtest.Ed25519Key(3)
	sig := walletsigtest.SignNEP413(near, msg, nonce, "escrow.test.near")
	if err := walletsig.VerifyNEP413(msg, nonce, "escrow.test.near", walletsigtest.NEARPublicKey(near), sig); err != nil {
		t.Errorf("NEP-413: %v", err)
	}

	evm := walletsigtest.NewEVMKey(1)
	if evm.Address() != "0x7e5f4552091a69125d5dfcb7b8c2659029395bdf" {
		t.Errorf("EVM address of key 1 = %s", evm.Address())
	}
	for i := 0; i < 5; i++ { // both recovery ids
		m := msg + string(rune('a'+i))
		if err := walletsig.VerifyEIP191(evm.Address(), m, evm.SignEIP191(m)); err != nil {
			t.Errorf("EIP-191: %v", err)
		}
	}

	sol := walletsigtest.Ed25519Key(4)
	if err := walletsig.VerifySolana(walletsigtest.SolanaAddress(sol), msg, walletsigtest.SignSolana(sol, msg)); err != nil {
		t.Errorf("Solana: %v", err)
	}
}
