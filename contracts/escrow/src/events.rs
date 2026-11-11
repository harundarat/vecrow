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
        total: U128,
    },
    #[event_version("1.0.0")]
    EscrowCancelled { escrow_id: String },
    #[event_version("1.0.0")]
    ApproverBound {
        escrow_id: String,
        approver: Approver,
        refund_account: AccountId,
    },
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
}
