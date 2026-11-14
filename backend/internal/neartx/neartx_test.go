package neartx_test

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"os"
	"strings"
	"testing"

	"github.com/mr-tron/base58"

	"github.com/harundarat/vecrow/backend/internal/neartx"
)

// testdata/vectors.json was generated with nearcore's near-primitives and
// near-crypto 0.38.1 from the same inputs as below.
type vectors struct {
	RelayerPK   string `json:"relayer_pk"`
	AlicePK     string `json:"alice_pk"`
	Tx1         string `json:"tx1"`
	Tx1Hash     string `json:"tx1_hash"`
	Tx1Signed   string `json:"tx1_signed"`
	Tx2         string `json:"tx2"`
	DA          string `json:"da"`
	DASignable  string `json:"da_signable"`
	SDA         string `json:"sda"`
	SDABase64   string `json:"sda_b64"`
	SDAVerifies bool   `json:"sda_verifies"`
	Tx3         string `json:"tx3"`
	Tx3Hash     string `json:"tx3_hash"`
}

func load(t *testing.T) vectors {
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

func seedKey(b byte) ed25519.PrivateKey {
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = b
	}
	return ed25519.NewKeyFromSeed(seed)
}

func filled(b byte) [32]byte {
	var out [32]byte
	for i := range out {
		out[i] = b
	}
	return out
}

func submitCall() neartx.FunctionCall {
	return neartx.FunctionCall{
		MethodName: "submit_milestone",
		Args:       []byte(`{"escrow_id":"e1","milestone":0}`),
		Gas:        30 * neartx.TGas,
	}
}

func delegateAction() neartx.DelegateAction {
	return neartx.DelegateAction{
		SenderID:       "alice.test.near",
		ReceiverID:     "escrow.test.near",
		Actions:        []neartx.Action{submitCall()},
		Nonce:          5,
		MaxBlockHeight: 1000,
		PublicKey:      neartx.PublicKeyOf(seedKey(2)),
	}
}

func TestKeys(t *testing.T) {
	v := load(t)
	if got := neartx.PublicKeyOf(seedKey(1)).String(); got != v.RelayerPK {
		t.Fatalf("relayer public key = %s, want %s", got, v.RelayerPK)
	}
	pk, err := neartx.ParsePublicKey(v.AlicePK)
	if err != nil || pk != neartx.PublicKeyOf(seedKey(2)) {
		t.Fatalf("ParsePublicKey = %v, %v", pk, err)
	}
	key := seedKey(1)
	parsed, err := neartx.ParsePrivateKey(neartx.FormatPrivateKey(key))
	if err != nil || !parsed.Equal(key) {
		t.Fatalf("ParsePrivateKey round trip: %v", err)
	}
	// A key whose public half does not match its seed is rejected.
	bad := append([]byte(nil), key...)
	bad[63] ^= 1
	if _, err := neartx.ParsePrivateKey("ed25519:" + base58.Encode(bad)); err == nil {
		t.Fatal("ParsePrivateKey accepted a mismatched key")
	}
	for _, s := range []string{"", "secp256k1:abc", "ed25519:0OIl", "ed25519:" + base58.Encode(key[:32])} {
		if _, err := neartx.ParsePrivateKey(s); err == nil {
			t.Errorf("ParsePrivateKey(%q) succeeded", s)
		}
	}
}

func TestFunctionCallTransaction(t *testing.T) {
	v := load(t)
	tx := neartx.Transaction{
		SignerID:   "relayer.test.near",
		PublicKey:  neartx.PublicKeyOf(seedKey(1)),
		Nonce:      7,
		ReceiverID: "escrow.test.near",
		BlockHash:  filled(9),
		Actions:    []neartx.Action{submitCall(), neartx.Transfer{Deposit: big.NewInt(1)}},
	}
	if got := hex.EncodeToString(tx.Encode()); got != v.Tx1 {
		t.Fatalf("Encode() =\n%s\nwant\n%s", got, v.Tx1)
	}
	h := tx.Hash()
	if got := hex.EncodeToString(h[:]); got != v.Tx1Hash {
		t.Fatalf("Hash() = %s, want %s", got, v.Tx1Hash)
	}
	signed := neartx.Sign(tx, seedKey(1))
	if got := hex.EncodeToString(signed.Encode()); got != v.Tx1Signed {
		t.Fatalf("signed =\n%s\nwant\n%s", got, v.Tx1Signed)
	}
	if signed.Hash() != base58.Encode(h[:]) {
		t.Fatalf("SignedTransaction.Hash() = %s", signed.Hash())
	}
}

func TestAccountSetupTransaction(t *testing.T) {
	v := load(t)
	oneNEAR, _ := new(big.Int).SetString("1000000000000000000000000", 10)
	tx := neartx.Transaction{
		SignerID:   "test.near",
		PublicKey:  neartx.PublicKeyOf(seedKey(1)),
		Nonce:      1,
		ReceiverID: "alice.test.near",
		BlockHash:  filled(3),
		Actions: []neartx.Action{
			neartx.CreateAccount{},
			neartx.Transfer{Deposit: oneNEAR},
			neartx.AddKey{PublicKey: neartx.PublicKeyOf(seedKey(2))},
			neartx.AddKey{PublicKey: neartx.PublicKeyOf(seedKey(1)), Permission: &neartx.FunctionCallPermission{
				Allowance: big.NewInt(250), ReceiverID: "escrow.test.near", MethodNames: []string{"a", "bc"},
			}},
			neartx.DeployContract{Code: []byte{0, 'a', 's', 'm'}},
		},
	}
	if got := hex.EncodeToString(tx.Encode()); got != v.Tx2 {
		t.Fatalf("Encode() =\n%s\nwant\n%s", got, v.Tx2)
	}
}

