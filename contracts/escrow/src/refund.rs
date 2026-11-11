//! Refund unsubmitted milestones to the client (R-18, D-04).

use crate::events::VecrowEvent;
use crate::payout::{complete_if_done, require_payout_gas};
use crate::*;
use near_sdk::json_types::U128;

#[near]
impl VecrowEscrow {
    /// Refunds `Pending` milestones of a `Funded` escrow to the client's
    /// `refund_account`: the given milestone, or every `Pending` one when
    /// `milestone` is omitted. Only the freelancer can call it; the client
    /// cannot. Milestones become `Refunded`, no fee is charged, and the amount
    /// is sent in one transfer.
    pub fn refund_unsubmitted(&mut self, escrow_id: String, milestone: Option<u32>) {
        let mut escrow = self.load_escrow(&escrow_id);
        require_freelancer(&escrow);
        require!(
            escrow.status == EscrowStatus::Funded,
            "escrow is not funded"
        );

        let indices: Vec<u32> = match milestone {
            Some(index) => {
                let m = escrow
                    .milestones
                    .get(index as usize)
                    .unwrap_or_else(|| env::panic_str("unknown milestone"));
                require!(
                    m.status == MilestoneStatus::Pending,
                    "only a Pending milestone can be refunded"
                );
                vec![index]
            }
            None => (0..escrow.milestones.len() as u32)
                .filter(|&i| escrow.milestones[i as usize].status == MilestoneStatus::Pending)
                .collect(),
        };
        require!(!indices.is_empty(), "no Pending milestone to refund");

        let mut amount: u128 = 0;
        for &index in &indices {
            let m = &mut escrow.milestones[index as usize];
            m.status = MilestoneStatus::Refunded;
            amount += m.amount.0;
        }
        escrow.balance = escrow
            .balance
            .checked_sub(amount)
            .unwrap_or_else(|| env::panic_str("escrow balance is too low"));
        let refund_account = escrow.refund_account.clone();
        require_payout_gas(1);

        VecrowEvent::MilestonesRefunded {
            escrow_id: escrow_id.clone(),
            milestones: indices,
            amount: U128(amount),
            refund_account: refund_account.clone(),
        }
        .emit();
        self.payout(&escrow_id, &refund_account, amount).detach();
        complete_if_done(&escrow_id, &mut escrow);
        self.save_escrow(&escrow_id, escrow);
    }
}
