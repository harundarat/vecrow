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
   On testnet, where 1Click does not run, fund it instead with
   `contracts/scripts/fund-escrow-testnet.sh <escrow-id> <account>` (the ID is in the
   page URL, `/escrows/<escrow-id>`) and reload the page → escrow status = `Funded`
   (see [Testnet vs mainnet](#testnet-vs-mainnet)).
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
├── frontend/    # Next.js — landing, escrow pages, dashboard, account pages
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
- [Bun](https://bun.sh) (frontend; tested with 1.4)
- PostgreSQL, Redis and RabbitMQ (or Docker, to run them with `docker compose`)
- A NEAR testnet account for the relayer and backend

Versions: "pinned" means the repository fixes the version; "tested with" is what the
team ran on Linux (amd64).

| Tool | Version | Where |
| --- | --- | --- |
| Rust | 1.97.0, pinned | `contracts/rust-toolchain.toml` (with rustfmt, clippy, `wasm32-unknown-unknown`) |
| cargo-near | 0.23.0, pinned | install command above |
| nearcore sandbox | 2.14.0, pinned | `SANDBOX_VERSION` in `contracts/Makefile` (run by near-workspaces 0.24.0) |
| Go | 1.27.1, pinned | `backend/go.mod` |
| goose | v3.28.0, tested with | `go install github.com/pressly/goose/v3/cmd/goose@v3.28.0` |
| sqlc | v1.31.1, pinned | header of the generated code in `backend/internal/repository/dbgen/` |
| Bun | 1.4.4, tested with | `bun install`, `bun dev` in `frontend/` |
| Node.js | 24.10.0, tested with | `bun dev` starts the `next` CLI, which runs on Node.js when it is installed |
| Next.js / React | 16.4.4 / 19.3.1, pinned | `frontend/package.json` |
| PostgreSQL / Redis / RabbitMQ | 18 / 8 / 4, pinned | images in `backend/docker-compose.yml` |
| Docker / Docker Compose | 29.9.3 / v5.7.1, tested with | runs the services above |

### Configuration

Copy `backend/.env.example` to `backend/.env` and `frontend/env.example` to
`frontend/.env`, then fill in the values. Every variable is listed there; no secrets are
committed.

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

Deploying (testnet first, D-11):

1. Choose the token, `<usdc>` below. On mainnet it is native USDC,
   `17208628f84f5d6ad33f0da3bbbeb27ffcb398eac501a31bd6ad2011e36133a1` (from the 1Click
   token list). On testnet, deploy the mock token after `make build` and use its account
   (about 1.6 NEAR for its 151 KB of code):
   `near contract deploy <usdc-account> use-file target/near/mock-ft/mock_ft.wasm with-init-call new json-args '{"symbol": "USDC", "decimals": 6}' prepaid-gas '30.0 Tgas' attached-deposit '0 NEAR' network-config testnet sign-with-keychain send`
2. `cargo near deploy build-non-reproducible-wasm <escrow-account>` in `contracts/escrow/`,
   choosing `with-init-call new` with
   `{"usdc_contract_id": "<usdc>", "treasury_account": "<treasury>", "fee_bps": 50, "review_window_sec": 1209600}`
   (a shorter `review_window_sec` for the demo).
3. Register the escrow account and the treasury on the USDC contract:
   `storage_deposit({"account_id": "<account>", "registration_only": true})` with the
   deposit from `storage_balance_bounds().min` (0.00125 NEAR on USDC and on mock-ft).
4. Keep NEAR on the escrow account for storage: about 2.3 NEAR locked by its 230 KB of
   code, then about 0.003 NEAR per escrow and 0.0007 NEAR per signed action (D-19).

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
[architecture.md §7.5](docs/architecture.md#75-wallet-login-r-13-r-14), email
verification and recovery in [§7.6](docs/architecture.md#76-email-and-recovery-r-14-r-22),
invitations in [§7.7](docs/architecture.md#77-invitations-r-15-r-16), the CSV export in
[§7.8](docs/architecture.md#78-csv-export-r-12), the escrow flow in
[§6](docs/architecture.md#6-key-flows), and the jobs in
[§7.3](docs/architecture.md#73-jobs).

The API emails one-time codes (email verification, recovery login) and a notice for
each wallet linked during recovery. Set `SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME`,
`SMTP_PASSWORD` and `SMTP_FROM` (STARTTLS by default; `SMTP_TLS=tls` for port 465);
`SMTP_HOST` and `SMTP_FROM` are required in production. In development, without `SMTP_HOST`, emails are only logged
(masked recipient and subject, never the body or the code); to read them, run a mail
catcher such as [Mailpit](https://mailpit.axllent.org) on `localhost:1025` with
`SMTP_HOST=localhost`, `SMTP_PORT=1025`, `SMTP_TLS=none`.

`make test-e2e` builds the contracts (`make -C ../contracts build`, needs cargo-near),
starts the nearcore sandbox version pinned in `contracts/Makefile` (through the
open-file-limit shim when needed, or `NEAR_SANDBOX_BIN_PATH`), deploys the escrow and
mock-ft contracts, and runs the API and the worker against PostgreSQL, Redis and
RabbitMQ from `make compose-up` and a fake 1Click server. Each run uses a fresh
database next to `DATABASE_URL` and needs goose on `PATH`. It covers both directions
(freelancer or client creates) with freelancer and client holding 0 NEAR: creation
on-chain, funding, submit, revision, approval, payout retry, auto-release and refund,
and the CSV export after the approval and after the refund.

`make help` lists every target. After adding or changing a query in
`internal/repository/queries/`, run `make sqlc`
(install: `go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest`).
`docker compose --profile app up --build` runs the API and worker from the container image.

### Frontend

Next.js 16 (App Router, Turbopack) with Bun. It needs the API from the backend section
above running on `localhost:8080`.

```sh
cd frontend
bun install
cp env.example .env   # then set NEXT_PUBLIC_REOWN_PROJECT_ID
bun dev               # http://localhost:5173
bun run typecheck && bun run lint && bun run build
```

| Variable | Meaning |
| --- | --- |
| `NEXT_PUBLIC_API_URL` | Base URL of the API, e.g. `http://localhost:8080/api/v1` |
| `NEXT_PUBLIC_NEAR_NETWORK` | `testnet` or `mainnet`: the network NEAR wallets connect to |
| `NEXT_PUBLIC_REOWN_PROJECT_ID` | Reown (WalletConnect) project ID for EVM and Solana wallets, from [dashboard.reown.com](https://dashboard.reown.com) |

The variables are checked at startup (`src/env.ts`); a missing one stops `bun dev`.

How it talks to the backend:

- Every call goes to `NEXT_PUBLIC_API_URL`. The backend must allow the frontend's origin
  (`CORS_ALLOWED_ORIGINS=http://localhost:5173`), sign-in messages are bound to
  `AUTH_DOMAIN=localhost:5173`, and escrow links point to `APP_URL=http://localhost:5173`.
- Sign-in: the wallet signs the message the API returns (`/auth/nonce`, then
  `/auth/verify`); the session token is kept in `localStorage`.
- Escrow actions are signed in the browser and sent to the API, which relays them
  on-chain. The freelancer signs NEP-366 delegate actions, so Vecrow pays the gas.
- Payments go through NEAR Intents 1Click: the API returns a deposit address, and the page
  polls the payment status.

See [`frontend/README.md`](./frontend/README.md) for the code layout and conventions.

### Testnet vs mainnet

Contract development and tests run on testnet with a mock NEP-141 token (`mock-ft`, with
an open `mint`). 1Click has no testnet, so cross-chain funding only runs on mainnet, with
small amounts. On testnet, a direct `ft_transfer_call` of the mock token takes the place
of the client's payment:

```sh
cd contracts
scripts/fund-escrow-testnet.sh <escrow-id> <payer-account>
```

The script reads `ESCROW_CONTRACT_ID`, `USDC_CONTRACT_ID` and `NEAR_RPC_URL` from
`backend/.env` and stops unless the RPC is on testnet and the escrow is `Created`. It
then registers the payer on the mock token if needed, mints the escrow total to the
payer, and sends it to the escrow contract with the escrow ID as `msg`. The escrow page
shows `Funded` on the next load, because the backend reads escrows from the contract.
The payer signs with the near CLI ([near-cli-rs](https://github.com/near/near-cli-rs));
set `NEAR_SIGN_WITH=sign-with-legacy-keychain` for keys in `~/.near-credentials`.

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
- No library, template or code written by the team before the hackathon is used. All
  third-party code comes from public packages, used as published.

### Libraries

| Part | Libraries (versions in the lock files) |
| --- | --- |
| Contracts | `near-sdk` 5.30.1 (with `unstable` for `env::ecrecover`), `near-contract-standards` 5.30.1 (mock USDC token), `hex` |
| Contract tests | `near-workspaces` 0.24.0, `near-jsonrpc-client` 0.23, `near-primitives` and `near-crypto` 0.38, `tokio`, `anyhow`, `serde_json`; test wallet signers use `ed25519-dalek`, `k256`, `sha2`, `sha3`, `borsh`, `bs58`, `base64` |
| Backend | `gin` and `gin-contrib/cors` (HTTP), `pgx` v5 (PostgreSQL), `go-redis` v9, `amqp091-go` (RabbitMQ), `caarlos0/env` and `godotenv` (configuration), `go-playground/validator`, `google/uuid`, `decred/dcrd/dcrec/secp256k1` and `golang.org/x/crypto/sha3` (EIP-191 signatures), `mr-tron/base58` (NEAR keys and transactions), `golang.org/x/sync` |
| Backend tools | goose (migrations), sqlc (query code generation) |
| Frontend | Next.js 16.4.4, React 19.3.1, `@hot-labs/near-connect` and `near-api-js` (NEAR wallets), `@reown/appkit` with its ethers and Solana adapters, and `ethers` (EVM and Solana wallets), `@base-ui/react`, `shadcn`, Tailwind CSS 4, `tw-animate-css`, `lucide-react`, `next-themes`, `class-variance-authority`, `cn`, `zod`, `@t3-oss/env-nextjs` |

### Templates and generated code

- UI primitives in `frontend/src/components/ui/` were added with the shadcn CLI
  (`bunx shadcn add`, style `base-vega`) and then adjusted.
- `frontend/AGENTS.md` is written by `next dev`.
- `backend/internal/repository/dbgen/` is generated by sqlc from the SQL queries and
  migrations.

### AI-assisted code

- Most of the code and documentation was written with Claude Code (Claude Opus 5.5 and
  Claude Sonnet 5.5).
- AI-assisted commits carry a `Co-Authored-By: Claude` trailer, so the git history shows
  which changes they are: 111 of the 121 non-merge commits up to 2026-11-14.

## License

MIT — see [`LICENSE`](./LICENSE).
