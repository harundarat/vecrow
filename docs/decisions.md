# Vecrow — Design Decisions

This document records the main design decisions behind Vecrow, why they were made,
and what they cost. Requirements (`R-xx`) are defined in [`spec.md`](./spec.md).

Each decision lists its context, the decision itself, the alternatives, and the
consequences, including the trade-offs we accept.

| ID | Decision | Date | Status | Requirements |
| --- | --- | --- | --- | --- |
| [D-01](#d-01--the-client-binds-the-approver-at-payment-time) | The client binds the approver at payment time | 2026-11-07 | Accepted | R-04, R-10, R-15 |
| [D-02](#d-02--approvals-are-verified-on-chain-from-wallet-signatures) | Approvals are verified on-chain from wallet signatures | 2026-11-07 | Accepted | R-06, R-09, R-17, R-21 |
| [D-03](#d-03--capped-revisions-and-auto-release-instead-of-rejection-or-arbitration) | Capped revisions and auto-release instead of rejection or arbitration | 2026-11-08 | Accepted | R-07, R-17 |
| [D-04](#d-04--only-the-freelancer-can-refund-and-only-unsubmitted-milestones) | Only the freelancer can refund, and only unsubmitted milestones | 2026-11-08 | Accepted | R-04, R-18 |
| [D-05](#d-05--funding-amount-rules-and-exact_output-quotes) | Funding amount rules and `EXACT_OUTPUT` quotes | 2026-11-08 | Accepted | R-02, R-08 |
| [D-06](#d-06--a-05-fee-locked-per-escrow-with-deterministic-rounding) | A 0.5% fee locked per escrow, with deterministic rounding | 2026-11-08 | Accepted | R-03, R-19 |
| [D-07](#d-07--the-backend-registers-storage-failed-payouts-are-retried) | The backend registers storage; failed payouts are retried | 2026-11-08 | Accepted | R-20 |
| [D-08](#d-08--accounts-are-for-identity-and-visibility-never-for-authority) | Accounts are for identity and visibility, never for authority | 2026-11-08 | Accepted | R-13 – R-16, R-21, R-22 |
| [D-09](#d-09--funds-arrive-through-1click-directly-into-the-contract) | Funds arrive through 1Click directly into the contract | 2026-11-08 | Pending mainnet test | R-08 |
| [D-10](#d-10--one-set-of-status-names-across-contract-and-api) | One set of status names across contract and API | 2026-11-08 | Accepted | — |
| [D-11](#d-11--testnet-for-contract-development-mainnet-for-1click) | Testnet for contract development, mainnet for 1Click | 2026-11-07 | Accepted | R-08 |

---

## D-01 — The client binds the approver at payment time

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

**Decision.** Approvals (R-06) and revision requests (R-17) are payloads signed by the
approver's own wallet, with a nonce and a deadline. The contract verifies the
signature itself, for three schemes:

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
`refund_account`, set when the approver was bound (D-01). Refunds are not charged a
fee (D-06).

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
- Less than the total: returned in full; the escrow stays `ApproverBound`.
- Equal to or more than the total: the escrow becomes `Funded` with exactly the total, and `amount - total` is returned through the NEP-141 return value.
- Transfers to an escrow that is not `ApproverBound`, of another token, or with an unknown `escrow_id`: returned in full.

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
- `fee_bps = 50` is stored per escrow at `create_escrow`; later configuration changes do not affect existing escrows (R-19).
- On each release, `fee = floor(amount × fee_bps / 10,000)` goes to `treasury_account`; the remainder is split by bps (R-03).
- Split rounding remainders go to the first recipient.
- The contract asserts that fee + all shares = milestone amount.
- No fee is charged on refunds (R-18) or on returned deposits (R-02).

**Example.** A 1,000 USDC milestone with a 70/30 split: 5 to treasury, 696.5 and 298.5 to the recipients.

**Consequences.**
- Fees are predictable for both parties and are shown on the dashboard and the payment page.
- The first split recipient may receive up to a few base units more than their exact share.

## D-07 — The backend registers storage; failed payouts are retried

**Context.** On NEAR, an account must be registered on the USDC contract
(`storage_deposit`) before it can receive tokens. A transfer to an unregistered account
fails, and in an escrow that would leave funds stuck. Users should not need NEAR to
register.

**Decision.** (R-20)
- The escrow contract and `treasury_account` are registered at deploy time.
- The backend checks `storage_balance_of` and pays `storage_deposit` (`registration_only = true`) from a backend account: split recipients before `create_escrow`, `refund_account` before binding the approver.
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
- The client can pay without an account. Binding the approver returns a guest token for one escrow and one wallet, valid for 24 hours (R-21).
- Email OTP login is a recovery-only session: it can link a new wallet (still proven by that wallet's signature) and unlink old ones, but cannot create escrows or change client invitations (R-22).
- The contract never reads accounts. Submit, approve, revise, refund, and payouts depend only on wallet keys and signatures.

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

**Decision.** The contract enums `EscrowStatus { Created, ApproverBound, Funded,
Completed, Cancelled }` and `MilestoneStatus { Pending, Submitted, RevisionRequested,
Released, Refunded }` are serialized as PascalCase strings. The API, mock data, and
documentation use the same strings with no mapping. 1Click funding status (`PENDING`,
`SUCCESS`, `REFUNDED`) is kept separate and appears only in
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
