use crate::signature::{Approver, ApproverKey};
use near_sdk::json_types::U128;
use near_sdk::{AccountId, near};

#[near(serializers = [borsh, json])]
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum EscrowStatus {
    Created,
    Funded,
    Completed,
    Cancelled,
}

#[near(serializers = [borsh, json])]
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum MilestoneStatus {
    Pending,
    Submitted,
    RevisionRequested,
    Released,
    Refunded,
}

#[near(serializers = [borsh, json])]
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct SplitShare {
    pub account_id: AccountId,
    pub bps: u16,
}

#[near(serializers = [borsh, json])]
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct PendingPayout {
    pub account_id: AccountId,
    pub amount: U128,
}

#[near(serializers = [borsh, json])]
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Milestone {
    pub amount: U128,
    pub status: MilestoneStatus,
    pub submitted_at_ms: Option<u64>,
    pub revision_count: u8,
}

#[near(serializers = [borsh])]
#[derive(Clone, Debug)]
pub struct Escrow {
    pub freelancer: AccountId,
    pub milestones: Vec<Milestone>,
    pub split: Vec<SplitShare>,
    pub fee_bps: u16,
    pub review_window_sec: u64,
    pub approver: ApproverKey,
    pub refund_account: AccountId,
    pub status: EscrowStatus,
    pub balance: u128,
    pub pending_payouts: Vec<PendingPayout>,
    pub created_at_ms: u64,
    pub funded_at_ms: Option<u64>,
}

impl Escrow {
    pub fn total(&self) -> u128 {
        self.milestones.iter().map(|m| m.amount.0).sum()
    }
}

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
    pub approver: Approver,
    pub refund_account: AccountId,
    pub total: U128,
    pub balance: U128,
    pub pending_payouts: Vec<PendingPayout>,
    pub created_at_ms: u64,
    pub funded_at_ms: Option<u64>,
}

impl EscrowView {
    pub fn new(escrow_id: &str, escrow: &Escrow) -> Self {
        Self {
            escrow_id: escrow_id.to_string(),
            freelancer: escrow.freelancer.clone(),
            status: escrow.status,
            milestones: escrow.milestones.clone(),
            split: escrow.split.clone(),
            fee_bps: escrow.fee_bps,
            review_window_sec: escrow.review_window_sec,
            approver: escrow.approver.to_approver(),
            refund_account: escrow.refund_account.clone(),
            total: U128(escrow.total()),
            balance: U128(escrow.balance),
            pending_payouts: escrow.pending_payouts.clone(),
            created_at_ms: escrow.created_at_ms,
            funded_at_ms: escrow.funded_at_ms,
        }
    }
}
