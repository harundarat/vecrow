// Package neartx builds, signs and decodes NEAR transactions and NEP-366
// delegate actions in their Borsh encoding, with the standard library only.
//
// Only what the backend needs is supported: ed25519 keys, version 0
// transactions, and the actions CreateAccount, DeployContract, FunctionCall,
// Transfer, AddKey and Delegate. testdata/vectors.json holds encodings made
// by nearcore's own near-primitives, which the tests compare against byte for
// byte.
package neartx

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/mr-tron/base58"
)

// Action tags in nearcore's Action enum.
const (
	tagCreateAccount  = 0
	tagDeployContract = 1
	tagFunctionCall   = 2
	tagTransfer       = 3
	tagAddKey         = 5
	tagDelegate       = 8
)

// keyTypeED25519 is the KeyType tag of ed25519 keys and signatures.
const keyTypeED25519 = 0

// delegateDiscriminant prefixes a delegate action when it is signed:
// 2^30 + 366 (NEP-461 signable messages, NEP-366 meta transactions).
const delegateDiscriminant = 1<<30 + 366

// TGas is 10^12 gas.
const TGas uint64 = 1_000_000_000_000

// PublicKey is an ed25519 public key.
type PublicKey [ed25519.PublicKeySize]byte

// ParsePublicKey parses "ed25519:<base58>".
func ParsePublicKey(s string) (PublicKey, error) {
	var pk PublicKey
	rest, ok := strings.CutPrefix(s, "ed25519:")
	if !ok {
		return pk, errors.New("neartx: public key must start with ed25519:")
	}
	b, err := base58.Decode(rest)
	if err != nil || len(b) != len(pk) {
		return pk, errors.New("neartx: public key must be 32 bytes of base58")
	}
	copy(pk[:], b)
	return pk, nil
}

func (pk PublicKey) String() string { return "ed25519:" + base58.Encode(pk[:]) }

// ParsePrivateKey parses a NEAR secret key, "ed25519:<base58>" of the 64-byte
// seed ‖ public key, as near-cli and key files store it. The public half must
// match the seed.
func ParsePrivateKey(s string) (ed25519.PrivateKey, error) {
	rest, ok := strings.CutPrefix(s, "ed25519:")
	if !ok {
		return nil, errors.New("neartx: private key must start with ed25519:")
	}
	b, err := base58.Decode(rest)
	if err != nil || len(b) != ed25519.PrivateKeySize {
		return nil, errors.New("neartx: private key must be 64 bytes of base58")
	}
	key := ed25519.NewKeyFromSeed(b[:ed25519.SeedSize])
	if string(key) != string(b) {
		return nil, errors.New("neartx: private key does not match its public key")
	}
	return key, nil
}

// FormatPrivateKey is the "ed25519:<base58>" form of key.
func FormatPrivateKey(key ed25519.PrivateKey) string { return "ed25519:" + base58.Encode(key) }

// PublicKeyOf returns the public key of key.
func PublicKeyOf(key ed25519.PrivateKey) PublicKey {
	var pk PublicKey
	copy(pk[:], key.Public().(ed25519.PublicKey))
	return pk
}

// Signature is an ed25519 signature.
type Signature [ed25519.SignatureSize]byte

// Action is a transaction action.
type Action interface {
	encode(e *encoder)
}

// CreateAccount creates the receiver account.
type CreateAccount struct{}

// DeployContract deploys Code to the receiver.
type DeployContract struct{ Code []byte }

// FunctionCall calls MethodName on the receiver. A nil Deposit is 0.
type FunctionCall struct {
	MethodName string
	Args       []byte
	Gas        uint64
	Deposit    *big.Int
}

// Transfer sends Deposit yoctoNEAR to the receiver.
type Transfer struct{ Deposit *big.Int }

// AddKey adds an access key to the receiver. A nil Permission is full
// access.
type AddKey struct {
	PublicKey  PublicKey
	Nonce      uint64
	Permission *FunctionCallPermission
}

