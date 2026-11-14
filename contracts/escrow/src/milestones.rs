use crate::events::VecrowEvent;
use crate::payout::ReleaseTrigger;
use crate::signature::{Action, action_message, parse_nonce};
use crate::*;

#[near]
impl VecrowEscrow {
    pub fn submit_milestone(&mut self, escrow_id: String, milestone: u32) {
        let mut escrow = self.load_escrow(&escrow_id);
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
        self.save_escrow(&escrow_id, escrow);
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
    pub fn approve_milestone(
        &mut self,
        escrow_id: String,
        milestone: u32,
        nonce: String,
        deadline_ms: u64,
        signature: String,
    ) {
        let mut escrow = self.load_escrow(&escrow_id);
        self.accept_milestone_action(
            &escrow_id,
            &escrow,
            Action::Approve,
            milestone,
            &nonce,
            deadline_ms,
            &signature,
        );
        self.release_milestone(&escrow_id, &mut escrow, milestone, ReleaseTrigger::Approval);
        self.save_escrow(&escrow_id, escrow);
    }

    pub fn request_revision(
        &mut self,
        escrow_id: String,
        milestone: u32,
        nonce: String,
        deadline_ms: u64,
        signature: String,
    ) {
        let mut escrow = self.load_escrow(&escrow_id);
        let revisions = escrow
            .milestones
            .get(milestone as usize)
            .filter(|m| m.status == MilestoneStatus::Submitted)
            .map(|m| m.revision_count);
        require!(
            revisions.is_none_or(|count| count < MAX_REVISIONS),
            "revision limit reached: a milestone can be sent back at most 2 times"
        );
        self.accept_milestone_action(
            &escrow_id,
            &escrow,
            Action::RequestRevision,
            milestone,
            &nonce,
            deadline_ms,
            &signature,
        );
        let m = milestone_mut(&mut escrow, milestone);
        m.status = MilestoneStatus::RevisionRequested;
        m.revision_count += 1;
        m.submitted_at_ms = None;
        let revision_count = m.revision_count;
        self.save_escrow(&escrow_id, escrow);
        VecrowEvent::RevisionRequested {
            escrow_id,
            milestone,
            revision_count,
        }
        .emit();
    }

    pub fn auto_release(&mut self, escrow_id: String, milestone: u32) {
        let mut escrow = self.load_escrow(&escrow_id);
        require_funded(&escrow);
        let window_ms = escrow.review_window_sec.saturating_mul(1_000);
        let m = escrow
            .milestones
            .get(milestone as usize)
            .unwrap_or_else(|| env::panic_str("unknown milestone"));
        match m.status {
            MilestoneStatus::Submitted => {}
            MilestoneStatus::RevisionRequested => {
                env::panic_str("a milestone awaiting revision cannot be auto-released")
            }
            _ => env::panic_str("only a Submitted milestone can be auto-released"),
        }
        let submitted_at_ms = m
            .submitted_at_ms
            .unwrap_or_else(|| env::panic_str("milestone has no submission time"));
        require!(
            env::block_timestamp_ms() >= submitted_at_ms.saturating_add(window_ms),
            "review window has not ended"
        );
        self.release_milestone(
            &escrow_id,
            &mut escrow,
            milestone,
            ReleaseTrigger::AutoRelease,
        );
        self.save_escrow(&escrow_id, escrow);
    }
}

impl VecrowEscrow {
    #[allow(clippy::too_many_arguments)]
    fn accept_milestone_action(
        &mut self,
        escrow_id: &str,
        escrow: &Escrow,
        action: Action,
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
        let nonce = parse_nonce(nonce).unwrap_or_else(|err| env::panic_str(err));
        let message = action_message(
            action,
            &env::current_account_id(),
            escrow_id,
            Some(milestone),
            &nonce,
            deadline_ms,
        );
        self.accept_signed_action(&escrow.approver, &message, &nonce, deadline_ms, signature);
    }
}

fn require_funded(escrow: &Escrow) {
    require!(
        escrow.status == EscrowStatus::Funded,
        "escrow is not funded"
    );
}

fn milestone_mut(escrow: &mut Escrow, index: u32) -> &mut Milestone {
    escrow
        .milestones
        .get_mut(index as usize)
        .unwrap_or_else(|| env::panic_str("unknown milestone"))
}
