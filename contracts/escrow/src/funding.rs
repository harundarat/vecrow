use crate::events::VecrowEvent;
use crate::*;
use near_sdk::PromiseOrValue;
use near_sdk::json_types::U128;

#[near]
impl VecrowEscrow {
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
    fn accept_funding(
        &mut self,
        sender_id: &AccountId,
        amount: u128,
        msg: &str,
    ) -> Result<u128, &'static str> {
        if env::predecessor_account_id() != self.config.usdc_contract_id {
            return Err("unsupported token");
        }
        let mut escrow = self.escrows.get(msg).cloned().ok_or("unknown escrow")?;
        if escrow.status != EscrowStatus::Created {
            return Err("escrow is not awaiting funds");
        }
        let total = escrow.total();
        if amount < total {
            return Err("amount is below the escrow total");
        }

        escrow.status = EscrowStatus::Funded;
        escrow.balance = total;
        escrow.funded_at_ms = Some(env::block_timestamp_ms());
        self.save_escrow(msg, escrow);
        VecrowEvent::EscrowFunded {
            escrow_id: msg.to_string(),
            sender_id: sender_id.clone(),
            amount: U128(total),
        }
        .emit();
        Ok(total)
    }
}