// FunctionCallPermission limits an access key to calls on ReceiverID. Empty
// MethodNames allows every method; a nil Allowance is unlimited.
type FunctionCallPermission struct {
	Allowance   *big.Int
	ReceiverID  string
	MethodNames []string
}

// Delegate executes a signed delegate action (NEP-366).
type Delegate struct{ SignedDelegateAction }

func (CreateAccount) encode(e *encoder) { e.u8(tagCreateAccount) }

func (a DeployContract) encode(e *encoder) {
	e.u8(tagDeployContract)
	e.bytes(a.Code)
}

func (a FunctionCall) encode(e *encoder) {
	e.u8(tagFunctionCall)
	e.string(a.MethodName)
	e.bytes(a.Args)
	e.u64(a.Gas)
	e.u128(a.Deposit)
}

func (a Transfer) encode(e *encoder) {
	e.u8(tagTransfer)
	e.u128(a.Deposit)
}

func (a AddKey) encode(e *encoder) {
	e.u8(tagAddKey)
	e.publicKey(a.PublicKey)
	e.u64(a.Nonce)
	if a.Permission == nil {
		e.u8(1) // FullAccess
		return
	}
	e.u8(0) // FunctionCall
	if a.Permission.Allowance == nil {
		e.u8(0)
	} else {
		e.u8(1)
		e.u128(a.Permission.Allowance)
	}
	e.string(a.Permission.ReceiverID)
	e.u32(uint32(len(a.Permission.MethodNames)))
	for _, m := range a.Permission.MethodNames {
		e.string(m)
	}
}

func (a Delegate) encode(e *encoder) {
	e.u8(tagDelegate)
	a.SignedDelegateAction.encode(e)
}

// Transaction is a version 0 NEAR transaction.
type Transaction struct {
	SignerID   string
	PublicKey  PublicKey
	Nonce      uint64
	ReceiverID string
	BlockHash  [32]byte
	Actions    []Action
}

// Encode returns the Borsh encoding of t.
func (t Transaction) Encode() []byte {
	var e encoder
	t.encode(&e)
	return e.buf
}

func (t Transaction) encode(e *encoder) {
	e.string(t.SignerID)
	e.publicKey(t.PublicKey)
	e.u64(t.Nonce)
	e.string(t.ReceiverID)
	e.raw(t.BlockHash[:])
	e.u32(uint32(len(t.Actions)))
	for _, a := range t.Actions {
		a.encode(e)
	}
}

// Hash is the transaction hash: sha256 of its encoding. It is what the
// signer signs and what explorers show (in base58).
func (t Transaction) Hash() [32]byte { return sha256.Sum256(t.Encode()) }

// SignedTransaction is a transaction with its signature.
type SignedTransaction struct {
	Transaction Transaction
	Signature   Signature
}

// Sign signs t with key.
func Sign(t Transaction, key ed25519.PrivateKey) SignedTransaction {
	hash := t.Hash()
	var sig Signature
	copy(sig[:], ed25519.Sign(key, hash[:]))
	return SignedTransaction{Transaction: t, Signature: sig}
}

// Encode returns the Borsh encoding sent to send_tx (base64 encoded).
func (s SignedTransaction) Encode() []byte {
	var e encoder
	s.Transaction.encode(&e)
	e.signature(s.Signature)
	return e.buf
}

// Hash returns the transaction hash in base58, as RPC and explorers show it.
func (s SignedTransaction) Hash() string {
	h := s.Transaction.Hash()
	return base58.Encode(h[:])
}

// DelegateAction is a NEP-366 delegate action: actions SenderID signs for a
// relayer to submit and pay for.
type DelegateAction struct {
	SenderID   string
	ReceiverID string
	// Actions never contains a Delegate.
	Actions        []Action
	Nonce          uint64
	MaxBlockHeight uint64
	PublicKey      PublicKey
}

