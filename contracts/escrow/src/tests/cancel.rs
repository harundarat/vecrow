//! R-01: cancel before funding, by the freelancer or with the approver's
//! signature.

use super::*;

#[test]
fn approver_cancels_with_each_wallet_scheme() {
    for wallet in Wallet::all() {
        let mut contract = new_contract();
        let id = create_escrow_with(&mut contract, &wallet);
        cancel_signed(&mut contract, RELAYER, &id, &wallet, 1, T0_MS);
        assert_eq!(view(&contract, &id).status, EscrowStatus::Cancelled);
        assert_eq!(
            near_sdk::test_utils::get_logs().last().unwrap(),
            r#"EVENT_JSON:{"standard":"vecrow","version":"1.0.0","event":"escrow_cancelled","data":{"escrow_id":"e1","by":"approver"}}"#
        );
    }
}

#[test]
fn freelancer_cancel_needs_no_signature() {
    let mut contract = new_contract();
    let id = create_default(&mut contract);
    set_context(FREELANCER, T0_MS);
    contract.cancel(id.clone(), None, None, None);
    assert_eq!(
        near_sdk::test_utils::get_logs().last().unwrap(),
        r#"EVENT_JSON:{"standard":"vecrow","version":"1.0.0","event":"escrow_cancelled","data":{"escrow_id":"e1","by":"freelancer"}}"#
    );
}

#[test]
#[should_panic(expected = "invalid signature")]
fn cancellation_signed_by_another_wallet_is_rejected() {
    let mut contract = new_contract();
    let id = create_escrow_with(&mut contract, &Wallet::evm());
    cancel_signed(
        &mut contract,
        RELAYER,
        &id,
        &Wallet::Evm(EvmWallet::new(77)),
        1,
        T0_MS,
    );
}

#[test]
#[should_panic(expected = "invalid signature")]
fn cancellation_signed_by_the_freelancer_key_is_rejected() {
    // The freelancer cancels as the predecessor; a signature does not make
    // anyone else the freelancer.
    let mut contract = new_contract();
    let id = create_escrow_with(&mut contract, &Wallet::near());
    let freelancer_key = Wallet::Near(freelancer_wallet());
    cancel_signed(&mut contract, RELAYER, &id, &freelancer_key, 1, T0_MS);
}

#[test]
#[should_panic(expected = "invalid signature")]
fn approval_signature_cannot_cancel() {
    let mut contract = new_contract();
    let wallet = Wallet::solana();
    let id = create_escrow_with(&mut contract, &wallet);
    let (nonce, signature) = sign_milestone(&wallet, "approve_milestone", &id, 0, 1, DEADLINE_MS);
    set_context(RELAYER, T0_MS);
    contract.cancel(id, Some(nonce), Some(DEADLINE_MS), Some(signature));
}

#[test]
#[should_panic(expected = "signature expired")]
fn expired_cancellation_is_rejected() {
    let mut contract = new_contract();
    let wallet = Wallet::near();
    let id = create_escrow_with(&mut contract, &wallet);
    cancel_signed(&mut contract, RELAYER, &id, &wallet, 1, DEADLINE_MS + 1);
}

#[test]
#[should_panic(expected = "nonce already used")]
fn cancellation_nonce_is_used_once_across_escrows() {
    let mut contract = new_contract();
    let wallet = Wallet::evm();
    let first = create_escrow_with(&mut contract, &wallet);
    let second = create_escrow_with(&mut contract, &wallet);
    cancel_signed(&mut contract, RELAYER, &first, &wallet, 9, T0_MS);
    cancel_signed(&mut contract, RELAYER, &second, &wallet, 9, T0_MS);
}

#[test]
#[should_panic(expected = "only the freelancer or the approver can cancel")]
fn incomplete_signed_cancellation_is_rejected() {
    let mut contract = new_contract();
    let wallet = Wallet::near();
    let id = create_escrow_with(&mut contract, &wallet);
    let (nonce, _) = sign_cancel(&wallet, &id, 1, DEADLINE_MS);
    set_context(RELAYER, T0_MS);
    contract.cancel(id, Some(nonce), Some(DEADLINE_MS), None);
}

#[test]
#[should_panic(expected = "only an unfunded escrow can be cancelled")]
fn approver_cannot_cancel_a_funded_escrow() {
    let mut contract = new_contract();
    let wallet = Wallet::solana();
    let id = funded_escrow(&mut contract, &wallet);
    cancel_signed(&mut contract, RELAYER, &id, &wallet, 1, T0_MS);
}

#[test]
fn freelancer_can_still_cancel_with_unrelated_arguments() {
    // A freelancer call does not need a signature; any given is ignored.
    let mut contract = new_contract();
    let id = create_default(&mut contract);
    set_context(FREELANCER, T0_MS);
    contract.cancel(id.clone(), Some("zz".into()), None, Some("x".into()));
    assert_eq!(view(&contract, &id).status, EscrowStatus::Cancelled);
}
