# Vecrow — Architecture

How Vecrow is built. For what it does, see [`spec.md`](./spec.md); for why it is built
this way, see [`decisions.md`](./decisions.md).

## 1. Overview

Vecrow has three parts:

| Part | Responsibility | Holds funds? |
| --- | --- | --- |
| **Escrow contract** (NEAR) | Holds USDC per escrow, verifies the terms both parties signed, enforces milestone rules, verifies signed approvals, pays fee and split | Yes — the only component that does |
| **Backend** | Escrows until they are on-chain (terms, submissions, creator approval) and creating them with `create_escrow`, 1Click quotes and funding watcher, meta transaction relayer, storage registration, scheduled jobs, accounts and CSV export | No (see D-09 for the fallback exception) |
| **Frontend** | Client escrow page, freelancer dashboard, login, profile, "My escrows" | No |

Funds can leave the contract only through contract methods: approval, auto-release,
freelancer refund, or `retry_payout`. An escrow goes on-chain only when both parties
have signed its terms, and the contract checks both signatures itself. The backend
pays gas and `storage_deposit` in NEAR, but never decides where USDC goes or who
approves.

## 2. Repository layout

```text
.
├── contracts/   # Rust, near-sdk — the escrow contract and its tests
├── backend/     # Go — REST API, relayer, 1Click watcher, jobs
├── frontend/    # React + Vite — escrow pages, dashboard, account pages
└── docs/        # spec.md, architecture.md, decisions.md
```

## 3. Components

```mermaid
flowchart TB
    subgraph FE["Frontend (React + Vite + NEAR Wallet Selector)"]
        PAY["Client escrow page"]
        DASH["Freelancer dashboard"]
        ACC["Login, profile, My escrows"]
    end
    subgraph BE["Backend (Go + Gin + PostgreSQL + Redis + RabbitMQ)"]
        API["REST API + auth<br/>(wallet session, guest token, email OTP)"]
        Q["1Click quote + watcher"]
        REL["Meta transaction relayer"]
        JOB["Jobs: create_escrow, auto_release, retry_payout"]
        REG["storage_deposit registration"]
    end
    subgraph NEAR["NEAR"]
        ESC["Vecrow escrow contract"]
        USDC["USDC contract (NEP-141)"]
    end
    ONE["1Click API (NEAR Intents)"]

    PAY --> API
    DASH --> API
    ACC --> API
    API --> Q --> ONE
    API --> REL --> ESC
    JOB --> REL
    JOB --> ESC
    REG --> USDC
    ONE -->|USDC| USDC -->|ft_on_transfer| ESC
    ESC -->|ft_transfer| USDC
```

| Layer | Technology | Notes |
| --- | --- | --- |
| Contract | Rust, near-sdk, cargo-near, near-workspaces | Unit and integration tests run against a mock NEP-141 token on a local sandbox / testnet |
| Backend | Go (Gin), PostgreSQL, Redis, RabbitMQ | 1Click is called over its REST API; NEAR transactions (relayer, `storage_deposit`, jobs) are sent over NEAR JSON-RPC. Jobs (escrow creation, funding watcher, auto-release, payout retry) are queued in RabbitMQ and processed by workers. Redis holds short-lived state with a TTL (login nonces, sessions, OTP rate limits). Layering in section 7.4 |
| Frontend | React, Vite, NEAR Wallet Selector | Single-page app that talks only to the backend API. EVM and Solana wallets are only needed for signing; paying is a transfer to a deposit address from any wallet |

## 4. Money flow

```mermaid
flowchart LR
    C["Client<br/>any chain + token"] -->|send to deposit address| D["1Click<br/>deposit address"]
    D --> S["NEAR Intents solver<br/>swap to USDC on NEAR"]
    S -->|"ft_transfer_call<br/>msg = escrow_id"| E["Vecrow escrow contract"]
    E -.->|"under / over payment<br/>(R-02)"| SD["sender_id"]
    E -->|"approve / auto_release<br/>fee 0.5%"| T["treasury_account"]
    E -->|"approve / auto_release<br/>remainder by bps"| M["Split recipients"]
    E -->|"refund_unsubmitted<br/>(R-18)"| R["Client refund_account"]
    E -.->|"ft_transfer failed<br/>on_payout"| P["pending_payout"]
    P -.->|retry_payout| E
```

