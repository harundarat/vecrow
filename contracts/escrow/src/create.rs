//! Put escrows on-chain from terms signed by both parties, and cancel them
//! before funding (R-01, R-03, R-04, R-19, R-23; D-14).

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
    /// Puts an escrow on-chain with terms that both parties signed (R-01,
    /// R-23). The backend calls it through the relayer after the creator
    /// approved; the caller gains no rights, because validity comes only from
    /// the two signatures over the same message (`signature::terms_message`):
    ///
    /// - `freelancer_signature`: NEP-413 by the freelancer's NEAR wallet;
    /// - `client_signature`: by the approver wallet named in the terms, in its
    ///   scheme, which also proves that the client controls it (R-04).
    ///
    /// Rejects an escrow ID that exists or is malformed, terms past their
    /// deadline, a `fee_bps` other than the current configuration, the split
    /// and approver rules of R-03 and R-04, and any signature that does not
    /// match. The escrow starts `Created`, with the fee and the review window
    /// locked. Split recipients and the refund account should already be
    /// registered on the USDC contract (R-20); if not, their payouts are kept
    /// as pending.
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

    /// Cancels an escrow that is not funded yet (`Created`), R-01. Either the
    /// freelancer calls it (via the relayer; checked with the predecessor),
    /// or anyone forwards a cancellation signed by the approver: `nonce`,
    /// `deadline_ms` and `signature` over `signature::action_message(cancel)`,
    /// with the same nonce and deadline rules as approvals. Anyone else is
    /// rejected.
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

/// The R-04 rules on who the approver and the refund account may be (D-18):
/// the approver is neither the freelancer nor a split recipient, also under
/// their implicit and eth-implicit account IDs or the freelancer's signing
/// key; the refund account is neither of them nor the contract. A freelancer
/// that is an implicit account must sign with its own key.
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