func TestDelegateAction(t *testing.T) {
	v := load(t)
	da := delegateAction()
	if got := hex.EncodeToString(da.Encode()); got != v.DA {
		t.Fatalf("Encode() =\n%s\nwant\n%s", got, v.DA)
	}
	signable, _ := hex.DecodeString(v.DASignable)
	h := da.SigningHash()
	if want := sha256Hex(signable); hex.EncodeToString(h[:]) != want {
		t.Fatalf("SigningHash() = %x, want %s", h, want)
	}
	sda := neartx.SignDelegate(da, seedKey(2))
	if got := hex.EncodeToString(sda.Encode()); got != v.SDA {
		t.Fatalf("signed =\n%s\nwant\n%s", got, v.SDA)
	}
	if !sda.Verify() || !v.SDAVerifies {
		t.Fatal("signed delegate action does not verify")
	}

	// The relayer's transaction wrapping it.
	tx := neartx.Transaction{
		SignerID:   "relayer.test.near",
		PublicKey:  neartx.PublicKeyOf(seedKey(1)),
		Nonce:      8,
		ReceiverID: "alice.test.near",
		BlockHash:  filled(9),
		Actions:    []neartx.Action{neartx.Delegate{SignedDelegateAction: sda}},
	}
	if got := hex.EncodeToString(tx.Encode()); got != v.Tx3 {
		t.Fatalf("relayer tx =\n%s\nwant\n%s", got, v.Tx3)
	}
	th := tx.Hash()
	if got := hex.EncodeToString(th[:]); got != v.Tx3Hash {
		t.Fatalf("relayer tx hash = %s, want %s", got, v.Tx3Hash)
	}
}

func TestDecodeSignedDelegateAction(t *testing.T) {
	v := load(t)
	raw, err := base64.StdEncoding.DecodeString(v.SDABase64)
	if err != nil {
		t.Fatal(err)
	}
	sda, err := neartx.DecodeSignedDelegateAction(raw)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	da := sda.DelegateAction
	if da.SenderID != "alice.test.near" || da.ReceiverID != "escrow.test.near" || da.Nonce != 5 ||
		da.MaxBlockHeight != 1000 || len(da.Actions) != 1 {
		t.Fatalf("decoded = %+v", da)
	}
	fc, ok := da.Actions[0].(neartx.FunctionCall)
	if !ok || fc.MethodName != "submit_milestone" || fc.Gas != 30*neartx.TGas || fc.Deposit.Sign() != 0 ||
		string(fc.Args) != `{"escrow_id":"e1","milestone":0}` {
		t.Fatalf("decoded action = %+v", da.Actions[0])
	}
	if !sda.Verify() {
		t.Fatal("decoded action does not verify")
	}
	if hex.EncodeToString(sda.Encode()) != v.SDA {
		t.Fatal("re-encoding differs")
	}

	// A changed byte breaks the signature.
	tampered := append([]byte(nil), raw...)
	tampered[len(tampered)-100] ^= 1
	if s, err := neartx.DecodeSignedDelegateAction(tampered); err == nil && s.Verify() {
		t.Fatal("tampered action verifies")
	}
}

func TestDecodeRejectsMalformed(t *testing.T) {
	good := neartx.SignDelegate(delegateAction(), seedKey(2)).Encode()

	nested := delegateAction()
	inner := neartx.SignDelegate(delegateAction(), seedKey(2))
	nested.Actions = []neartx.Action{neartx.Delegate{SignedDelegateAction: inner}}
	nestedBytes := neartx.SignDelegate(nested, seedKey(2)).Encode()

	secp := append([]byte(nil), good...)
	// The public key's key type sits right before its 32 bytes, which come
	// before the 65-byte signature.
	secp[len(secp)-65-32-1] = 1

	tests := map[string][]byte{
		"empty":           nil,
		"truncated":       good[:len(good)-1],
		"trailing bytes":  append(append([]byte(nil), good...), 0),
		"nested delegate": nestedBytes,
		"secp256k1 key":   secp,
		"huge length":     {0xff, 0xff, 0xff, 0xff},
	}
	for name, b := range tests {
		if _, err := neartx.DecodeSignedDelegateAction(b); !errors.Is(err, neartx.ErrMalformed) {
			t.Errorf("%s: error = %v, want ErrMalformed", name, err)
		}
	}
}

func sha256Hex(b []byte) string {
	h := sha256Sum(b)
	return hex.EncodeToString(h[:])
}

func TestU128Encoding(t *testing.T) {
	max, _ := new(big.Int).SetString(strings.Repeat("f", 32), 16)
	da := delegateAction()
	da.Actions = []neartx.Action{neartx.Transfer{Deposit: max}}
	sda := neartx.SignDelegate(da, seedKey(2))
	got, err := neartx.DecodeSignedDelegateAction(sda.Encode())
	if err != nil {
		t.Fatal(err)
	}
	if d := got.DelegateAction.Actions[0].(neartx.Transfer).Deposit; d.Cmp(max) != 0 {
		t.Fatalf("u128 round trip = %s", d)
	}
}

func sha256Sum(b []byte) [32]byte { return sha256.Sum256(b) }
