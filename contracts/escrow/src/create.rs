//! Create and cancel escrows (R-01).

use crate::events::VecrowEvent;
use crate::split::validate_split;
use crate::*;
use near_sdk::json_types::U128;

#[near]
impl VecrowEscrow {
    /// Creates an escrow for the caller (the freelancer) with the given
    /// milestone amounts (USDC base units) and revenue split. The fee and the
    /// review window are copied from the configuration and locked for the
    /// lifetime of the escrow. Returns the new escrow ID.
    ///
    /// Split recipients must already be registered on the USDC contract; the
    /// backend does this before calling (R-20). A recipient that is not is
    /// still safe: its payouts are kept as pending until `retry_payout`.
    pub fn create_escrow(&mut self, milestones: Vec<U128>, split: Vec<SplitShare>) -> String {
        require!(!milestones.is_empty(), "at least one milestone is required");
        require!(milestones.len() <= MAX_MILESTONES, "too many milestones");
        let mut total: u128 = 0;
        for amount in &milestones {
            require!(amount.0 > 0, "milestone amounts must be positive");
            total = total
                .checked_add(amount.0)
                .unwrap_or_else(|| env::panic_str("milestone total overflows"));
        }
        validate_split(&split);

        let id = self.next_escrow_id;
        self.next_escrow_id += 1;
        let freelancer = env::predecessor_account_id();
        let escrow = Escrow {
            freelancer: freelancer.clone(),
            milestones: milestones
                .into_iter()
                .map(|amount| Milestone {
                    amount,
                    status: MilestoneStatus::Pending,
                    submitted_at_ms: None,
                    revision_count: 0,
                })
                .collect(),
            split,
            fee_bps: self.config.fee_bps,
            review_window_sec: self.config.review_window_sec,
            status: EscrowStatus::Created,
            created_at_ms: env::block_timestamp_ms(),
        };
        self.save_escrow(id, escrow);

        let escrow_id = id.to_string();
        VecrowEvent::EscrowCreated {
            escrow_id: escrow_id.clone(),
            freelancer,
            total: U128(total),
        }
        .emit();
        escrow_id
    }

    /// Cancels an escrow that is not funded yet (`Created` or
    /// `ApproverBound`). Only the freelancer can cancel.
    pub fn cancel(&mut self, escrow_id: String) {
        let (id, mut escrow) = self.load_escrow(&escrow_id);
        require_freelancer(&escrow);
        require!(
            matches!(
                escrow.status,
                EscrowStatus::Created | EscrowStatus::ApproverBound
            ),
            "only an unfunded escrow can be cancelled"
        );
        escrow.status = EscrowStatus::Cancelled;
        self.save_escrow(id, escrow);
        VecrowEvent::EscrowCancelled { escrow_id }.emit();
    }
}