func (d DelegateAction) encode(e *encoder) {
	e.string(d.SenderID)
	e.string(d.ReceiverID)
	e.u32(uint32(len(d.Actions)))
	for _, a := range d.Actions {
		a.encode(e)
	}
	e.u64(d.Nonce)
	e.u64(d.MaxBlockHeight)
	e.publicKey(d.PublicKey)
}

// Encode returns the Borsh encoding of d.
func (d DelegateAction) Encode() []byte {
	var e encoder
	d.encode(&e)
	return e.buf
}

// SigningHash is the hash the sender signs: sha256 of the NEP-461 prefix
// 2^30 + 366 followed by the encoding of d.
func (d DelegateAction) SigningHash() [32]byte {
	var e encoder
	e.u32(delegateDiscriminant)
	d.encode(&e)
	return sha256.Sum256(e.buf)
}

// SignedDelegateAction is a delegate action with the sender's signature.
type SignedDelegateAction struct {
	DelegateAction DelegateAction
	Signature      Signature
}

// SignDelegate signs d with key.
func SignDelegate(d DelegateAction, key ed25519.PrivateKey) SignedDelegateAction {
	hash := d.SigningHash()
	var sig Signature
	copy(sig[:], ed25519.Sign(key, hash[:]))
	return SignedDelegateAction{DelegateAction: d, Signature: sig}
}

func (s SignedDelegateAction) encode(e *encoder) {
	s.DelegateAction.encode(e)
	e.signature(s.Signature)
}

// Encode returns the Borsh encoding of s, as wallets return it.
func (s SignedDelegateAction) Encode() []byte {
	var e encoder
	s.encode(&e)
	return e.buf
}

// Verify reports whether the signature is the delegate action's public key's
// signature of it. The chain checks again that the key belongs to the sender.
func (s SignedDelegateAction) Verify() bool {
	hash := s.DelegateAction.SigningHash()
	return ed25519.Verify(s.DelegateAction.PublicKey[:], hash[:], s.Signature[:])
}

// ErrMalformed is returned for encodings that cannot be decoded.
var ErrMalformed = errors.New("neartx: malformed encoding")

// maxDecodeLen bounds lengths read while decoding, so a crafted length
// cannot allocate much memory.
const maxDecodeLen = 1 << 16

// DecodeSignedDelegateAction decodes a Borsh SignedDelegateAction. Only the
// actions this package supports are accepted, ed25519 keys only, and the
// input must be consumed exactly.
func DecodeSignedDelegateAction(b []byte) (SignedDelegateAction, error) {
	d := decoder{buf: b}
	var s SignedDelegateAction
	s.DelegateAction.SenderID = d.string()
	s.DelegateAction.ReceiverID = d.string()
	n := d.length()
	for i := 0; i < n && d.err == nil; i++ {
		s.DelegateAction.Actions = append(s.DelegateAction.Actions, d.action())
	}
	s.DelegateAction.Nonce = d.u64()
	s.DelegateAction.MaxBlockHeight = d.u64()
	s.DelegateAction.PublicKey = d.publicKey()
	s.Signature = d.signature()
	if d.err != nil {
		return SignedDelegateAction{}, d.err
	}
	if d.pos != len(d.buf) {
		return SignedDelegateAction{}, fmt.Errorf("%w: %d trailing bytes", ErrMalformed, len(d.buf)-d.pos)
	}
	return s, nil
}

type encoder struct{ buf []byte }

func (e *encoder) u8(v uint8)      { e.buf = append(e.buf, v) }
func (e *encoder) u32(v uint32)    { e.buf = binary.LittleEndian.AppendUint32(e.buf, v) }
func (e *encoder) u64(v uint64)    { e.buf = binary.LittleEndian.AppendUint64(e.buf, v) }
func (e *encoder) raw(b []byte)    { e.buf = append(e.buf, b...) }
func (e *encoder) string(s string) { e.u32(uint32(len(s))); e.buf = append(e.buf, s...) }
func (e *encoder) bytes(b []byte)  { e.u32(uint32(len(b))); e.raw(b) }

