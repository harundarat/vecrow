//go:build e2e

package e2e_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/harundarat/vecrow/backend/internal/infrastructure/nearrpc"
	"github.com/harundarat/vecrow/backend/internal/neartx"
)

// sandbox is a local nearcore node started from contracts/: the binary
// (and fd-limit shim) that `make test-e2e` passes in NEAR_SANDBOX_BIN_PATH.
type sandbox struct {
	url  string
	rpc  *nearrpc.Client
	root *nearrpc.Signer
}

// startSandbox initializes a fresh node in a temporary home and runs it
// until the test ends. Its genesis account test.near signs the setup.
func startSandbox(t *testing.T) *sandbox {
	t.Helper()
	bin := os.Getenv("NEAR_SANDBOX_BIN_PATH")
	if bin == "" {
		t.Fatal("NEAR_SANDBOX_BIN_PATH is not set; run `make test-e2e`")
	}
	home := t.TempDir()
	if out, err := exec.Command(bin, "--home", home, "init", "--fast").CombinedOutput(); err != nil {
		t.Fatalf("sandbox init: %v\n%s", err, out)
	}
	// As near-workspaces does: fewer open files, larger RPC payloads.
	patchJSON(t, filepath.Join(home, "config.json"), func(cfg map[string]any) {
		cfg["store"].(map[string]any)["max_open_files"] = 3000
		cfg["rpc"].(map[string]any)["limits_config"] = map[string]any{"json_payload_max_size": 64 << 20}
	})

	rpcAddr, netAddr := freeAddr(t), freeAddr(t)
	logFile, err := os.Create(filepath.Join(home, "sandbox.log"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "--home", home, "run", "--rpc-addr", rpcAddr, "--network-addr", netAddr)
	cmd.Stdout, cmd.Stderr = logFile, logFile
	if err := cmd.Start(); err != nil {
		t.Fatalf("start sandbox: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		_ = logFile.Close()
	})

	url := "http://" + rpcAddr
	deadline := time.Now().Add(90 * time.Second)
	for {
		resp, err := http.Get(url + "/status")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		if time.Now().After(deadline) {
			log, _ := os.ReadFile(logFile.Name())
			t.Fatalf("sandbox did not start: %v\n%s", err, tail(log, 4000))
		}
		time.Sleep(250 * time.Millisecond)
	}

	var validator struct {
		AccountID string `json:"account_id"`
		SecretKey string `json:"secret_key"`
	}
	readJSON(t, filepath.Join(home, "validator_key.json"), &validator)
	key, err := neartx.ParsePrivateKey(validator.SecretKey)
	if err != nil {
		t.Fatalf("validator key: %v", err)
	}
	rpc := nearrpc.New(url, 30*time.Second)
	s := &sandbox{url: url, rpc: rpc}
	s.root = s.signer(validator.AccountID, key)
	return s
}

func (s *sandbox) signer(accountID string, key ed25519.PrivateKey) *nearrpc.Signer {
	return nearrpc.NewSigner(s.rpc, accountID, key, nearrpc.NewLocalNonces())
}

// send signs and sends a transaction and fails the test if the transaction
// or any of its receipts failed.
func (s *sandbox) send(t *testing.T, from *nearrpc.Signer, receiver string, actions ...neartx.Action) *nearrpc.Outcome {
	t.Helper()
	out, err := from.Send(context.Background(), receiver, actions...)
	if err != nil {
		t.Fatalf("%s → %s: %v", from.AccountID(), receiver, err)
	}
	for _, o := range append([]nearrpc.ExecutionOutcome{out.TransactionOutcome}, out.ReceiptsOutcome...) {
		if f := o.Outcome.Status.Failure; len(f) > 0 {
			t.Fatalf("%s → %s failed in %s: %s", from.AccountID(), receiver, o.Outcome.ExecutorID, nearrpc.FailureMessage(f))
		}
	}
	return out
}

// call calls a contract method with JSON args.
func (s *sandbox) call(t *testing.T, from *nearrpc.Signer, contract, method string, args any, deposit *big.Int) *nearrpc.Outcome {
	t.Helper()
	return s.send(t, from, contract, neartx.FunctionCall{
		MethodName: method, Args: mustJSON(t, args), Gas: 300 * neartx.TGas, Deposit: deposit,
	})
}

// view calls a view method and decodes its JSON result into out.
func (s *sandbox) view(t *testing.T, contract, method string, args any, out any) {
	t.Helper()
	b, _, err := s.rpc.CallFunction(context.Background(), contract, method, mustJSON(t, args))
	if err != nil {
		t.Fatalf("view %s.%s: %v", contract, method, err)
	}
	if err := json.Unmarshal(b, out); err != nil {
		t.Fatalf("view %s.%s: decode %s: %v", contract, method, b, err)
	}
}

// createAccount creates a sub-account of the root with balance and a full
// access key, then applies extra actions (deploy, init) in the same
// transaction.
func (s *sandbox) createAccount(t *testing.T, accountID string, balance *big.Int, key ed25519.PrivateKey, extra ...neartx.Action) *nearrpc.Signer {
	t.Helper()
	actions := []neartx.Action{neartx.CreateAccount{}}
	if balance.Sign() > 0 {
		actions = append(actions, neartx.Transfer{Deposit: balance})
	}
	actions = append(actions, neartx.AddKey{PublicKey: neartx.PublicKeyOf(key)})
	s.send(t, s.root, accountID, append(actions, extra...)...)
	return s.signer(accountID, key)
}

// nearBalance returns an account's NEAR balance in yoctoNEAR.
func (s *sandbox) nearBalance(t *testing.T, accountID string) *big.Int {
	t.Helper()
	var res struct {
		Result struct {
			Amount string `json:"amount"`
		} `json:"result"`
		Error json.RawMessage `json:"error"`
	}
	body := mustJSON(t, map[string]any{
		"jsonrpc": "2.0", "id": "e2e", "method": "query",
		"params": map[string]string{"request_type": "view_account", "finality": "final", "account_id": accountID},
	})
	resp, err := http.Post(s.url, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil || len(res.Error) > 0 {
		t.Fatalf("view_account %s: %v %s", accountID, err, res.Error)
	}
	n, ok := new(big.Int).SetString(res.Result.Amount, 10)
	if !ok {
		t.Fatalf("view_account %s: amount %q", accountID, res.Result.Amount)
	}
	return n
}

// delegate signs a NEP-366 delegate action from accountID to call method on
// contract, as a NEAR wallet does, and returns it as base64 Borsh.
func (s *sandbox) delegate(t *testing.T, accountID string, key ed25519.PrivateKey, contract, method string, args any, gas uint64, deposit *big.Int) string {
	t.Helper()
	ctx := context.Background()
	pub := neartx.PublicKeyOf(key)
	nonce, err := s.rpc.AccessKeyNonce(ctx, accountID, pub.String())
	if err != nil {
		t.Fatalf("access key nonce of %s: %v", accountID, err)
	}
	block, err := s.rpc.FinalBlock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	d := neartx.DelegateAction{
		SenderID:   accountID,
		ReceiverID: contract,
		Actions: []neartx.Action{neartx.FunctionCall{
			MethodName: method, Args: mustJSON(t, args), Gas: gas, Deposit: deposit,
		}},
		Nonce:          nonce + 1,
		MaxBlockHeight: block.Height + 100,
		PublicKey:      pub,
	}
	return base64.StdEncoding.EncodeToString(neartx.SignDelegate(d, key).Encode())
}

// near returns n NEAR in yoctoNEAR.
func near(n int64) *big.Int {
	return new(big.Int).Mul(big.NewInt(n), new(big.Int).Exp(big.NewInt(10), big.NewInt(24), nil))
}

func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return "127.0.0.1:" + strconv.Itoa(l.Addr().(*net.TCPAddr).Port)
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func readJSON(t *testing.T, path string, out any) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, out); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}

func patchJSON(t *testing.T, path string, patch func(map[string]any)) {
	t.Helper()
	var m map[string]any
	readJSON(t, path, &m)
	patch(m)
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
}

func tail(b []byte, n int) string {
	if len(b) > n {
		b = b[len(b)-n:]
	}
	return string(b)
}

// wasm reads a contract built by `make -C contracts build`.
func wasm(t *testing.T, dir, file string) []byte {
	t.Helper()
	base := os.Getenv("VECROW_WASM_DIR")
	if base == "" {
		base = filepath.Join("..", "..", "contracts", "target", "near")
	}
	b, err := os.ReadFile(filepath.Join(base, dir, file))
	if err != nil {
		t.Fatalf("%v: run `make -C contracts build` first", err)
	}
	return b
}

func usdc(whole int64) *big.Int { return new(big.Int).Mul(big.NewInt(whole), big.NewInt(1_000_000)) }
