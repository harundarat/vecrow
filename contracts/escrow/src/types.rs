//! Escrow state and the JSON views returned by `get_escrow`.

use crate::signature::{Approver, ApproverKey};
use near_sdk::json_types::U128;
use near_sdk::{AccountId, near};

/// Escrow lifecycle (spec §6). Serialized as PascalCase strings, identical in
/// the contract, the API and the documentation (D-10).
#[near(serializers = [borsh, json])]
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum EscrowStatus {
    /// Created; no approver and no funds yet.
    Created,
    /// Approver bound; waiting for funds.
    ApproverBound,
    /// USDC received; approver locked.
    Funded,
    /// Every milestone is `Released` or `Refunded`.
    Completed,
    /// Cancelled before funding.
    Cancelled,
}

/// Milestone lifecycle (spec §6). Serialized as PascalCase strings.
#[near(serializers = [borsh, json])]
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum MilestoneStatus {
    /// Not submitted yet.
    Pending,
    /// Review window running.
    Submitted,
    /// The client requested a revision; the timer is paused until re-submission.
    RevisionRequested,
    /// Paid out to the treasury and the split recipients.
    Released,
    /// Returned to the client's `refund_account`.
    Refunded,
}

/// One split recipient and its share of each release, in basis points.
#[near(serializers = [borsh, json])]
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct SplitShare {
    pub account_id: AccountId,
    pub bps: u16,
}

/// USDC owed to an account after a failed transfer, re-sent by
/// `retry_payout` (R-20).
#[near(serializers = [borsh, json])]
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct PendingPayout {
    pub account_id: AccountId,
    pub amount: U128,
}

/// A milestone as stored and as returned by `get_escrow`.
#[near(serializers = [borsh, json])]
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Milestone {
    /// USDC base units.
    pub amount: U128,
    pub status: MilestoneStatus,
    /// Block time of the latest submission, in milliseconds since the Unix
    /// epoch. `None` until submitted, and cleared when a revision is
    /// requested, which stops the auto-release timer.
    pub submitted_at_ms: Option<u64>,
    /// Revision requests so far (max 2, R-17).
    pub revision_count: u8,
}

/// An escrow as stored in contract state.
#[near(serializers = [borsh])]
#[derive(Clone, Debug)]
pub struct Escrow {
    pub freelancer: AccountId,
    pub milestones: Vec<Milestone>,
    pub split: Vec<SplitShare>,
    /// Platform fee locked at `create_escrow` (R-19).
    pub fee_bps: u16,
    /// Review window locked at `create_escrow` (R-07).
    pub review_window_sec: u64,
    /// Set once by `bind_approver` (R-04), then locked.
    pub approver: Option<ApproverKey>,
    /// The client's NEAR account for refunds, set with the approver.
    pub refund_account: Option<AccountId>,
    pub status: EscrowStatus,
    /// USDC held for milestones that are neither released nor refunded.
    /// Equals the total once funded, and decreases with each payout.
    pub balance: u128,
    /// Failed transfers per account, waiting for `retry_payout` (R-20).
    pub pending_payouts: Vec<PendingPayout>,
    pub created_at_ms: u64,
    pub funded_at_ms: Option<u64>,
}

impl Escrow {
    /// Sum of all milestone amounts: the amount that funds the escrow.
    pub fn total(&self) -> u128 {
        self.milestones.iter().map(|m| m.amount.0).sum()
    }
}

/// JSON view of an escrow returned by `get_escrow`.
#[near(serializers = [json])]
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct EscrowView {
    pub escrow_id: String,
    pub freelancer: AccountId,
    pub status: EscrowStatus,
    pub milestones: Vec<Milestone>,
    pub split: Vec<SplitShare>,
    pub fee_bps: u16,
    pub review_window_sec: u64,
    pub approver: Option<Approver>,
    pub refund_account: Option<AccountId>,
    /// Sum of all milestone amounts.
    pub total: U128,
    /// USDC still held for unreleased, unrefunded milestones.
    pub balance: U128,
    /// Released or refunded USDC whose transfer failed, per account.
    pub pending_payouts: Vec<PendingPayout>,
    pub created_at_ms: u64,
    pub funded_at_ms: Option<u64>,
}

impl EscrowView {
    pub fn new(escrow_id: u64, escrow: &Escrow) -> Self {
        Self {
            escrow_id: escrow_id.to_string(),
            freelancer: escrow.freelancer.clone(),
            status: escrow.status,
            milestones: escrow.milestones.clone(),
            split: escrow.split.clone(),
            fee_bps: escrow.fee_bps,
            review_window_sec: escrow.review_window_sec,
            approver: escrow.approver.as_ref().map(ApproverKey::to_approver),
            refund_account: escrow.refund_account.clone(),
            total: U128(escrow.total()),
            balance: U128(escrow.balance),
            pending_payouts: escrow.pending_payouts.clone(),
            created_at_ms: escrow.created_at_ms,
            funded_at_ms: escrow.funded_at_ms,
        }
    }
}
