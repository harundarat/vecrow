//! Milestone lifecycle: submit (R-05) and signed approval (R-06).

use crate::events::VecrowEvent;
use crate::payout::ReleaseTrigger;
use crate::signature::{MilestoneAction, milestone_message, parse_nonce};
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

#[near]
impl VecrowEscrow {
    /// Approves a `Submitted` milestone with a payload signed by the
    /// approver (R-06). Forwarded by the relayer; the caller gains no rights.
    /// The milestone becomes `Released`: the fee goes to the treasury and
    /// the rest to the split recipients. Attach 300 TGas so every payout and
    /// its callback have gas.
    pub fn approve_milestone(
        &mut self,
        escrow_id: String,
        milestone: u32,
        nonce: String,
        deadline_ms: u64,
        signature: String,
    ) {
        let (id, mut escrow) = self.load_escrow(&escrow_id);
        self.accept_milestone_action(
            id,
            &escrow,
            MilestoneAction::Approve,
            milestone,
            &nonce,
            deadline_ms,
            &signature,
        );
        self.release_milestone(id, &mut escrow, milestone, ReleaseTrigger::Approval);
        self.save_escrow(id, escrow);
    }
}

impl VecrowEscrow {
    /// Checks a signed milestone action: a `Funded` escrow, a `Submitted`
    /// milestone, and a valid, unexpired, unused signature by the approver.
    #[allow(clippy::too_many_arguments)]
    fn accept_milestone_action(
        &mut self,
        id: u64,
        escrow: &Escrow,
        action: MilestoneAction,
        milestone: u32,
        nonce: &str,
        deadline_ms: u64,
        signature: &str,
    ) {
        require_funded(escrow);
        let status = escrow
            .milestones
            .get(milestone as usize)
            .unwrap_or_else(|| env::panic_str("unknown milestone"))
            .status;
        require!(
            status == MilestoneStatus::Submitted,
            "only a Submitted milestone can be approved or sent back for revision"
        );
        let approver = escrow
            .approver
            .as_ref()
            .unwrap_or_else(|| env::panic_str("escrow has no approver"));
        let nonce = parse_nonce(nonce).unwrap_or_else(|err| env::panic_str(err));
        let message = milestone_message(
            action,
            &env::current_account_id(),
            id,
            milestone,
            &nonce,
            deadline_ms,
        );
        self.accept_signed_action(approver, &message, &nonce, deadline_ms, signature);
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
