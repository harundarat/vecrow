use crate::events::VecrowEvent;
use crate::split::release_plan;
use crate::*;
use near_sdk::json_types::U128;
use near_sdk::{Gas, NearToken, Promise, ext_contract, is_promise_success};

pub const GAS_FT_TRANSFER: Gas = Gas::from_tgas(10);
pub const GAS_ON_PAYOUT: Gas = Gas::from_tgas(10);
const GAS_RESERVE: Gas = Gas::from_tgas(10);

#[ext_contract(ext_ft)]
#[allow(dead_code)] // Only the generated `ext_ft::ext` client is used.
trait FungibleTokenCore {
    fn ft_transfer(&mut self, receiver_id: AccountId, amount: U128, memo: Option<String>);
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum ReleaseTrigger {
    Approval,
    AutoRelease,
}

impl ReleaseTrigger {
    fn method(self) -> &'static str {
        match self {
            Self::Approval => "approve_milestone",
            Self::AutoRelease => "auto_release",
        }
    }
}

#[near]
impl VecrowEscrow {
    #[private]
    pub fn on_payout(&mut self, escrow_id: String, account_id: AccountId, amount: U128) -> bool {
        let paid = is_promise_success();
        if !paid {
            let mut escrow = self.load_escrow(&escrow_id);
            match escrow
                .pending_payouts
                .iter_mut()
                .find(|p| p.account_id == account_id)
            {
                Some(pending) => pending.amount = U128(pending.amount.0 + amount.0),
                None => escrow.pending_payouts.push(PendingPayout {
                    account_id: account_id.clone(),
                    amount,
                }),
            }
            self.save_escrow(&escrow_id, escrow);
        }
        VecrowEvent::Payout {
            escrow_id,
            account_id,
            amount,
            paid,
        }
        .emit();
        paid
    }

    pub fn retry_payout(&mut self, escrow_id: String, account_id: AccountId) -> Promise {
        let mut escrow = self.load_escrow(&escrow_id);
        let index = escrow
            .pending_payouts
            .iter()
            .position(|p| p.account_id == account_id)
            .unwrap_or_else(|| env::panic_str("no pending payout for this account"));
        // Removed before sending and restored by `on_payout` only on failure, so
        // concurrent retries can never pay twice.
        let pending = escrow.pending_payouts.remove(index);
        require_payout_gas(1);
        self.save_escrow(&escrow_id, escrow);
        self.payout(&escrow_id, &account_id, pending.amount.0)
    }
}

impl VecrowEscrow {
    pub(crate) fn payout(&self, escrow_id: &str, account_id: &AccountId, amount: u128) -> Promise {
        ext_ft::ext(self.config.usdc_contract_id.clone())
            .with_attached_deposit(NearToken::from_yoctonear(1))
            .with_static_gas(GAS_FT_TRANSFER)
            .ft_transfer(
                account_id.clone(),
                U128(amount),
                Some(format!("vecrow escrow {escrow_id}")),
            )
            .then(
                Self::ext(env::current_account_id())
                    .with_static_gas(GAS_ON_PAYOUT)
                    .on_payout(escrow_id.to_string(), account_id.clone(), U128(amount)),
            )
    }

    pub(crate) fn release_milestone(
        &mut self,
        escrow_id: &str,
        escrow: &mut Escrow,
        index: u32,
        trigger: ReleaseTrigger,
    ) {
        let milestone = &mut escrow.milestones[index as usize];
        milestone.status = MilestoneStatus::Released;
        let amount = milestone.amount.0;
        escrow.balance = escrow
            .balance
            .checked_sub(amount)
            .unwrap_or_else(|| env::panic_str("escrow balance is too low"));

        let plan = release_plan(amount, escrow.fee_bps, &escrow.split);
        let mut payouts = vec![(self.config.treasury_account.clone(), plan.fee)];
        payouts.extend(
            escrow
                .split
                .iter()
                .map(|s| s.account_id.clone())
                .zip(plan.shares),
        );
        // NEP-141 rejects zero-amount transfers.
        payouts.retain(|(_, amount)| *amount > 0);
        require_payout_gas(payouts.len());

        VecrowEvent::MilestoneReleased {
            escrow_id: escrow_id.to_string(),
            milestone: index,
            amount: U128(amount),
            fee: U128(plan.fee),
            by: trigger.method().to_string(),
        }
        .emit();
        for (account_id, amount) in &payouts {
            self.payout(escrow_id, account_id, *amount).detach();
        }
        complete_if_done(escrow_id, escrow);
    }
}

pub(crate) fn require_payout_gas(count: usize) {
    let per_payout = GAS_FT_TRANSFER.saturating_add(GAS_ON_PAYOUT);
    let needed = Gas::from_gas(per_payout.as_gas() * count as u64).saturating_add(GAS_RESERVE);
    let available = env::prepaid_gas().saturating_sub(env::used_gas());
    require!(
        available >= needed,
        format!("not enough gas for {count} payouts: attach at least {needed} more")
    );
}

pub(crate) fn complete_if_done(escrow_id: &str, escrow: &mut Escrow) {
    let done = escrow.milestones.iter().all(|m| {
        matches!(
            m.status,
            MilestoneStatus::Released | MilestoneStatus::Refunded
        )
    });
    if done && escrow.status == EscrowStatus::Funded {
        escrow.status = EscrowStatus::Completed;
        VecrowEvent::EscrowCompleted {
            escrow_id: escrow_id.to_string(),
        }
        .emit();
    }
}
