use crate::events::VecrowEvent;
use crate::signature::{
    Action, ApproverKey, EscrowTerms, FreelancerSignature, action_message, parse_near_public_key,
    parse_nonce, terms_message, terms_nonce, verify_nep413,
};
use crate::split::validate_split;
use crate::*;
use near_sdk::json_types::U128;

#[near]
impl VecrowEscrow {
    pub fn create_escrow(
        &mut self,
        terms: EscrowTerms,
        freelancer_signature: FreelancerSignature,
        client_signature: String,
    ) {
        require!(is_valid_escrow_id(&terms.escrow_id), "invalid escrow ID");
        require!(
            !self.escrows.contains_key(&terms.escrow_id),
            "escrow ID already exists"
        );
        require!(
            env::block_timestamp_ms() <= terms.deadline_ms,
            "terms expired"
        );
        require!(
            terms.fee_bps == self.config.fee_bps,
            "fee_bps does not match the current configuration"
        );
        require!(
            !terms.milestones.is_empty(),
            "at least one milestone is required"
        );
        require!(
            terms.milestones.len() <= MAX_MILESTONES,
            "too many milestones"
        );
        let mut total: u128 = 0;
        for amount in &terms.milestones {
            require!(amount.0 > 0, "milestone amounts must be positive");
            total = total
                .checked_add(amount.0)
                .unwrap_or_else(|| env::panic_str("milestone total overflows"));
        }
        validate_split(&terms.split);
        let contract = env::current_account_id();
        require!(
            terms.freelancer != contract,
            "the escrow contract cannot be the freelancer"
        );

        let freelancer_key = parse_near_public_key(&freelancer_signature.public_key)
            .unwrap_or_else(|err| env::panic_str(err));
        let approver =
            ApproverKey::parse(&terms.approver).unwrap_or_else(|err| env::panic_str(err));
        check_parties(&terms, &approver, &freelancer_key);

        let message = terms_message(&contract, &terms, &approver);
        let nonce = terms_nonce(&message);
        let freelancer_valid = verify_nep413(
            &freelancer_key,
            &message,
            &nonce,
            contract.as_str(),
            &freelancer_signature.signature,
        )
        .unwrap_or_else(|err| env::panic_str(err));
        require!(freelancer_valid, "invalid freelancer signature");
        let client_valid = approver
            .verify(&message, &nonce, contract.as_str(), &client_signature)
            .unwrap_or_else(|err| env::panic_str(err));
        require!(client_valid, "invalid client signature");

        let escrow = Escrow {
            freelancer: terms.freelancer.clone(),
            milestones: terms
                .milestones
                .into_iter()
                .map(|amount| Milestone {
                    amount,
                    status: MilestoneStatus::Pending,
                    submitted_at_ms: None,
                    revision_count: 0,
                })
                .collect(),
            split: terms.split,
            fee_bps: terms.fee_bps,
            review_window_sec: self.config.review_window_sec,
            approver: approver.clone(),
            refund_account: terms.refund_account.clone(),
            status: EscrowStatus::Created,
            balance: 0,
            pending_payouts: Vec::new(),
            created_at_ms: env::block_timestamp_ms(),
            funded_at_ms: None,
        };
        self.save_escrow(&terms.escrow_id, escrow);
        VecrowEvent::EscrowCreated {
            escrow_id: terms.escrow_id,
            freelancer: terms.freelancer,
            approver: approver.to_approver(),
            refund_account: terms.refund_account,
            total: U128(total),
        }
        .emit();
    }

    pub fn cancel(
        &mut self,
        escrow_id: String,
        nonce: Option<String>,
        deadline_ms: Option<u64>,
        signature: Option<String>,
    ) {
        let mut escrow = self.load_escrow(&escrow_id);
        require!(
            escrow.status == EscrowStatus::Created,
            "only an unfunded escrow can be cancelled"
        );
        let by = if env::predecessor_account_id() == escrow.freelancer {
            "freelancer"
        } else {
            let (Some(nonce), Some(deadline_ms), Some(signature)) = (nonce, deadline_ms, signature)
            else {
                env::panic_str("only the freelancer or the approver can cancel")
            };
            let nonce = parse_nonce(&nonce).unwrap_or_else(|err| env::panic_str(err));
            let message = action_message(
                Action::Cancel,
                &env::current_account_id(),
                &escrow_id,
                None,
                &nonce,
                deadline_ms,
            );
            self.accept_signed_action(&escrow.approver, &message, &nonce, deadline_ms, &signature);
            "approver"
        };
        escrow.status = EscrowStatus::Cancelled;
        self.save_escrow(&escrow_id, escrow);
        VecrowEvent::EscrowCancelled {
            escrow_id,
            by: by.to_string(),
        }
        .emit();
    }
}

fn check_parties(terms: &EscrowTerms, approver: &ApproverKey, freelancer_key: &[u8; 32]) {
    let is_party = |account: &str| {
        account == terms.freelancer.as_str()
            || terms.split.iter().any(|s| s.account_id.as_str() == account)
    };
    require!(
        !approver.near_account_ids().iter().any(|a| is_party(a))
            && approver.ed25519_key() != Some(freelancer_key),
        "approver must not be the freelancer or a split recipient"
    );
    require!(
        !is_party(terms.refund_account.as_str()),
        "refund_account must not be the freelancer or a split recipient"
    );
    require!(
        terms.refund_account != env::current_account_id(),
        "refund_account must not be the escrow contract"
    );
    let freelancer = terms.freelancer.as_str();
    let implicit = freelancer.len() == 64
        && freelancer
            .bytes()
            .all(|b| matches!(b, b'0'..=b'9' | b'a'..=b'f'));
    require!(
        !implicit || freelancer == hex::encode(freelancer_key),
        "an implicit freelancer account must sign with its own key"
    );
}
