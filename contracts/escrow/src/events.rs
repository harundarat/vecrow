//! NEP-297 events, logged as `EVENT_JSON:{"standard":"vecrow","version":"1.0.0","event":...,"data":...}`.
//!
//! The backend can index these logs for the dashboard and the CSV export; the
//! contract state stays the source of truth.

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
}
