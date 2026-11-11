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
| [D-15](#d-15--deploy-time-configuration-with-no-setter-checked-and-locked-per-escrow) | Deploy-time configuration with no setter, checked and locked per escrow | 2026-11-11 | Accepted | R-01, R-07, R-19 |
| [D-16](#d-16--nep-413-signatures-are-checked-against-the-key-given-with-them) | NEP-413 signatures are checked against the key given with them | 2026-11-11 | Accepted | R-01, R-04, R-06, R-17 |
| [D-17](#d-17--signed-payloads-are-plain-text-rebuilt-by-the-contract) | Signed payloads are plain text rebuilt by the contract | 2026-11-11 | Accepted | R-01, R-06, R-17, R-23 |
| [D-18](#d-18--approver-and-refund-account-rules-cover-implicit-accounts-and-keys) | Approver and refund-account rules cover implicit accounts and keys | 2026-11-11 | Accepted | R-04 |
| [D-19](#d-19--one-transfer-per-payout-paid-for-by-the-contract) | One transfer per payout, paid for by the contract | 2026-11-11 | Accepted | R-02, R-03, R-18, R-19, R-20 |
| [D-20](#d-20--contract-toolchain-and-generated-wallet-fixtures) | Contract toolchain and generated wallet fixtures | 2026-11-11 | Accepted | R-06 |

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

## D-15 — Deploy-time configuration with no setter, checked and locked per escrow

**Context.** R-19 requires that terms signed with an outdated `fee_bps` are rejected and
that a configuration change does not affect escrows already on-chain. R-01 says
`create_escrow` stores the review window. The contract methods are fixed by
architecture.md §5.1, which has no administrative method, and any key that can change the
configuration of a live contract can also change where fees go or which token is paid.

**Decision.**
- `new(usdc_contract_id, treasury_account, fee_bps, review_window_sec)` sets the
  configuration once. There is no owner and no setter; the configuration changes only by
  redeploying the contract code with a state migration.
- `create_escrow` rejects terms whose `fee_bps` differs from the configuration, then
  copies `fee_bps` and `review_window_sec` into the escrow. Releases use the escrow's
  values, never the current configuration.
- `new` rejects `fee_bps` above 10,000, a review window of 0, and the contract itself as
  the treasury.

**Alternatives.**
- An owner-only setter: convenient for the demo, but it adds a privileged key to a
  contract that holds client funds.
- Taking `fee_bps` from the terms without checking it: both parties could sign a fee of 0.

**Consequences.**
- A shorter demo window (R-07) is a different deployment, not a call.
- After a redeploy with a new fee, the backend must have the parties sign again any terms
  it has not put on-chain yet.
- The treasury and the USDC contract are global: a redeploy that changes the treasury
  affects future releases of existing escrows; only `fee_bps` and the window are locked.
- The unit tests simulate a redeploy by changing the stored configuration.

## D-16 — NEP-413 signatures are checked against the key given with them

**Context.** A NEP-413 signature is checked against an ed25519 public key, but a NEAR
account ID is a name: a contract cannot read another account's access keys, so it cannot
check that a key belongs to an account. Both the freelancer (always) and a client with a
NEAR wallet sign with NEP-413.

**Decision.**
- The freelancer's signature of the terms comes with the `public_key` the wallet returns.
  A NEAR approver is part of the terms as `{account_id, public_key}`, and every later
  approval, revision or cancellation must be signed with that key.
- When the account is an implicit account (64 hex characters), its ID must be the hex of
  the key; the contract checks this for the freelancer and treats an approver's implicit
  account as the approver (D-18).
- For named accounts, the backend checks over RPC that the key is a full access key of the
  account before it calls `create_escrow`, as it does for login (architecture.md §7.5).

**Alternatives.**
- Accept only implicit accounts, whose ID is the key: no label to trust, but named
  accounts such as `alice.near` could not take part.
- Have the freelancer call `create_escrow` through a meta transaction, which the runtime
  verifies against the account's keys: not possible when the client creates the escrow,
  because the freelancer signs before the client's signature exists (D-14).

**Consequences.**
- If the client deletes or rotates the bound key, they cannot approve, request revisions
  or cancel. The freelancer is still paid by auto-release, and refunds do not need the
  approver.
- For a named freelancer account, a compromised backend could sign the freelancer's side
  with a key it holds. It still needs the client's signature over the same terms, so the
  amounts, split, approver and refund account are what the client signed, and the named
  freelancer only gains rights by acting from their own account. The contract cannot
  close this gap; it is listed as a limitation.

## D-17 — Signed payloads are plain text rebuilt by the contract

**Context.** Three wallet families sign differently (D-02). A signature must not be
replayable on another deployment, escrow, milestone or action, wallets should show the
signer what they sign, and the backend chooses the escrow ID before the escrow is
on-chain (D-14).

**Decision.**
- The contract builds each message from the call arguments: a first line
  `Vecrow {method}`, then one `Key: value` line per field, separated by `\n`
  (architecture.md §5.3). Both parties sign the same terms text; NEP-413 uses the
  contract account as `recipient`.
- Terms carry no nonce: the escrow ID can be used only once and the terms expire at their
  deadline. Their NEP-413 nonce is `sha256(message)`.
- Actions carry a nonce of 32 random bytes, written as 64 lowercase hex characters, that
  each approver can use once across all escrows and actions; the contract stores
  `sha256(borsh(approver key) ‖ nonce)`.
- Deadlines are milliseconds since the Unix epoch, valid while the block time is at most
  the deadline. Milestones are 0-based indexes. Escrow IDs are 1 to 64 characters of
  `a-z`, `0-9`, `-` and `_`, chosen by the backend.
- EIP-191 signatures with a high `s` value are rejected (EIP-2); `v` may be 27/28 or 0/1.

**Alternatives.**
- A JSON payload: needs a canonical serialization on every client to sign the same bytes.
- EIP-712 typed data: only for EVM wallets.
- A per-approver counter instead of random nonces: less storage, but two signed actions
  made in parallel could only be submitted in order.
- Escrow IDs assigned by the contract: the terms could not name the escrow before it is
  on-chain.

**Consequences.**
- The backend and the frontend have to produce the messages byte for byte; the format is
  specified in architecture.md and reproduced by `contracts/wallet-fixtures`.
- Wallets show readable text, including the contract account and the action.
- Each used nonce costs 73 bytes of contract storage.

## D-18 — Approver and refund-account rules cover implicit accounts and keys

**Context.** R-04 rejects an approver or refund account equal to the freelancer or a
split recipient. On NEAR, the same key or address can also appear as an implicit account
(64 hex characters of an ed25519 key) or an eth-implicit account (`0x` + 40 hex of an EVM
address), and a NEAR and a Solana wallet can hold the same ed25519 key.

**Decision.** `create_escrow` rejects:
- an approver whose NEAR account, the implicit account of its ed25519 key, or the
  eth-implicit account of its EVM address is the freelancer or a split recipient;
- an approver holding the ed25519 key the freelancer signed the terms with;
- a refund account that is the freelancer, a split recipient, or the contract;
- an implicit freelancer account that signs with another key.

The refund account may be the client's own NEAR approver account. No method changes the
approver or the refund account after `create_escrow`.

**Consequences.**
- The checks are stricter than comparing strings, but they still cannot detect a second
  wallet of the freelancer (D-14).

## D-19 — One transfer per payout, paid for by the contract

**Context.** NEP-141 `ft_transfer` requires exactly 1 yoctoNEAR, a positive amount and a
registered receiver. Users hold no NEAR (goal 5), and one unregistered recipient must not
block the others (R-20).

**Decision.**
- Each payout is its own `ft_transfer` with its own `on_payout` callback: the fee, every
  split share, and a refund (the sum of the refunded milestones, in one transfer). Zero
  amounts are skipped.
- A failed transfer adds its amount to the escrow's `pending_payouts` for that account.
  `retry_payout` removes the amount before sending and `on_payout` restores it only on
  failure (D-07).
- Storage and the 1 yoctoNEAR per transfer are paid from the contract's NEAR balance. No
  method is payable.
- Rejected or excess deposits are returned through the `ft_on_transfer` return value
  rather than a panic, with the reason logged.
- An escrow has at most 20 milestones and 10 split recipients, so a release (up to 11
  transfers, 20 TGas reserved for each transfer and its callback) fits in one 300 TGas
  call.

**Alternatives.**
- All transfers of a release in one batch receipt: fewer receipts, but one unregistered
  account would fail the whole batch.
- Recipients claim their own funds: they would need NEAR to send the claim.

**Consequences.**
- The contract account must keep a NEAR balance for storage: about 0.003 NEAR per escrow
  and 0.0007 NEAR per signed action. Anyone holding two wallets can sign terms and create
  escrows, so storage can be consumed by spam; the size limits bound each escrow, and the
  balance has to be monitored.
- The relayer and the jobs must attach 300 TGas to `approve_milestone` and `auto_release`.

## D-20 — Contract toolchain and generated wallet fixtures

**Context.** The contract has to be built reproducibly and tested both as plain logic and
across contracts (D-11). D-02 calls for test fixtures signed by real wallets, but those
can only be captured by hand from the browser extensions.

**Decision.**
- Rust 1.97.0 is pinned in `contracts/rust-toolchain.toml`; the contract uses near-sdk
  5.30 and is built with cargo-near 0.23 (`non-reproducible-wasm`, which runs `wasm-opt`).
- Logic is unit-tested on near-sdk's mocked blockchain. Token transfers, callbacks,
  retries and registration are tested with near-workspaces against the built wasm on a
  local sandbox node, with a mock NEP-141 token.
- Signature fixtures are generated in the tests with ed25519-dalek, k256 and borsh, in the
  exact byte formats the wallets use (architecture.md §5.3); known vectors check the
  derivations. The NEP-366 meta transaction test signs delegate actions with
  near-primitives, because near-workspaces has no builder for them.

**Consequences.**
- R-06 stays Partial until signatures captured from a NEAR wallet, MetaMask and Phantom are
  added as fixtures.
- The sandbox binary is downloaded on the first test run. nearcore raises its open-file
  limit to 65,535 at startup, so the environment's hard limit must allow it (README).

