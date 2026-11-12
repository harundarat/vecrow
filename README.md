# Vecrow — milestone escrow for cross-border teams

Built at NEAR HACKATHON (Nov 2026) · Track: Real-World Money (`realworld`)

## What it is

A client pays a freelancer or agency from any chain and token. Either side can open
the escrow and share a link; the other side joins with their wallet, and once the
creator approves, the escrow goes on-chain. Funds are held in an escrow contract on
NEAR and released per approved milestone as USDC, minus a 0.5% platform fee, split
automatically between team members. Freelancers and clients never buy NEAR for gas.

## The problem

- Freelancers deliver work and do not get paid; clients do not want to pay upfront.
- Agencies receive one payment and split it by hand across members in different countries.
- Existing crypto escrows require the client and the freelancer to use the same chain.

## Try it (judges)

> TODO: live demo URL · demo video (X post URL)

1. Log in at `<demo-url>` with a wallet, open `/new`, and create an escrow as the
   freelancer with 2 milestones and a 70/30 split → you get a link for the client,
   escrow status = `Open`.
2. Open the link in another browser (no account needed), connect a wallet as the
   client, enter a NEAR refund account, and sign the terms once
   → escrow status = `PendingApproval`.
3. As the freelancer, check the client's wallet and click "Approve" (one signature)
   → the backend creates the escrow on-chain, escrow status = `Created`.
4. As the client, pick USDC on Base and send to the deposit address shown
   → escrow status = `Funded`, with the NEAR transaction linked.
5. As the freelancer, click "Submit milestone 1" (the wallet holds 0 NEAR)
   → the relayer pays gas, milestone status = `Submitted`.
6. As the client, click "Request revision" → milestone status = `RevisionRequested`;
   the freelancer submits again → `Submitted`.
7. As the client, click "Approve" → the treasury receives 0.5% and both team accounts
   receive their share; transaction links are shown on the dashboard.
8. As the freelancer, click "Refund" on milestone 2 → USDC returns to the client's
   refund account, milestone status = `Refunded`, escrow status = `Completed`.
9. Click "Export CSV" → one row per payout, fee included, ready for an accountant.

The reverse direction works the same way: create the escrow as the client in step 1
(approver wallet and refund account), let the freelancer join through the link with
their NEAR account and split, and approve it as the client.

> TODO: confirm every step against the deployed demo before the repository goes public.

## How NEAR is used

- **NEAR Intents 1Click API:** `EXACT_OUTPUT` quote with `recipient` = escrow contract and
  `customRecipientMsg` = `escrow_id`; the solver delivers USDC through `ft_transfer_call`.
- **Escrow contract** (Rust, near-sdk): `create_escrow` checks both parties' signatures
  over the terms, `ft_on_transfer` funding, milestones, revisions, split, fee, refund,
  auto-release, payout callback with retry.
- **Signed terms and approvals** from NEAR (NEP-413), EVM (EIP-191) and Solana (ed25519)
  wallets, verified by the contract.
- **Meta transaction relayer:** pays gas only for calls to the Vecrow contract,
  including creating escrows that both parties signed.
- **NEP-141 storage registration:** the backend registers recipients so payouts never
  require users to hold NEAR.

## Documentation

| Document | Contents |
| --- | --- |
| [`docs/spec.md`](./docs/spec.md) | Requirements R-01 to R-23, acceptance criteria, status, and links to code and tests |
| [`docs/architecture.md`](./docs/architecture.md) | Components, money flow, contract methods, key flows (escrow creation, funding, approval, payout), trust boundaries |
| [`docs/decisions.md`](./docs/decisions.md) | Design decisions and their trade-offs |

## Repository layout

```text
.
├── contracts/   # Rust, near-sdk — escrow contract and tests
├── backend/     # Go — REST API, relayer, 1Click watcher, jobs
├── frontend/    # React + Vite — escrow pages, dashboard, account pages
└── docs/        # spec, architecture, decisions
```

