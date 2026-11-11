//! NEP-297 events, logged as `EVENT_JSON:{"standard":"vecrow","version":"1.0.0","event":...,"data":...}`.
//!
//! The backend can index these logs for the dashboard and the CSV export; the
//! contract state stays the source of truth.

use crate::signature::Approver;
use near_sdk::json_types::U128;
use near_sdk::{AccountId, near};

#[near(event_json(standard = "vecrow"))]
pub enum VecrowEvent {
    #[event_version("1.0.0")]
    EscrowCreated {
        escrow_id: String,
        freelancer: AccountId,
        approver: Approver,
        refund_account: AccountId,
        total: U128,
    },
    #[event_version("1.0.0")]
    EscrowCancelled { escrow_id: String },
    #[event_version("1.0.0")]
    EscrowFunded {
        escrow_id: String,
        sender_id: AccountId,
        amount: U128,
    },
    #[event_version("1.0.0")]
    MilestoneSubmitted {
        escrow_id: String,
        milestone: u32,
        submitted_at_ms: u64,
    },
    #[event_version("1.0.0")]
    RevisionRequested {
        escrow_id: String,
        milestone: u32,
        revision_count: u8,
    },
    #[event_version("1.0.0")]
    MilestoneReleased {
        escrow_id: String,
        milestone: u32,
        amount: U128,
        fee: U128,
        /// `approve_milestone` or `auto_release`.
        by: String,
    },
    #[event_version("1.0.0")]
    MilestonesRefunded {
        escrow_id: String,
        milestones: Vec<u32>,
        amount: U128,
        refund_account: AccountId,
    },
    /// The result of one `ft_transfer`; unpaid amounts become pending.
    #[event_version("1.0.0")]
    Payout {
        escrow_id: String,
        account_id: AccountId,
        amount: U128,
        paid: bool,
    },
    #[event_version("1.0.0")]
    EscrowCompleted { escrow_id: String },
}
