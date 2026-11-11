//! Bind the approver and the refund account (R-04, D-01).

use crate::events::VecrowEvent;
use crate::signature::{ApproverKey, bind_message, parse_nonce};
use crate::*;

#[near]
impl VecrowEscrow {
    /// Binds the client's wallet as the sole approver of an escrow, together
    /// with the NEAR `refund_account` for refunds (R-04). Called by the relayer
    /// (or anyone) with a payload signed by that wallet; the caller gains no
    /// rights. Allowed once, while the escrow is `Created`; the status becomes
    /// `ApproverBound`, and both values stay locked for the escrow's lifetime.
    ///
    /// `nonce` is 64 lowercase hex characters (32 bytes); `deadline_ms` is in
    /// milliseconds since the Unix epoch. The signed message is built by
    /// `signature::bind_message`.
    pub fn bind_approver(
        &mut self,
        escrow_id: String,
        approver: Approver,
        refund_account: AccountId,
        nonce: String,
        deadline_ms: u64,
        signature: String,
    ) {
        let (id, mut escrow) = self.load_escrow(&escrow_id);
        match escrow.status {
            EscrowStatus::Created => {}
            EscrowStatus::Cancelled => env::panic_str("escrow is cancelled"),
            _ => env::panic_str("approver is already bound"),
        }

        let key = ApproverKey::parse(&approver).unwrap_or_else(|err| env::panic_str(err));
        let is_party = |account: &str| {
            account == escrow.freelancer.as_str()
                || escrow
                    .split
                    .iter()
                    .any(|s| s.account_id.as_str() == account)
        };
        require!(
            !key.near_account_ids().iter().any(|a| is_party(a)),
            "approver must not be the freelancer or a split recipient"
        );
        require!(
            !is_party(refund_account.as_str()),
            "refund_account must not be the freelancer or a split recipient"
        );
        require!(
            refund_account != env::current_account_id(),
            "refund_account must not be the escrow contract"
        );

        let nonce = parse_nonce(&nonce).unwrap_or_else(|err| env::panic_str(err));
        let message = bind_message(
            &env::current_account_id(),
            id,
            &key,
            &refund_account,
            &nonce,
            deadline_ms,
        );
        self.accept_signed_action(&key, &message, &nonce, deadline_ms, &signature);

        escrow.approver = Some(key.clone());
        escrow.refund_account = Some(refund_account.clone());
        escrow.status = EscrowStatus::ApproverBound;
        self.save_escrow(id, escrow);
        VecrowEvent::ApproverBound {
            escrow_id,
            approver: key.to_approver(),
            refund_account,
        }
        .emit();
    }
}
