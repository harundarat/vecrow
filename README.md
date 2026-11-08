# Vecrow — milestone escrow for cross-border teams

Built at NEAR HACKATHON (Nov 2026) · Track: Real-World Money (`realworld`)

## What it is

A client pays a freelancer or agency from any chain and token. Funds are held in an
escrow contract on NEAR and released per approved milestone as USDC, minus a 0.5%
platform fee, split automatically between team members. Freelancers and clients
never buy NEAR for gas.

## The problem

- Freelancers deliver work and do not get paid; clients do not want to pay upfront.
- Agencies receive one payment and split it by hand across members in different countries.
- Existing crypto escrows require the client and the freelancer to use the same chain.

## Try it (judges)

> TODO: live demo URL · demo video (X post URL)

1. Open `<demo-url>/new`, create an escrow with 2 milestones and a 70/30 split
   → you get a client payment link.
2. Open the link (no account needed), connect a wallet, sign once to bind it as
   approver and set a NEAR refund account, pick USDC on Base, send to the deposit
   address shown → status goes `ApproverBound` → `Funded`, with the NEAR transaction linked.
3. As the freelancer, click "Submit milestone 1" (the wallet holds 0 NEAR)
   → the relayer pays gas, milestone status = `Submitted`.
4. As the client, click "Request revision" → milestone status = `RevisionRequested`;
   the freelancer submits again → `Submitted`.
5. As the client, click "Approve" → the treasury receives 0.5% and both team accounts
   receive their share; transaction links are shown on the dashboard.
6. As the freelancer, click "Refund" on milestone 2 → USDC returns to the client's
   refund account, milestone status = `Refunded`, escrow status = `Completed`.
7. Click "Export CSV" → one row per payout, fee included, ready for an accountant.

> TODO: confirm every step against the deployed demo before the repository goes public.

## How NEAR is used

- **NEAR Intents 1Click API:** `EXACT_OUTPUT` quote with `recipient` = escrow contract and
  `customRecipientMsg` = `escrow_id`; the solver delivers USDC through `ft_transfer_call`.
- **Escrow contract** (Rust, near-sdk): `ft_on_transfer` funding, milestones, revisions,
  split, fee, refund, auto-release, payout callback with retry.
- **Signed approvals** from NEAR (NEP-413), EVM (EIP-191) and Solana (ed25519) wallets,
  verified by the contract.
- **Meta transaction relayer:** pays gas only for calls to the Vecrow contract.
- **NEP-141 storage registration:** the backend registers recipients so payouts never
  require users to hold NEAR.

## Documentation

| Document | Contents |
| --- | --- |
| [`docs/spec.md`](./docs/spec.md) | Requirements R-01 to R-22, acceptance criteria, status, and links to code and tests |
| [`docs/architecture.md`](./docs/architecture.md) | Components, money flow, contract methods, key flows, trust boundaries |
| [`docs/decisions.md`](./docs/decisions.md) | Design decisions and their trade-offs |

## Repository layout

```text
.
├── contracts/   # Rust, near-sdk — escrow contract and tests
├── backend/     # Go — REST API, relayer, 1Click watcher, jobs
├── frontend/    # React + Vite — payment page, dashboard, account pages
└── docs/        # spec, architecture, decisions
```

## Contract

- Account: `TODO: <escrow>.near` (mainnet) · `TODO: <escrow>.testnet` (testnet)
- Methods: `create_escrow`, `cancel`, `bind_approver`, `ft_on_transfer`, `submit_milestone`,
  `approve_milestone`, `request_revision`, `auto_release`, `refund_unsubmitted`,
  `retry_payout`, `get_escrow`
- Escrow statuses: `Created`, `ApproverBound`, `Funded`, `Completed`, `Cancelled`
- Milestone statuses: `Pending`, `Submitted`, `RevisionRequested`, `Released`, `Refunded`

## Run locally

### Prerequisites

- Rust + `cargo-near` (contracts)
- Go (backend)
- Node.js (frontend)
- PostgreSQL and RabbitMQ
- A NEAR testnet account for the relayer and backend

> TODO: exact versions.

### Configuration

Copy `.env.example` to `.env` in `backend/` and `frontend/` and fill in the values.
Every variable is listed there; no secrets are committed.

### Contract

```sh
cd contracts
# TODO: build, test, and deploy commands
```

### Backend

```sh
cd backend
# TODO: go mod download, migrate, and run commands
```

### Frontend

```sh
cd frontend
# TODO: install and run commands
```

### Testnet vs mainnet

Contract development and tests run on testnet with a mock NEP-141 token. 1Click has no
testnet, so cross-chain funding runs on mainnet with small amounts; on testnet, quotes
are requested with `dry: true`.

## Limitations

- No third-party arbitration; a client cannot reject a milestone outright (max 2
  revisions, then auto-release after 14 days).
- USDC only; no fiat off-ramp; no KYC.
- Refunds go to a NEAR account, not back to the chain the client paid from.
- Over- or under-payment returns go to `sender_id` per NEP-141, which on the 1Click
  path is not the client's source wallet.
- The contract cannot prove the approver wallet is not a second wallet owned by the
  freelancer; the payment page shows the bound approver and warns the client.
- A lost wallet cannot recover contract rights (submit, approve, payouts); email
  recovery restores the account only.

## Hackathon disclosure

- All code in this repository was written during the hackathon. The first commit is from
  2026-11-08, inside the eligible window (2026-10-19 to 2026-11-14); there is no
  pre-hackathon baseline.
- Libraries, templates, and AI-assisted code: TODO: list.

## License

MIT — see [`LICENSE`](./LICENSE).