Every arrow out of the contract is an `ft_transfer` on the USDC contract, followed by
the `on_payout` callback (section 6.4).

## 5. Escrow contract

The contract lives in [`contracts/`](../contracts/), a Cargo workspace with Rust 1.97.0
pinned in `rust-toolchain.toml`. It uses near-sdk 5.30 and is built to wasm with
cargo-near 0.23 (commands in the [README](../README.md#contract-1)).

| Crate | Contents |
| --- | --- |
| [`escrow`](../contracts/escrow/) (`vecrow-escrow`) | The contract, one module per flow, and its unit tests on near-sdk's mocked blockchain (`src/tests/`) |
| [`mock-ft`](../contracts/mock-ft/) | Mock NEP-141 token standing in for USDC: the standard token plus an open `mint`; accounts must be registered with `storage_deposit` |
| [`wallet-fixtures`](../contracts/wallet-fixtures/) | Test-only signers that reproduce the bytes NEAR (NEP-413), EVM (`personal_sign`) and Solana (`signMessage`) wallets sign, built with ed25519-dalek, k256 and borsh |
| [`integration-tests`](../contracts/integration-tests/) | near-workspaces tests that deploy the cargo-near wasm files to a local sandbox node |

### 5.1 Methods

| Method | Caller | Effect | Requirements |
| --- | --- | --- | --- |
| `create_escrow` | Relayer, with the freelancer's and the client's signatures | Verifies both signatures over the same terms; stores freelancer, milestones, split, approver, `refund_account`, review window, and `fee_bps`; status `Created` | R-01, R-03, R-04, R-19, R-23 |
| `cancel` | Freelancer (via relayer), or relayer with the approver's signature | `Created` → `Cancelled` | R-01 |
| `ft_on_transfer` | USDC contract | Applies the funding amount rules; `Created` → `Funded` | R-02 |
| `submit_milestone` | Freelancer (via relayer) | `Pending` / `RevisionRequested` → `Submitted`; starts the review window | R-05 |
| `approve_milestone` | Relayer, with the approver's signature | `Submitted` → `Released`; pays fee and split | R-06, R-19 |
| `request_revision` | Relayer, with the approver's signature | `Submitted` → `RevisionRequested`; increments `revision_count` (max 2) | R-17 |
| `auto_release` | Anyone (backend job) | `Submitted` → `Released` after the review window | R-07 |
| `refund_unsubmitted` | Freelancer (via relayer) | `Pending` → `Refunded`; pays `refund_account` | R-18 |
| `on_payout` | Contract callback | Records failed transfers in `pending_payout` | R-20 |
| `retry_payout` | Anyone (backend job) | Re-sends a pending payout | R-20 |
| `get_escrow` | Anyone (view) | Escrow, milestones, approver, pending payouts | — |

The escrow becomes `Completed` when every milestone is `Released` or `Refunded`.
Escrows that are not on-chain yet (`Open`, `PendingApproval`, `Approved`) exist only in
the backend (section 6.1); the contract never sees them. Besides these methods, the
contract has the `new` initializer, called once at deploy, and the NEP-330 view
`contract_source_metadata` that near-sdk adds to every contract.

Arguments and results, in JSON:

```text
new(usdc_contract_id: AccountId, treasury_account: AccountId, fee_bps: u16, review_window_sec: u64)
create_escrow(terms: EscrowTerms, freelancer_signature: {public_key: String, signature: String},
              client_signature: String)
cancel(escrow_id: String, nonce: String?, deadline_ms: u64?, signature: String?)   // signed fields: approver only
ft_on_transfer(sender_id: AccountId, amount: U128, msg: String) -> U128            // amount to give back
submit_milestone(escrow_id: String, milestone: u32)
approve_milestone(escrow_id: String, milestone: u32, nonce: String, deadline_ms: u64, signature: String)
request_revision(escrow_id: String, milestone: u32, nonce: String, deadline_ms: u64, signature: String)
auto_release(escrow_id: String, milestone: u32)
refund_unsubmitted(escrow_id: String, milestone: u32?)                             // omitted: every Pending milestone
on_payout(escrow_id: String, account_id: AccountId, amount: U128) -> bool          // #[private]
retry_payout(escrow_id: String, account_id: AccountId) -> bool                     // true if the transfer succeeded
get_escrow(escrow_id: String) -> EscrowView | null

EscrowTerms {
  escrow_id: String, freelancer: AccountId, milestones: [U128],
  split: [{account_id: AccountId, bps: u16}], approver: Approver,
  refund_account: AccountId, fee_bps: u16, deadline_ms: u64
}
```

- **No attached deposit.** No method is payable. Storage and the 1 yoctoNEAR that each
  `ft_transfer` requires are paid from the contract's own NEAR balance (D-19).
- **Who may call.** Freelancer methods (`submit_milestone`, `refund_unsubmitted`, and
  `cancel` without a signature) check `predecessor_account_id`, which a NEP-366 meta
  transaction keeps as the freelancer, so the relayer cannot act on its own. Signed
  methods (`create_escrow`, `approve_milestone`, `request_revision`, and `cancel` with a
  signature) check only the signatures; the caller gains no rights (D-02).
- **JSON conventions.** Amounts are USDC base units as `U128` strings. `milestone` is a
  0-based index. Times are milliseconds since the Unix epoch (`*_ms`), except
  `review_window_sec`. `Approver` is tagged by `scheme`, with the same scheme names as
  wallet login (§7.5):
  `{"scheme": "nep413", "account_id": "client.near", "public_key": "ed25519:<base58>"}`,
  `{"scheme": "eip191", "address": "0x<40 hex>"}`, or
  `{"scheme": "ed25519", "address": "<base58 public key>"}`. The freelancer's
  `public_key` is `ed25519:<base58>`, like a NEAR wallet returns it.
- **Gas.** A release (`approve_milestone`, `auto_release`) makes up to 11 payouts (the
  fee and 10 split recipients); each reserves 10 TGas for `ft_transfer` and 10 TGas for
  `on_payout`. Attach 300 TGas. The largest release burns about 48 TGas in the sandbox
  tests.

**Escrow ID.** The backend chooses the escrow ID when the escrow is proposed (`Open`),
for example a lowercase UUID, and `create_escrow` stores the escrow under it. It is 1 to
64 characters from `a-z`, `0-9`, `-` and `_`, so it has one spelling in the signed
messages and in the funding `msg` of `ft_transfer_call`. `create_escrow` rejects an ID
that is malformed or already used, including by a cancelled escrow. An escrow is
identified globally by the pair (contract account, escrow ID).

`get_escrow` returns the signed terms with the current state:

```json
{
  "escrow_id": "6f1c2a5e-8d3b-4c1e-9f00-1234567890ab",
  "freelancer": "alice.near",
  "status": "Funded",
  "milestones": [
    { "amount": "1000000000", "status": "Submitted", "submitted_at_ms": 1767225600000, "revision_count": 0 },
    { "amount": "500000000", "status": "Pending", "submitted_at_ms": null, "revision_count": 0 }
  ],
  "split": [{ "account_id": "alice.near", "bps": 7000 }, { "account_id": "bob.near", "bps": 3000 }],
  "fee_bps": 50,
  "review_window_sec": 1209600,
  "approver": { "scheme": "eip191", "address": "0x7e5f4552091a69125d5dfcb7b8c2659029395bdf" },
  "refund_account": "client.near",
  "total": "1500000000",
  "balance": "1500000000",
  "pending_payouts": [],
  "created_at_ms": 1767139200000,
  "funded_at_ms": 1767142800000
}
```

`balance` is the USDC still held for milestones that are neither released nor refunded.
`pending_payouts` lists released or refunded USDC whose transfer failed, per account.
A milestone can be auto-released from `submitted_at_ms + review_window_sec × 1000`.

### 5.2 State

| Item | Contents |
| --- | --- |
| Configuration | USDC contract ID, `treasury_account`, default `fee_bps`, review window (set at deploy, no setter; D-15) |
| Escrows | Freelancer, split, approver, `refund_account`, and locked `fee_bps` (all fixed by `create_escrow`); milestones (amount, status, last submit time, `revision_count`); status |
| Used nonces | Per approver, to reject replayed signatures |
| `pending_payout` | `escrow_id → account → amount` for transfers that failed, kept in each escrow record |

Storage layout (all values Borsh-serialized):

| Key | Value |
| --- | --- |
| `STATE` | `{ config: Config, escrows: prefix 0x00, used_nonces: prefix 0x01 }` |
| `0x00 ‖ borsh(escrow ID: String)` | `Escrow` |
| `0x01 ‖ sha256(borsh(ApproverKey) ‖ nonce)` | empty (set membership) |

```text
Config        { usdc_contract_id: String, treasury_account: String, fee_bps: u16, review_window_sec: u64 }
Escrow        { freelancer: String, milestones: Vec<Milestone>, split: Vec<SplitShare>, fee_bps: u16,
                review_window_sec: u64, approver: ApproverKey, refund_account: String,
                status: EscrowStatus, balance: u128, pending_payouts: Vec<PendingPayout>,
                created_at_ms: u64, funded_at_ms: Option<u64> }
Milestone     { amount: u128, status: MilestoneStatus, submitted_at_ms: Option<u64>, revision_count: u8 }
SplitShare    { account_id: String, bps: u16 }
PendingPayout { account_id: String, amount: u128 }
ApproverKey   0: Nep413 { account_id: String, public_key: [u8; 32] }
              1: Eip191 { address: [u8; 20] }
              2: Ed25519 { public_key: [u8; 32] }
EscrowStatus    0 Created, 1 Funded, 2 Completed, 3 Cancelled
MilestoneStatus 0 Pending, 1 Submitted, 2 RevisionRequested, 3 Released, 4 Refunded
```

An escrow with 2 milestones, 2 recipients, a NEP-413 approver and a UUID as ID takes
about 300 bytes of storage (0.003 NEAR); each used nonce takes 73 bytes. Limits of
20 milestones and 10 split recipients per escrow bound both storage and the gas of a
release.

### 5.3 Signature verification

The contract verifies two kinds of signed payload:

- **Escrow terms** (`create_escrow`): escrow ID, freelancer, milestones, split,
  approver scheme and address, `refund_account`, `fee_bps`, and a deadline. Both
  parties sign the same terms: the freelancer with NEP-413 from their NEAR account, the
  client with the approver's scheme. The backend builds the message, so both
  signatures cover the same bytes. The contract rejects an expired deadline and an
  escrow ID that already exists, so signed terms cannot be replayed.
- **Actions** (approve, request revision, cancel by the client): escrow ID, milestone
  (for approve and revision), action, nonce, and deadline, signed by the approver. The contract rejects reused
  nonces and expired deadlines.

Each signature is checked for its scheme:

| Scheme | Wallets | Verification |
| --- | --- | --- |
| NEP-413 | NEAR wallets | ed25519 over the NEP-413 message format, against the public key given with the signature (D-16) |
| EIP-191 | MetaMask and other EVM wallets | keccak256 of the prefixed message, `ecrecover` (high-s rejected), compare with the approver address |
| ed25519 | Phantom and other Solana wallets | ed25519 over the raw message bytes |

**Messages.** Every payload is UTF-8 text built from the call arguments. Lines are
separated by `\n` (0x0A); there is no trailing newline and no other whitespace. The
contract rebuilds the text and verifies the signatures over it, so the backend and the
frontend must produce it byte for byte (D-17). `wallet-fixtures` reproduces these bytes
for the tests.

Escrow terms, signed by both parties for `create_escrow`:

```text
Vecrow create_escrow
Contract: {contract account ID}
Escrow: {escrow ID}
Freelancer: {freelancer}
Milestones: {amount},{amount},…
Split: {account}:{bps},{account}:{bps},…
Approver: {scheme}:{address}
Refund account: {refund_account}
Fee: {fee_bps} bps
Deadline: {deadline_ms}
```

Approver actions:

```text
Vecrow {approve_milestone | request_revision}
Contract: {contract account ID}
Escrow: {escrow ID}
Milestone: {milestone}
Nonce: {nonce}
Deadline: {deadline_ms}
```

```text
Vecrow cancel
Contract: {contract account ID}
Escrow: {escrow ID}
Nonce: {nonce}
Deadline: {deadline_ms}
```

| Field | Format |
| --- | --- |
| Contract account ID | The escrow contract, e.g. `escrow.vecrow.testnet` |
| Escrow ID | As in §5.1 |
| Milestones | Amounts in USDC base units, decimal, in milestone order, separated by `,` |
| Split | `{account}:{bps}` in split order (the first recipient gets the rounding remainder), separated by `,` |
| `{scheme}:{address}` | `nep413:{account_id}`, `eip191:0x{40 lowercase hex}`, or `ed25519:{base58 public key}` |
| `fee_bps` | Decimal; must equal the configuration (R-19) |
| `milestone` | 0-based index, decimal |
| `nonce` | 32 random bytes as 64 lowercase hex characters; uppercase is rejected. The same string is the `nonce` argument |
| `deadline_ms` | Milliseconds since the Unix epoch, decimal. Valid while the block time is at most the deadline |

What each wallet signs over that `message`:

| Scheme | Wallet call | Signed bytes | Signature argument |
| --- | --- | --- | --- |
| `nep413` | `signMessage({message, nonce, recipient})` with `recipient` = the contract account ID, no `callbackUrl`, and `nonce` = the 32 action nonce bytes, or `sha256(message)` for terms | `sha256(9d 01 00 80 ‖ u32_le(len(message)) ‖ message ‖ nonce[32] ‖ u32_le(len(recipient)) ‖ recipient ‖ 00)`: the Borsh tag 2^31 + 413, then the Borsh payload with `callbackUrl = None` | base64, 64 bytes |
| `eip191` | `personal_sign(message)` | `keccak256("\x19Ethereum Signed Message:\n" ‖ decimal byte length of message ‖ message)` | hex of `r ‖ s ‖ v` (65 bytes, `0x` optional), `v` = 27/28 or 0/1 |
| `ed25519` | `signMessage(utf8 bytes of message)` | `message` | base58, 64 bytes |

The freelancer always signs the terms with NEP-413, passing the `public_key` the wallet
returns. The client signs them with the approver wallet named in the terms, which also
proves that they control it (R-04). For EIP-191, the address recovered with `ecrecover`
(last 20 bytes of the keccak256 of the 64-byte public key) must equal the approver
address. For example, approving milestone 3 of escrow `e12` on `escrow.test.near` with
nonce `ff…ff` and deadline 42 means signing:

```text
Vecrow approve_milestone
Contract: escrow.test.near
Escrow: e12
Milestone: 3
Nonce: ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff
Deadline: 42
```

`create_escrow` checks the escrow ID, the deadline, the fee, the milestone and split
rules, the approver and refund-account rules (D-18), then both signatures. Actions check
the escrow and milestone status, the deadline, the approver's signature, and that this
approver has not used the nonce before (for any escrow or action); then the nonce is
recorded. A rejected call is reverted, so it uses up nothing. The contract ID (also the
NEP-413 recipient) prevents replay on another deployment, the escrow ID and milestone
pin the target, and the first line keeps terms, approvals, revisions and cancellations
apart.

### 5.4 Payouts

Every transfer out of the contract is an `ft_transfer(receiver_id, amount, memo:
"vecrow escrow {escrow ID}")` on the USDC contract with 1 yoctoNEAR attached, followed by
`on_payout(escrow_id, account_id, amount)` (section 6.4). A release pays
`fee = floor(amount × fee_bps / 10,000)` to `treasury_account` and splits the rest by
bps, with the rounding remainder to the first recipient; the contract asserts that the
fee and the shares add up to the milestone amount. Each recipient gets its own transfer,
zero amounts are skipped (NEP-141 rejects them), and a refund is one transfer of the sum
of the refunded milestones, without a fee (D-19).

### 5.5 Events

The contract logs NEP-297 events, `EVENT_JSON:{"standard":"vecrow","version":"1.0.0","event":…,"data":{…}}`,
that the backend can index for the dashboard and the CSV export:

| Event | Data |
| --- | --- |
| `escrow_created` | `escrow_id`, `freelancer`, `approver`, `refund_account`, `total` |
| `escrow_cancelled` | `escrow_id`, `by` (`freelancer` or `approver`) |
| `escrow_funded` | `escrow_id`, `sender_id`, `amount` |
| `milestone_submitted` | `escrow_id`, `milestone`, `submitted_at_ms` |
| `revision_requested` | `escrow_id`, `milestone`, `revision_count` |
| `milestone_released` | `escrow_id`, `milestone`, `amount`, `fee`, `by` (`approve_milestone` or `auto_release`) |
| `milestones_refunded` | `escrow_id`, `milestones`, `amount`, `refund_account` |
| `payout` | `escrow_id`, `account_id`, `amount`, `paid` (false: now pending) |
| `escrow_completed` | `escrow_id` |

A returned deposit is logged as text: `returning {amount} to {sender_id}: {reason}`.

## 6. Key flows

### 6.1 Escrow proposal and creation on-chain

```mermaid
sequenceDiagram
    participant C as Creator (freelancer or client)
    participant O as Other party (link page)
    participant B as Backend
    participant U as USDC contract
    participant E as Escrow contract

    C->>B: POST /escrows (role, milestones, own details)
    B-->>C: escrow link (status Open)
    C-->>O: share the link
    O->>B: POST /escrows/:id/terms (own details)
    B-->>O: final terms + message to sign
    O->>O: sign the terms
    O->>B: POST /escrows/:id/counterparty (details + signature)
    B-->>O: guest token (status PendingApproval)
    C->>B: GET /escrows/:id (submission + same message)
    C->>C: sign the same terms
    C->>B: POST /escrows/:id/counterparty/approve (signature)
    Note over B: status Approved, job queued
    B->>U: storage_deposit(split recipients, refund_account) if needed
    B->>E: create_escrow(terms, freelancer signature, client signature) via relayer
    E->>E: verify both signatures, deadline, rules
    B-->>C: status Created
```

The details each party adds depend on their role, not on who created the escrow: the
freelancer always provides their NEAR account and the split, and the client always
provides the approver wallet and `refund_account`. If the creator rejects the
submission, or its deadline passes, the escrow returns to `Open` and the same link can
be used again. If `create_escrow` is rejected, for example because the deadline passed
before the job ran, the escrow also returns to `Open`.

### 6.2 Funding from another chain

Funding starts once the escrow is `Created` (section 6.1). The client proves the
approver wallet with the guest token from joining (R-21) or with a session that
includes that wallet.

```mermaid
sequenceDiagram
    participant C as Client (approver wallet)
    participant B as Backend
    participant O as 1Click
    participant U as USDC contract
    participant E as Escrow contract

    C->>B: POST /escrows/:id/quote (guest token or session)
    B->>O: quote EXACT_OUTPUT, recipient = contract, customRecipientMsg = escrow_id
    O-->>B: deposit address
    B-->>C: deposit address
    C->>O: send any token from any chain
    O->>U: ft_transfer_call(contract, total, msg = escrow_id)
    U->>E: ft_on_transfer
    E-->>U: unused amount (returned to sender_id)
    loop polling
        C->>B: GET /escrows/:id/funding
        B->>O: status
    end
    B-->>C: SUCCESS, escrow Funded
```

If the mainnet test shows `customRecipientMsg` is not reliable, 1Click delivers to a
backend account instead, and the backend calls `ft_transfer_call` itself (D-09).

### 6.3 Gasless submit and approve

```mermaid
sequenceDiagram
    participant F as Freelancer
    participant A as Approver wallet
    participant B as Backend relayer
    participant E as Escrow contract

    F->>B: POST /milestones/:n/submit (signed delegate action)
    B->>E: submit_milestone (relayer pays gas)
    A->>A: sign approve payload (nonce, deadline)
    A->>B: POST /milestones/:n/approve (payload + signature)
    B->>E: approve_milestone(payload, signature)
    E->>E: verify signature, nonce, deadline
    E->>E: pay fee + split (section 6.4)
```

The relayer only submits transactions whose receiver is the Vecrow contract. It does
not sign approvals or terms, and cannot change a payload without invalidating the
signature.

### 6.4 Payout with retry

```mermaid
sequenceDiagram
    participant E as Escrow contract
    participant U as USDC contract
    participant J as Backend job

    E->>U: ft_transfer(treasury, fee)
    E->>U: ft_transfer(recipient_i, share_i)
    U-->>E: on_payout(result)
    alt transfer failed
        E->>E: pending_payout[escrow][account] += amount
        J->>E: retry_payout(escrow_id, account)
        E->>E: set pending to 0, then ft_transfer
        U-->>E: on_payout(result)
        E->>E: on failure, restore pending amount
    end
```

Zeroing the pending amount before sending, and restoring it only on failure, means
two retries in a row cannot pay twice. `retry_payout` returns whether the transfer
succeeded, so the job can tell whether the account is registered yet.

## 7. Backend

### 7.1 Modules

| Module | Responsibility | Requirements |
| --- | --- | --- |
| API | REST endpoints listed in `spec.md` section 7 | — |
| Auth | Wallet login (nonce + signature), sessions, guest tokens, email OTP recovery | R-13, R-14, R-21, R-22 |
| Escrows | Escrows until they are on-chain (terms, the other party's submission, creator approval or rejection), invitations, role queries, revision notes | R-15, R-16, R-17, R-23 |
| 1Click | Token list, quotes, funding status watcher | R-08 |
| Relayer | Forwards signed actions and approved escrow terms to the Vecrow contract and pays gas | R-09, R-23 |
| Storage | `storage_balance_of` checks and `storage_deposit` registration | R-20 |
| Jobs | `create_escrow` for approved escrows, `auto_release` when the window ends, `retry_payout` for pending payouts | R-07, R-20, R-23 |
| Export | CSV export, one row per payout, fee as its own row | R-12 |

### 7.2 Data

PostgreSQL stores what the contract does not need, or does not have yet: escrows
before they go on-chain (terms, the other party's submission, and both signatures),
accounts, linked wallets and emails, invitations, revision notes, 1Click quotes and
funding status, and an index of payouts and transaction hashes for the dashboard and
CSV export. Once an escrow is on-chain, the contract is the source of truth for its
balances and statuses; the backend reads them from the contract.

### 7.3 Jobs

| Job | Trigger | Action |
| --- | --- | --- |
| Escrow creation | The creator approved (`Approved`) | Ensures storage registration, then calls `create_escrow` with both signatures; marks the escrow `Created`, or returns it to `Open` if the contract rejects the terms |
| Funding watcher | After a quote | Polls 1Click until `SUCCESS` or `REFUNDED` |
| Auto-release | Review window ended for a `Submitted` milestone | Calls `auto_release` |
| Payout retry | `pending_payout` is non-empty | Ensures registration, then calls `retry_payout` |

A scheduler in the backend periodically finds due work in PostgreSQL (approved escrows
not yet on-chain, quotes still pending, milestones whose review window has ended,
pending payouts) and publishes a message per item to RabbitMQ. Workers consume the
messages and call 1Click or the contract. Failed messages are retried with a delay:
each topic has a retry queue whose TTL is `QUEUE_RETRY_DELAY`. After
`QUEUE_MAX_RETRIES` retries, or on an error marked permanent (such as a malformed
payload), the message moves to the topic's dead-letter queue.

Queue topology for a topic `T` (all durable):

- Exchange `vecrow.jobs` (direct) routes key `T` to queue `T`.
- Queue `T.retry` has `x-message-ttl=QUEUE_RETRY_DELAY` and dead-letters back to the exchange with key `T`.
- Queue `T.dlq` holds messages that failed permanently or exhausted their retries.

A failed message is re-published to `T.retry` with `x-attempt` + 1 and the original is
acked; when the TTL expires, RabbitMQ routes it back to `T`. If a handler hits its
timeout, the message is settled the same way. Changing `QUEUE_RETRY_DELAY` for an
existing topic requires deleting `T.retry` first, because RabbitMQ rejects re-declaring
a queue with different arguments.

All jobs are idempotent: running one twice has the same effect as running it once,
because the contract rejects calls that are not valid for the current state. When
`create_escrow` reports that the escrow ID already exists, the creation job reads it
with `get_escrow` and marks it `Created`. This makes RabbitMQ's at-least-once delivery
safe.

### 7.4 Code structure

The backend follows clean architecture with manual constructor injection (D-12).
Dependencies point inward:

| Layer | Package | Depends on |
| --- | --- | --- |
| Domain | `internal/domain` | Standard library only |
| Use cases | `internal/usecase/...` | Domain, and the ports (interfaces) each use case declares |
| Repositories | `internal/repository` | Use-case ports, generic infrastructure interfaces |
| Delivery | `internal/delivery/http` (Gin), `internal/delivery/worker` (queue consumers) | Use cases, through interfaces |
| Infrastructure | `internal/infrastructure/{database,cache,queue,logger}` | Driver libraries |
| Composition root | `internal/bootstrap`, `cmd/api`, `cmd/worker` | Everything; the only place concrete technologies are chosen |

Infrastructure packages are named after their role, not their technology.
`cache.Store`, `queue.Publisher` and `queue.Subscriber` are generic interfaces.
PostgreSQL, Redis and RabbitMQ appear only in the implementation files
(`database/postgres.go`, `cache/redis.go`, `queue/rabbitmq.go`) and in `bootstrap`.
One exception: `database.Querier` uses pgx types, because it must match the `DBTX`
interface that sqlc generates for pgx. Repositories built on sqlc therefore depend on
pgx too, and replacing PostgreSQL also means regenerating them.

Logging uses `log/slog`: JSON in production, text in development. Every record about an
HTTP request or a queue message carries its `request_id`, which is passed from the
`X-Request-ID` header into queue message headers. Errors are logged once, by the boundary
that handles them (HTTP access log, queue consumer, scheduler). Request bodies, query
strings, tokens, signatures and OTP codes are never logged; as a safety net, attributes
with sensitive keys are redacted.

### 7.5 Wallet login (R-13, R-14)

Login and wallet linking use the same challenge (D-13):

1. `POST /auth/nonce {scheme, address}` returns `{nonce, message, recipient, expires_at}`.
   `scheme` is `eip191` (EVM), `ed25519` (Solana) or `nep413` (NEAR). The nonce is valid
   for `AUTH_NONCE_TTL` and can be used once.
2. The wallet signs `message` exactly as returned. It is plain text:

   ```
   {AUTH_DOMAIN} wants you to sign in with your wallet.

   Wallet: {address}
   Nonce: {nonce}
   Expires: {expires_at}

   Signing is free and does not send a transaction.
   ```

3. `POST /auth/verify {nonce, signature, public_key?}` logs in and returns
   `{token, expires_at, new_account, account}`. The first login creates the account with a
   generated username (`user_xxxxxxxx`), which `PATCH /me` can change. With a session,
   `POST /me/wallets` takes the same body and links the wallet to the account instead.

| Scheme | Wallet call | `signature` | `public_key` |
| --- | --- | --- | --- |
| `eip191` | `personal_sign(message)` | hex, `0x` + 65 bytes | — |
| `ed25519` | `signMessage(utf8 bytes of message)` | base58, 64 bytes | — |
| `nep413` | `signMessage({message, nonce: hex-decoded nonce (32 bytes), recipient})`, no `callbackUrl` | base64, 64 bytes | `ed25519:<base58>` |

For NEAR, the backend also checks over RPC (`NEAR_RPC_URL`) that `public_key` is a full
access key of the account, because a NEAR account ID is a name, not a key. An implicit
account (64 hex characters) that is not yet on chain is accepted when it is the hex of
`public_key`, so users with a NEAR balance of 0 can log in.

The token is sent as `Authorization: Bearer <token>` to `/me` and `/auth/logout`. It is an
opaque random string; Redis stores only its SHA-256, with the `AUTH_SESSION_TTL` expiry.
A wallet belongs to at most one account (`UNIQUE (scheme, address)`), and the last wallet
of an account cannot be unlinked.

## 8. Trust boundaries

| Actor | Can | Cannot |
| --- | --- | --- |
| Freelancer | Create an escrow or join one, choose and sign their account and the split, approve or reject the client's submission when they created the escrow, cancel before funding, submit, refund `Pending` milestones | Approve milestones, choose or change the approver, take funds for itself outside the split |
| Client (approver) | Create an escrow or join one, choose and sign the approver and `refund_account`, approve or reject the freelancer's submission when they created the escrow, cancel before funding, approve, request up to 2 revisions | Choose the split, refund, change the approver after the escrow is on-chain, hold funds beyond the review window |
| Anyone | Call `auto_release` after the window, `retry_payout` | Change where funds go |
| Relayer | Pay gas for calls to the Vecrow contract, including `create_escrow` | Approve, forge or alter signed payloads or terms, call other contracts |
| Backend | Store escrows until they are on-chain, create on-chain the escrows both parties signed, register storage, run jobs, manage accounts | Change signed terms, create an escrow without both signatures, move USDC (except under the D-09 fallback) |
| Account / session | Show escrows, act as a login, start an escrow | Grant any right in the contract |

## 9. Networks and configuration

| Environment | Contract | Token | Funding |
| --- | --- | --- | --- |
| Local / testnet | Deployed from `contracts/` | Mock NEP-141 token | Direct `ft_transfer_call`; 1Click quotes with `dry: true` |
| Mainnet (demo) | Separate development account | USDC (contract ID from the 1Click token list) | 1Click with small amounts |

Configuration such as RPC URL, contract and treasury account IDs, the relayer and
backend keys, the PostgreSQL and RabbitMQ URLs, the 1Click API settings, the review
window, and the signing deadline for escrow terms is read from environment variables.
Every variable is listed in the `.env.example` file of `backend/` and `frontend/`
without real values; keys are never committed.
