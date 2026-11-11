//! Funding through NEP-141 `ft_transfer_call` (R-02, D-05).

use crate::events::VecrowEvent;
use crate::*;
use near_sdk::PromiseOrValue;
use near_sdk::json_types::U128;

#[near]
impl VecrowEscrow {
    /// NEP-141 receiver, called by the token contract during
    /// `ft_transfer_call(receiver_id = this contract, msg = escrow_id)`.
    /// Returns the amount the token contract must give back to `sender_id`:
    ///
    /// - USDC of at least the total, to an `ApproverBound` escrow: the escrow
    ///   becomes `Funded` with exactly the total, and the excess is returned.
    /// - Anything else is returned in full: less than the total, an escrow in
    ///   any other status, an unknown or malformed `escrow_id`, or a token
    ///   other than the configured USDC.
    ///
    /// Returned deposits are not charged a fee.
    pub fn ft_on_transfer(
        &mut self,
        sender_id: AccountId,
        amount: U128,
        msg: String,
    ) -> PromiseOrValue<U128> {
        match self.accept_funding(&sender_id, amount.0, &msg) {
            Ok(used) => PromiseOrValue::Value(U128(amount.0 - used)),
            Err(reason) => {
                env::log_str(&format!("returning {} to {sender_id}: {reason}", amount.0));
                PromiseOrValue::Value(amount)
            }
        }
    }
}

impl VecrowEscrow {
    /// Applies the funding rules and returns the amount kept.
    fn accept_funding(
        &mut self,
        sender_id: &AccountId,
        amount: u128,
        msg: &str,
    ) -> Result<u128, &'static str> {
        if env::predecessor_account_id() != self.config.usdc_contract_id {
            return Err("unsupported token");
        }
        let id = parse_escrow_id(msg).ok_or("unknown escrow")?;
        let mut escrow = self.escrows.get(&id).cloned().ok_or("unknown escrow")?;
        if escrow.status != EscrowStatus::ApproverBound {
            return Err("escrow is not awaiting funds");
        }
        let total = escrow.total();
        if amount < total {
            return Err("amount is below the escrow total");
        }

        escrow.status = EscrowStatus::Funded;
        escrow.balance = total;
        escrow.funded_at_ms = Some(env::block_timestamp_ms());
        self.save_escrow(id, escrow);
        VecrowEvent::EscrowFunded {
            escrow_id: id.to_string(),
            sender_id: sender_id.clone(),
            amount: U128(total),
        }
        .emit();
        Ok(total)
    }
}
