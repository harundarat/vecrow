# Vecrow — Design Decisions

This document records the main design decisions behind Vecrow, why they were made,
and what they cost. Requirements (`R-xx`) are defined in [`spec.md`](./spec.md).

Each decision lists its context, the decision itself, the alternatives, and the
consequences, including the trade-offs we accept.

| ID | Decision | Date | Status | Requirements |
| --- | --- | --- | --- | --- |
| [D-01](#d-01--the-client-binds-the-approver-at-payment-time) | The client binds the approver at payment time | 2026-11-07 | Superseded by D-14 | R-04, R-10, R-15 |
| [D-02](#d-02--approvals-are-verified-on-chain-from-wallet-signatures) | Approvals are verified on-chain from wallet signatures | 2026-11-07 | Accepted | R-01, R-06, R-09, R-17, R-21 |
| [D-03](#d-03--capped-revisions-and-auto-release-instead-of-rejection-or-arbitration) | Capped revisions and auto-release instead of rejection or arbitration | 2026-11-08 | Accepted | R-07, R-17 |
| [D-04](#d-04--only-the-freelancer-can-refund-and-only-unsubmitted-milestones) | Only the freelancer can refund, and only unsubmitted milestones | 2026-11-08 | Accepted | R-04, R-18 |
| [D-05](#d-05--funding-amount-rules-and-exact_output-quotes) | Funding amount rules and `EXACT_OUTPUT` quotes | 2026-11-08 | Accepted | R-02, R-08 |
| [D-06](#d-06--a-05-fee-locked-per-escrow-with-deterministic-rounding) | A 0.5% fee locked per escrow, with deterministic rounding | 2026-11-08 | Accepted | R-03, R-19 |
| [D-07](#d-07--the-backend-registers-storage-failed-payouts-are-retried) | The backend registers storage; failed payouts are retried | 2026-11-08 | Accepted | R-20 |
| [D-08](#d-08--accounts-are-for-identity-and-visibility-never-for-authority) | Accounts are for identity and visibility, never for authority | 2026-11-08 | Accepted | R-13 – R-16, R-21, R-22 |
| [D-09](#d-09--funds-arrive-through-1click-directly-into-the-contract) | Funds arrive through 1Click directly into the contract | 2026-11-08 | Pending mainnet test | R-08 |
| [D-10](#d-10--one-set-of-status-names-across-contract-and-api) | One set of status names across contract and API | 2026-11-08 | Accepted | — |
| [D-11](#d-11--testnet-for-contract-development-mainnet-for-1click) | Testnet for contract development, mainnet for 1Click | 2026-11-07 | Accepted | R-08 |
| [D-12](#d-12--backend-stack-and-layering) | Backend stack and layering | 2026-11-08 | Accepted | — |
| [D-13](#d-13--wallet-login-plain-text-challenge-and-opaque-bearer-sessions) | Wallet login: plain-text challenge and opaque bearer sessions | 2026-11-09 | Accepted | R-13, R-14 |
| [D-14](#d-14--either-party-creates-the-escrow-it-goes-on-chain-only-after-both-sign-the-same-terms) | Either party creates the escrow; it goes on-chain only after both sign the same terms | 2026-11-11 | Accepted | R-01, R-03, R-04, R-15, R-21, R-23 |

---

## D-01 — The client binds the approver at payment time

> **Superseded by [D-14](#d-14--either-party-creates-the-escrow-it-goes-on-chain-only-after-both-sign-the-same-terms)
> (2026-11-11).** Either party can now create the escrow. The client sets the approver
> and `refund_account` in the terms they sign, whether they created the escrow or
> joined it, and `create_escrow` verifies that signature. There is no `bind_approver`
> step and no `ApproverBound` status any more. The core rule below still holds: only
> the client chooses the approver. The rest of this section is kept as history.

**Context.** The freelancer creates the escrow, but the approver must be the client.
If the freelancer could name the approver, they could name a wallet they control and
approve their own milestones.

**Decision.** The approver is set by the client on the payment page: they connect a
wallet and sign once. The same signature sets the `refund_account` (a NEAR account).
Both are locked once the escrow is `Funded`. The contract rejects an approver or
`refund_account` equal to the freelancer or any split recipient. The 1Click deposit
address is only shown to the bound approver wallet.

**Alternatives.**
- The freelancer enters the client's wallet when creating the escrow. Simpler, but the freelancer controls who approves.
- Approval through a backend account role. Rejected because the backend would become the authority over funds (see D-02).

**Consequences.**
- The client pays and approves with the same wallet, with no extra setup.
- The contract cannot prove that the approver is not a second wallet owned by the freelancer. The mitigation is on the payment page: the bound approver address is visible, and a client who did not bind it is warned not to pay.
- Inviting a client by username or email (R-15) only grants visibility; it never sets the approver.

## D-02 — Approvals are verified on-chain from wallet signatures

**Context.** Clients hold wallets on different chains and should not need NEAR or gas
to approve. A relayer pays the gas, which means the relayer must not be trusted with
the decision itself.

**Decision.** Approvals (R-06), revision requests (R-17), and a client's cancellation
before funding (R-01) are payloads signed by the approver's own wallet, with a nonce
and a deadline. The escrow terms are signed the same way by both parties before the
escrow goes on-chain (D-14). The contract verifies the signatures itself, for three
schemes:

| Wallet | Scheme |
| --- | --- |
| NEAR | NEP-413 |
| EVM (e.g. MetaMask) | EIP-191, verified with `ecrecover` |
| Solana (e.g. Phantom) | ed25519 |

The relayer (R-09) only forwards signed payloads, pays gas only for calls to the
Vecrow contract, and cannot approve anything on its own. Backend sessions and guest
tokens (R-21) are never accepted as proof of approval.

**Alternatives.**
- Require a NEAR wallet for approval. Simpler contract, but most clients would have to create a NEAR wallet first.
- Let the backend approve on the client's behalf after login. Gasless and simple, but makes the backend a custodian of the approval right.

**Consequences.**
- A compromised backend or relayer cannot release funds.
- Three signature formats increase contract complexity. Each scheme is tested with real signatures from the corresponding wallet, stored as test fixtures.
- Schemes are implemented in the order NEAR, EVM, Solana. Any scheme that is not finished is listed under Limitations in `spec.md`.

## D-03 — Capped revisions and auto-release instead of rejection or arbitration

**Context.** A freelancer needs a guarantee that funds cannot be held forever. A client
needs a way to say "not done yet". Third-party arbitration is out of scope for the MVP.

**Decision.**
- After a submission, the client has 14 days to respond. After that, anyone may call `auto_release`; a backend job does so automatically (R-07).
- The client may call `request_revision` at most 2 times per milestone (R-17). This pauses the timer until the freelancer re-submits, and the 14 days restart from the latest submission.
- `auto_release` is rejected while a milestone is `RevisionRequested`.
- The client cannot reject a milestone outright.

**Alternatives.**
- Unlimited revisions: the client could hold funds indefinitely.
- Outright rejection with a refund: requires a dispute process to be fair to the freelancer.
- Third-party arbitration: out of scope for the MVP.

**Consequences.**
- Funds are always released or refunded within a bounded time.
- A client who is still unhappy after 2 revisions has no on-chain recourse. This is listed as a limitation.
- The review window is a deploy-time parameter, so the demo can use a short window without changing the code.

## D-04 — Only the freelancer can refund, and only unsubmitted milestones

**Context.** A project can stop midway. Funds for work that was never delivered should
go back to the client, but the client must not be able to take back funds for work
that has been submitted.

**Decision.** Only the freelancer can call `refund_unsubmitted`, for one or all
`Pending` milestones of a `Funded` escrow (R-18). The USDC goes to the client's
`refund_account`, which the client set in the signed escrow terms (D-14). Refunds are
not charged a fee (D-06).

**Alternatives.**
- Client-initiated refund: lets the client claw back funds without a dispute process.
- Refund back to the client's original chain via 1Click: out of scope for the MVP.

**Consequences.**
- Refunds cannot be used against the freelancer.
- If the freelancer disappears, `Pending` milestones stay locked; there is no client-side exit.
- Refunds land on a NEAR account, not on the chain the client paid from. This is less smooth than funding and is listed as a limitation.

## D-05 — Funding amount rules and `EXACT_OUTPUT` quotes

**Context.** USDC arrives at the contract through `ft_transfer_call`, and the amount may
not match the escrow total exactly.

**Decision.** In `ft_on_transfer` (R-02):
- Less than the total: returned in full; the escrow stays `Created`.
- Equal to or more than the total: the escrow becomes `Funded` with exactly the total, and `amount - total` is returned through the NEP-141 return value.
- Transfers to an escrow that is not `Created`, of another token, or with an unknown `escrow_id` (including an escrow that is not on-chain yet): returned in full.

Funding quotes use 1Click `EXACT_OUTPUT` (R-08), so the amount delivered matches the total.

**Alternatives.**
- Accept partial funding and track a running balance: more states and more edge cases for little benefit in the MVP.

**Consequences.**
- An escrow is either fully funded or not funded; there is no partial state.
- Returns go to `sender_id` as defined by NEP-141. On the 1Click path, `sender_id` is the account that calls `ft_transfer_call`, not the client's source wallet. `EXACT_OUTPUT` keeps the difference close to zero; where the returned funds end up is verified in the mainnet test (D-09).

## D-06 — A 0.5% fee locked per escrow, with deterministic rounding

**Context.** The platform charges a fee, and the total paid out must always equal the
milestone amount, with no dust left in the contract.

**Decision.**
- `fee_bps = 50` is part of the terms both parties sign (D-14) and is stored per escrow at `create_escrow`. Terms signed with a `fee_bps` that differs from the current configuration are rejected; later configuration changes do not affect escrows that are already on-chain (R-19).
- On each release, `fee = floor(amount × fee_bps / 10,000)` goes to `treasury_account`; the remainder is split by bps (R-03).
- Split rounding remainders go to the first recipient.
- The contract asserts that fee + all shares = milestone amount.
- No fee is charged on refunds (R-18) or on returned deposits (R-02).

**Example.** A 1,000 USDC milestone with a 70/30 split: 5 to treasury, 696.5 and 298.5 to the recipients.

**Consequences.**
- Fees are predictable for both parties and are shown before signing the terms, on the dashboard, and on the client escrow page.
- The first split recipient may receive up to a few base units more than their exact share.

## D-07 — The backend registers storage; failed payouts are retried

**Context.** On NEAR, an account must be registered on the USDC contract
(`storage_deposit`) before it can receive tokens. A transfer to an unregistered account
fails, and in an escrow that would leave funds stuck. Users should not need NEAR to
register.

**Decision.** (R-20)
- The escrow contract and `treasury_account` are registered at deploy time.
- The backend checks `storage_balance_of` and pays `storage_deposit` (`registration_only = true`) from a backend account for the split recipients and the `refund_account`, before it calls `create_escrow` (D-14).
- Every payout uses `ft_transfer` with an `on_payout` callback. A failed transfer is recorded in `pending_payout[escrow][account]`.
- Anyone may call `retry_payout(escrow_id, account)`; a backend job calls it automatically. The pending amount is zeroed before sending and restored if the transfer fails again.

**Alternatives.**
- Require users to register themselves: users would need NEAR.
- Have the contract pay registration from escrowed funds: mixes client funds with operating costs.

**Consequences.**
- No USDC is lost to an unregistered account, and a retry cannot pay twice.
- The backend spends a small amount of NEAR per registration. This is the only value the backend contributes; it never holds client funds (except under the D-09 fallback).

## D-08 — Accounts are for identity and visibility, never for authority

**Context.** Users want a list of their escrows, several wallets per account, and a way
to recover a lost login. None of this should weaken the guarantees enforced by the
contract.

**Decision.**
- Sign-up and login use a wallet signature over a backend nonce; the first login creates the account (R-13). Further wallets and an optional email can be linked (R-14).
- Creating an escrow needs a wallet session. The other party can join through the link without an account: their signed submission returns a guest token for one escrow and one wallet, valid for 24 hours (R-21).
- Email OTP login is a recovery-only session: it can link a new wallet (still proven by that wallet's signature) and unlink old ones, but cannot create escrows, approve or reject the other party, or change invitations (R-22).
- The contract never reads accounts. Creating the escrow on-chain, submit, approve, revise, refund, and payouts depend only on wallet keys and signatures.

**Consequences.**
- A compromised backend account or email cannot move funds.
- Recovering an account does not restore contract rights. A lost freelancer or approver wallet cannot be replaced in an existing escrow; this is listed as a limitation.

## D-09 — Funds arrive through 1Click directly into the contract

**Status: pending the mainnet test.** This decision will be confirmed or switched to the fallback after the test, and this section updated with the result.

**Context.** Clients pay from any chain and token. NEAR Intents' 1Click API swaps the
payment into USDC on NEAR. The escrow contract needs to know which escrow the funds
belong to. 1Click has no testnet.

**Decision.** The quote uses `recipient` = escrow contract and `customRecipientMsg` =
`escrow_id`, so USDC arrives through `ft_transfer_call` with `msg = escrow_id` and
the backend never holds funds.

**Fallback.** If `customRecipientMsg` does not deliver reliably (the 1Click
documentation marks it as experimental), 1Click sends USDC to a backend account, and
the backend calls `ft_transfer_call` to the contract with `msg = escrow_id`.

**Consequences.**
- Primary path: funds go straight from the solver to the contract.
- Fallback path: the backend briefly holds client funds between the 1Click delivery and its own `ft_transfer_call`. If the fallback is used, this is disclosed in the README.

## D-10 — One set of status names across contract and API

**Decision.** The contract enums `EscrowStatus { Created, Funded, Completed,
Cancelled }` and `MilestoneStatus { Pending, Submitted, RevisionRequested, Released,
Refunded }` are serialized as PascalCase strings. Before an escrow is on-chain, the
backend uses three more escrow statuses, `Open`, `PendingApproval`, and `Approved`,
and records `Cancelled` itself for escrows cancelled before that point (D-14). The
API, mock data, and documentation use the same strings with no mapping. 1Click
funding status (`PENDING`, `SUCCESS`, `REFUNDED`) is kept separate and appears only in
`GET /escrows/:id/funding`.

**Consequences.** There is no translation layer to drift between contract, backend,
and frontend.

## D-11 — Testnet for contract development, mainnet for 1Click

**Context.** The escrow contract should be developed and tested without real funds, but
1Click has no testnet.

**Decision.**
- Contract: Rust with near-sdk, built with cargo-near, tested with near-workspaces. Development runs on testnet with a mock NEP-141 token.
- Cross-chain funding is exercised on mainnet with small amounts from separate development accounts.
- The USDC contract ID and the list of source assets are taken from the 1Click token list, not hard-coded.

**Consequences.**
- All contract behaviour can be reproduced on testnet without spending money.
- The cross-chain funding step can only be reproduced on mainnet; `dry: true` quotes show it without sending funds.

## D-12 — Backend stack and layering

**Context.** The backend serves the REST API and runs the jobs (architecture.md §7). A
small team builds it during the hackathon, so it has to be quick to extend, easy to
read, and testable without running every dependency. Some infrastructure choices, such
as the queue, may change later.

**Decision.**
- Go with Gin for HTTP. The API (`cmd/api`) and the worker (`cmd/worker`: scheduler and queue consumers) are separate binaries built from the same module and container image.
- Clean architecture: domain → use cases → repositories and delivery → composition root (architecture.md §7.4). Each use case declares the interfaces it needs, and `internal/bootstrap` wires the implementations by hand (constructor injection, no DI framework or code generation).
- Infrastructure packages are named after their role (`database`, `cache`, `queue`) and expose generic interfaces. Replacing a technology only changes that package and `bootstrap`, except the database: `database.Querier` uses pgx types to match sqlc's generated code, so replacing PostgreSQL also means regenerating the repositories.
- PostgreSQL through pgx. Queries are written in SQL and compiled to type-safe Go by sqlc. Schema changes are goose migrations, run explicitly (`make migrate-up`), never at startup.
- Redis for short-lived state with a TTL (login nonces, OTP rate limits). RabbitMQ for jobs as described in architecture.md §7.3, with a retry queue and a dead-letter queue per topic.
- `log/slog` for structured logging. Request IDs are propagated through HTTP and queue messages, and sensitive attribute keys are redacted.

**Alternatives.**
- A DI container (uber-go/fx) or compile-time DI (google/wire): less wiring code, but more indirection than the project needs today.
- GORM instead of sqlc: less SQL to write, but queries are less explicit and mistakes surface at runtime instead of at generation time.
- zap or zerolog: faster, but slog is in the standard library and fast enough here.
- One binary for the API and jobs: simpler to deploy, but the two cannot be scaled or restarted separately.
- Running migrations at startup: convenient, but replicas can race, and a bad migration blocks every deploy.

**Consequences.**
- Business logic is tested with in-memory fakes; only the infrastructure adapters need the real services (`make test-integration`).
- A feature touches several small packages (domain, use case, repository, handler, bootstrap). That is more files than a single-package design, in exchange for clear boundaries.
- sqlc must be re-run after a query changes, and the generated code is committed.
- Developers need Docker (or local PostgreSQL, Redis and RabbitMQ), goose, and sqlc.

## D-13 — Wallet login: plain-text challenge and opaque bearer sessions

**Context.** Users log in and link further wallets by signing with an EVM, Solana or NEAR
wallet (R-13, R-14). Each wallet family signs differently, and the frontend is a single-page
app on its own origin.

**Decision.**
- One plain-text message, built by the backend and returned with the nonce, is signed for
  login and for linking alike. It names `AUTH_DOMAIN`, the wallet, the nonce and the
  expiry. NEAR signs it through NEP-413 with `AUTH_DOMAIN` as the recipient.
- The backend stores the message with the nonce and verifies the signature over exactly that
  text. A nonce is consumed before the signature is checked, so each nonce allows one attempt.
- For NEAR, the signing key must be a full access key of the account, checked over RPC. An
  implicit account that is not yet on chain is accepted when its ID is the hex of the key.
- The first login creates the account with a generated username (`user_` + 8 random
  characters), which the user can change.
- Sessions are opaque random tokens sent as `Authorization: Bearer`. Redis stores only the
  SHA-256 of the token, with a TTL (`AUTH_SESSION_TTL`, default 24 hours). Logout deletes it.

**Alternatives.**
- SIWE (EIP-4361) and SIWS message formats: wallets render them as sign-in requests and check
  the domain, but they are two more formats to build and parse; the plain text keeps one format
  for all three wallet families.
- Choosing a username at sign-up: one more step before the first login.
- JWT sessions: no store lookup per request, but logout and revocation need a deny-list, and a
  signing key has to be managed.
- HttpOnly cookies: not readable by scripts, but a cross-origin SPA then needs credentialed
  CORS and CSRF protection.

**Consequences.**
- EVM smart-contract wallets (EIP-1271, ERC-4337) cannot log in, consistent with the contract's
  `ecrecover` check (D-02).
- Logging in with a NEAR named account depends on the NEAR RPC being available.
- The token is readable by frontend scripts, so XSS must be prevented on the frontend.
- Sessions are not revoked when a wallet is unlinked; they belong to the account.

## D-14 — Either party creates the escrow; it goes on-chain only after both sign the same terms

**Context.** A deal can start on either side: a client hiring for a project, or a
freelancer sending an offer. Until now only the freelancer could create an escrow, and
the client bound the approver on the payment page afterwards (D-01). The new rule is
that either party creates the escrow and shares a link, the other party submits their
address, and the creator approves the submission before the backend creates the
escrow on-chain. Because the backend now sends `create_escrow`, the contract must not
have to trust the backend for who is paid and who approves.

**Decision.** (R-01, R-23)
- Either party creates the escrow with a wallet session, choosing their role, the milestones, and their own details. It is stored only in the backend (`Open`), and the creator shares its link.
- Each party always provides the details of their own role, whoever created the escrow: the freelancer their NEAR account and the split, the client the approver wallet and `refund_account`. The rule from D-01 still holds, and now in both directions: only the client chooses the approver, and only the freelancer chooses the split.
- The other party submits their details and signs the final terms: escrow ID, freelancer, milestones, split, approver, `refund_account`, `fee_bps`, and a deadline (`PendingApproval`).
- The creator approves by signing the same terms (`Approved`), or rejects the submission, which returns the escrow to `Open`. A submission not approved before its deadline expires the same way.
- A backend job registers storage (D-07) and calls `create_escrow(terms, freelancer_signature, client_signature)` through the relayer. The contract verifies both signatures, the deadline, that the escrow ID is new, and the rules on approver, `refund_account`, split, and fee; the escrow becomes `Created`.
- `bind_approver` and the `ApproverBound` status are removed: the approver and `refund_account` are fixed by `create_escrow`.
- Before the escrow is on-chain, only the creator can cancel it, and nothing is sent to the contract. Once it is `Created` and before it is funded, the freelancer or the approver can call `cancel`.
- The terms cannot be edited once the link is shared; the creator cancels and creates a new escrow.

**Alternatives.**
- Keep freelancer-only creation and bind the approver at payment time (D-01): a client cannot start an escrow.
- The backend verifies the signatures off-chain and calls `create_escrow` as a trusted deployer: a simpler contract, but a compromised backend could set its own approver or split recipients.
- Create the escrow on-chain immediately and let the other party bind later: gas and contract storage are spent on escrows that are never accepted, and the creator cannot review the other party before anything is on-chain.
- Deploy a separate contract per escrow (factory): more gas and storage per escrow and harder upgrades, for no gain over one contract that holds many escrows.

**Consequences.**
- No gas or contract storage is spent until both parties agree.
- The backend can delay or refuse to create an escrow on-chain, but it cannot change the terms or create one without both signatures.
- The freelancer signs the terms with their NEAR wallet (NEP-413) and the client with the approver wallet, so the contract verifies two more signatures per escrow.
- If the creator approves too late, the other party's signature expires and they must submit again.
- As before, the contract cannot prove that the approver is not a second wallet owned by the freelancer. The creator sees the other party's addresses before approving, and the client escrow page shows the approver.
