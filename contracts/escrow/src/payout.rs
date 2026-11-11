//! Payouts: every transfer out of the contract is an `ft_transfer` of USDC
//! followed by the `on_payout` callback (architecture §6.3, R-20). A failed
//! transfer is recorded in the escrow's pending payouts instead of being lost.

use crate::events::VecrowEvent;
use crate::split::release_plan;
use crate::*;
use near_sdk::json_types::U128;
use near_sdk::{Gas, NearToken, Promise, ext_contract, is_promise_success};

/// Static gas for `ft_transfer` on the USDC contract.
pub const GAS_FT_TRANSFER: Gas = Gas::from_tgas(10);
/// Static gas for the `on_payout` callback. Unused gas of the call is also
/// shared out to each transfer and callback by weight.
pub const GAS_ON_PAYOUT: Gas = Gas::from_tgas(10);
/// Gas kept for the calling method itself after scheduling its payouts.
const GAS_RESERVE: Gas = Gas::from_tgas(10);

#[ext_contract(ext_ft)]
#[allow(dead_code)] // Only the generated `ext_ft::ext` client is used.
trait FungibleTokenCore {
    fn ft_transfer(&mut self, receiver_id: AccountId, amount: U128, memo: Option<String>);
}

/// Why a milestone was released; written in the `milestone_released` event.
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
    /// Callback of every payout. Records the amount as pending for
    /// `account_id` if the transfer failed (e.g. the account is not
    /// registered on the USDC contract). Returns whether it was paid.
    #[private]
    pub fn on_payout(&mut self, escrow_id: String, account_id: AccountId, amount: U128) -> bool {
        let paid = is_promise_success();
        if !paid {
            let (id, mut escrow) = self.load_escrow(&escrow_id);
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
            self.save_escrow(id, escrow);
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

    /// Re-sends the pending payout of `account_id` in an escrow (R-20), e.g.
    /// after the backend registered the account on the USDC contract. Anyone
    /// may call it; a backend job does so automatically. The pending amount
    /// is removed before the transfer is sent and restored by `on_payout` only
    /// if it fails again, so two calls in a row can never pay twice. Returns
    /// whether the transfer succeeded.
    pub fn retry_payout(&mut self, escrow_id: String, account_id: AccountId) -> Promise {
        let (id, mut escrow) = self.load_escrow(&escrow_id);
        let index = escrow
            .pending_payouts
            .iter()
            .position(|p| p.account_id == account_id)
            .unwrap_or_else(|| env::panic_str("no pending payout for this account"));
        let pending = escrow.pending_payouts.remove(index);
        require_payout_gas(1);
        self.save_escrow(id, escrow);
        self.payout(id, &account_id, pending.amount.0)
    }
}

impl VecrowEscrow {
    /// Sends `amount` USDC to `account_id`. The 1 yoctoNEAR that
    /// `ft_transfer` requires comes from the contract's balance.
    pub(crate) fn payout(&self, escrow_id: u64, account_id: &AccountId, amount: u128) -> Promise {
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

    /// Releases a `Submitted` milestone: marks it `Released`, takes its amount
    /// out of the escrow balance, and pays the fee to the treasury and the
    /// rest to the split recipients (R-03, R-19). Zero amounts are skipped,
    /// since NEP-141 rejects them.
    pub(crate) fn release_milestone(
        &mut self,
        id: u64,
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
        payouts.retain(|(_, amount)| *amount > 0);
        require_payout_gas(payouts.len());

        VecrowEvent::MilestoneReleased {
            escrow_id: id.to_string(),
            milestone: index,
            amount: U128(amount),
            fee: U128(plan.fee),
            by: trigger.method().to_string(),
        }
        .emit();
        for (account_id, amount) in &payouts {
            self.payout(id, account_id, *amount).detach();
        }
        complete_if_done(id, escrow);
    }
}

/// Fails early, with a clear message, when the attached gas cannot cover
/// `count` payouts and their callbacks.
pub(crate) fn require_payout_gas(count: usize) {
    let per_payout = GAS_FT_TRANSFER.saturating_add(GAS_ON_PAYOUT);
    let needed = Gas::from_gas(per_payout.as_gas() * count as u64).saturating_add(GAS_RESERVE);
    let available = env::prepaid_gas().saturating_sub(env::used_gas());
    require!(
        available >= needed,
        format!("not enough gas for {count} payouts: attach at least {needed} more")
    );
}

/// Marks the escrow `Completed` once every milestone is `Released` or
/// `Refunded`.
pub(crate) fn complete_if_done(id: u64, escrow: &mut Escrow) {
    let done = escrow.milestones.iter().all(|m| {
        matches!(
            m.status,
            MilestoneStatus::Released | MilestoneStatus::Refunded
        )
    });
    if done && escrow.status == EscrowStatus::Funded {
        escrow.status = EscrowStatus::Completed;
        VecrowEvent::EscrowCompleted {
            escrow_id: id.to_string(),
        }
        .emit();
    }
}