// u128 writes v (nil is 0) as 16 little-endian bytes. v must fit in u128.
func (e *encoder) u128(v *big.Int) {
	var b [16]byte
	if v != nil {
		v.FillBytes(b[:]) // big-endian
		for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
			b[i], b[j] = b[j], b[i]
		}
	}
	e.raw(b[:])
}

func (e *encoder) publicKey(pk PublicKey) {
	e.u8(keyTypeED25519)
	e.raw(pk[:])
}

func (e *encoder) signature(s Signature) {
	e.u8(keyTypeED25519)
	e.raw(s[:])
}

type decoder struct {
	buf []byte
	pos int
	err error
}

func (d *decoder) fail(msg string) {
	if d.err == nil {
		d.err = fmt.Errorf("%w: %s", ErrMalformed, msg)
	}
}

func (d *decoder) take(n int) []byte {
	if d.err != nil {
		return nil
	}
	if n < 0 || len(d.buf)-d.pos < n {
		d.fail("unexpected end of input")
		return nil
	}
	b := d.buf[d.pos : d.pos+n]
	d.pos += n
	return b
}

func (d *decoder) u8() uint8 {
	b := d.take(1)
	if b == nil {
		return 0
	}
	return b[0]
}

func (d *decoder) u32() uint32 {
	b := d.take(4)
	if b == nil {
		return 0
	}
	return binary.LittleEndian.Uint32(b)
}

func (d *decoder) u64() uint64 {
	b := d.take(8)
	if b == nil {
		return 0
	}
	return binary.LittleEndian.Uint64(b)
}

func (d *decoder) u128() *big.Int {
	b := d.take(16)
	if b == nil {
		return new(big.Int)
	}
	be := make([]byte, 16)
	for i := range b {
		be[15-i] = b[i]
	}
	return new(big.Int).SetBytes(be)
}

func (d *decoder) length() int {
	n := d.u32()
	if n > maxDecodeLen {
		d.fail("length too large")
		return 0
	}
	return int(n)
}

func (d *decoder) bytes() []byte {
	b := d.take(d.length())
	return append([]byte(nil), b...)
}

func (d *decoder) string() string { return string(d.take(d.length())) }

func (d *decoder) publicKey() PublicKey {
	var pk PublicKey
	if d.u8() != keyTypeED25519 {
		d.fail("only ed25519 keys are supported")
		return pk
	}
	copy(pk[:], d.take(len(pk)))
	return pk
}

func (d *decoder) signature() Signature {
	var s Signature
	if d.u8() != keyTypeED25519 {
		d.fail("only ed25519 signatures are supported")
		return s
	}
	copy(s[:], d.take(len(s)))
	return s
}

func (d *decoder) action() Action {
	switch tag := d.u8(); tag {
	case tagCreateAccount:
		return CreateAccount{}
	case tagDeployContract:
		return DeployContract{Code: d.bytes()}
	case tagFunctionCall:
		return FunctionCall{MethodName: d.string(), Args: d.bytes(), Gas: d.u64(), Deposit: d.u128()}
	case tagTransfer:
		return Transfer{Deposit: d.u128()}
	case tagAddKey:
		a := AddKey{PublicKey: d.publicKey(), Nonce: d.u64()}
		switch d.u8() {
		case 0:
			p := &FunctionCallPermission{}
			switch d.u8() {
			case 0:
			case 1:
				p.Allowance = d.u128()
			default:
				d.fail("invalid option tag")
			}
			p.ReceiverID = d.string()
			n := d.length()
			for i := 0; i < n && d.err == nil; i++ {
				p.MethodNames = append(p.MethodNames, d.string())
			}
			a.Permission = p
		case 1:
		default:
			d.fail("unsupported access key permission")
		}
		return a
	default:
		// Delegate is not allowed inside a delegate action; other actions are
		// not supported.
		d.fail(fmt.Sprintf("unsupported action %d", tag))
		return nil
	}
}