## Contract

- Account: `TODO: <escrow>.near` (mainnet) · `TODO: <escrow>.testnet` (testnet)
- Code: [`contracts/escrow/`](./contracts/escrow/) (Rust, near-sdk 5.30), built with cargo-near
- Methods: `create_escrow`, `cancel`, `ft_on_transfer`, `submit_milestone`,
  `approve_milestone`, `request_revision`, `auto_release`, `refund_unsubmitted`,
  `on_payout` (callback), `retry_payout`, `get_escrow`, plus the `new` initializer
- Escrow statuses: `Created`, `Funded`, `Completed`, `Cancelled` (before an escrow is
  on-chain, the backend also uses `Open`, `PendingApproval`, and `Approved`)
- Milestone statuses: `Pending`, `Submitted`, `RevisionRequested`, `Released`, `Refunded`
- No method takes an attached deposit: the contract pays storage and the 1 yoctoNEAR of
  each `ft_transfer` from its own balance.
- Arguments, the signed terms and actions (byte by byte), the escrow ID format, the
  storage layout and the events are in
  [architecture.md §5](docs/architecture.md#5-escrow-contract).

## Run locally

### Prerequisites

- [rustup](https://rustup.rs) (contracts; Rust 1.97.0 with the wasm32 target is installed
  from `contracts/rust-toolchain.toml` on first use) and cargo-near 0.23:
  `cargo install cargo-near --version 0.23.0 --locked --no-default-features`
  (without default features it skips Ledger support, which needs libudev)
- Go 1.27+ (backend), with [goose](https://github.com/pressly/goose) for migrations and
  [sqlc](https://sqlc.dev) for query code generation
- Node.js (frontend)
- PostgreSQL, Redis and RabbitMQ (or Docker, to run them with `docker compose`)
- A NEAR testnet account for the relayer and backend

> TODO: exact versions.

### Configuration

Copy `.env.example` to `.env` in `backend/` and `frontend/` and fill in the values.
Every variable is listed there; no secrets are committed.

### Contract

```sh
cd contracts
make fmt-check   # rustfmt
make lint        # clippy -D warnings, for the host and the wasm32 target
make build       # cargo-near: target/near/escrow/vecrow_escrow.wasm and target/near/mock-ft/mock_ft.wasm
make test        # unit tests, then build + near-workspaces sandbox tests
```

`make test-unit` runs only the unit tests (no sandbox, no wasm build); `make test-sandbox`
builds the wasm files and runs the sandbox tests in `integration-tests/`. The first sandbox
run downloads the nearcore sandbox binary (about 95 MB unpacked, from
`s3-us-west-1.amazonaws.com`). `make help` lists every target.

| Crate | Contents |
| --- | --- |
| `escrow` | The escrow contract; unit tests in `src/tests/` |
| `mock-ft` | Mock NEP-141 token used as USDC in tests (open `mint`, NEP-145 registration) |
| `wallet-fixtures` | Test signers reproducing NEP-413, EIP-191 and Solana `signMessage` bytes |
| `integration-tests` | Sandbox tests: signed terms, funding, payouts, retries, unregistered accounts, meta transactions |

nearcore raises its open-file limit to 65,535 at startup. On Linux, when the hard limit
(`ulimit -H -n`) is lower and cannot be raised, as in some containers, `make test-sandbox`
downloads the sandbox binary into `target/sandbox-shim/` and runs it with
[`scripts/fd-limit-shim.c`](./contracts/scripts/fd-limit-shim.c) preloaded, which clamps
that request to the hard limit (this needs a C compiler). Set `NEAR_SANDBOX_BIN_PATH` to
use another sandbox binary instead.

Deploying (testnet first, D-11) takes three steps:

1. `cargo near deploy build-non-reproducible-wasm <escrow-account>`, choosing
   `with-init-call new` with
   `{"usdc_contract_id": "<usdc>", "treasury_account": "<treasury>", "fee_bps": 50, "review_window_sec": 1209600}`
   (a shorter `review_window_sec` for the demo).
2. Register the escrow account and the treasury on the USDC contract:
   `storage_deposit({"account_id": "<account>", "registration_only": true})` with the
   deposit from `storage_balance_bounds().min`.
3. Keep NEAR on the escrow account for storage: about 0.003 NEAR per escrow and
   0.0007 NEAR per signed action (D-19).

### Backend

```sh
cd backend
cp .env.example .env
make compose-up     # PostgreSQL, Redis and RabbitMQ in Docker
make migrate-up     # needs goose: go install github.com/pressly/goose/v3/cmd/goose@latest
make run-api        # http://localhost:8080 — GET /readyz checks the database and cache
make run-worker     # scheduler + queue consumers
make test           # unit tests; `make test-integration` also needs `make compose-up`
make test-e2e       # end-to-end tests on a local NEAR sandbox (see below)
make vet            # go vet, including the integration and end-to-end tests
```

The API and the worker also need `AUTH_DOMAIN`, `NEAR_RPC_URL`, the deployed contracts
(`ESCROW_CONTRACT_ID`, `ESCROW_FEE_BPS`, `USDC_CONTRACT_ID`), `APP_URL`, and the relayer
account (`RELAYER_ACCOUNT_ID`, `RELAYER_PRIVATE_KEY`); the worker also needs the storage
account (`STORAGE_ACCOUNT_ID`, `STORAGE_PRIVATE_KEY`), which pays `storage_deposit`. Both
are NEAR accounts of the backend with a NEAR balance; their keys come only from the
environment (D-23). `ONECLICK_API_KEY` is the optional 1Click JWT. Every variable is described in
`.env.example`. Wallet login for the frontend is described in
[architecture.md §7.5](docs/architecture.md#75-wallet-login-r-13-r-14), the escrow flow
in [§6](docs/architecture.md#6-key-flows), and the jobs in
[§7.3](docs/architecture.md#73-jobs).

`make test-e2e` builds the contracts (`make -C ../contracts build`, needs cargo-near),
starts the nearcore sandbox version pinned in `contracts/Makefile` (through the
open-file-limit shim when needed, or `NEAR_SANDBOX_BIN_PATH`), deploys the escrow and
mock-ft contracts, and runs the API and the worker against PostgreSQL, Redis and
RabbitMQ from `make compose-up` and a fake 1Click server. Each run uses a fresh
database next to `DATABASE_URL` and needs goose on `PATH`. It covers both directions
(freelancer or client creates) with freelancer and client holding 0 NEAR: creation
on-chain, funding, submit, revision, approval, payout retry, auto-release and refund.

`make help` lists every target. After adding or changing a query in
`internal/repository/queries/`, run `make sqlc`
(install: `go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest`).
`docker compose --profile app up --build` runs the API and worker from the container image.

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
  freelancer; the creator sees the other party's addresses before approving, and the
  client escrow page shows the approver and warns the client.
- The terms cannot be edited after the link is shared; the creator cancels and creates
  a new escrow.
- The contract cannot read NEAR access keys, so it checks a NEP-413 signature against the
  key given with it; for a named freelancer account the backend checks that key over RPC
  (D-16).
- A NEAR approver is tied to the key in the signed terms; if that key is removed, the
  client can no longer approve, and milestones are released by auto-release.
- A lost wallet cannot recover contract rights (submit, approve, payouts); email
  recovery restores the account only.

## Hackathon disclosure

- All code in this repository was written during the hackathon. The first commit is from
  2026-11-08, inside the eligible window (2026-10-19 to 2026-11-14); there is no
  pre-hackathon baseline.
- Libraries, templates, and AI-assisted code: TODO: list.

## License

MIT — see [`LICENSE`](./LICENSE).
