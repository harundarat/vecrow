//! Milestone lifecycle: submit (R-05).

use crate::events::VecrowEvent;
use crate::*;

#[near]
impl VecrowEscrow {
    /// Submits a milestone (0-based index) for review. Only the freelancer,
    /// only on a `Funded` escrow, and only from `Pending` or
    /// `RevisionRequested`. Each submission starts a new review window.
    pub fn submit_milestone(&mut self, escrow_id: String, milestone: u32) {
        let (id, mut escrow) = self.load_escrow(&escrow_id);
        require_freelancer(&escrow);
        require_funded(&escrow);
        let now = env::block_timestamp_ms();
        let m = milestone_mut(&mut escrow, milestone);
        require!(
            matches!(
                m.status,
                MilestoneStatus::Pending | MilestoneStatus::RevisionRequested
            ),
            "only a Pending or RevisionRequested milestone can be submitted"
        );
        m.status = MilestoneStatus::Submitted;
        m.submitted_at_ms = Some(now);
        self.save_escrow(id, escrow);
        VecrowEvent::MilestoneSubmitted {
            escrow_id,
            milestone,
            submitted_at_ms: now,
        }
        .emit();
    }
}

fn require_funded(escrow: &Escrow) {
    require!(
        escrow.status == EscrowStatus::Funded,
        "escrow is not funded"
    );
}

/// The milestone at `index`, or a panic.
fn milestone_mut(escrow: &mut Escrow, index: u32) -> &mut Milestone {
    escrow
        .milestones
        .get_mut(index as usize)
        .unwrap_or_else(|| env::panic_str("unknown milestone"))
}
