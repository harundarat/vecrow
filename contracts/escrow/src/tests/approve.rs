//! R-06 (signed approval) and R-19 (fee on release).

use super::*;

fn submitted(contract: &mut VecrowEscrow, wallet: &Wallet) -> String {
    let id = funded_escrow(contract, wallet);
    submit(contract, &id, 0, T0_MS);
    id
}

#[test]
fn each_wallet_scheme_approves_and_pays_fee_and_split() {
    for wallet in Wallet::all() {
        let mut contract = new_contract();
        let id = submitted(&mut contract, &wallet);
        approve(&mut contract, &id, 0, &wallet, 1, T0_MS + 10);

        // R-19: 1,000 USDC with 70/30 pays 5 to the treasury, then 696.5 and
        // 298.5, each through ft_transfer + on_payout.
        assert_eq!(
            ft_transfers(),
            vec![
                (TREASURY.to_string(), usdc(5)),
                (FREELANCER.to_string(), 696_500_000),
                (MEMBER.to_string(), 298_500_000),
            ]
        );
        let escrow = view(&contract, &id);
        assert_eq!(escrow.milestones[0].status, MilestoneStatus::Released);
        assert_eq!(escrow.milestones[1].status, MilestoneStatus::Pending);
        assert_eq!(escrow.balance, U128(usdc(500)));
        assert_eq!(escrow.status, EscrowStatus::Funded);
    }
}

#[test]
fn release_emits_event_with_amount_and_fee() {
    let mut contract = new_contract();
    let id = submitted(&mut contract, &Wallet::near());
    approve(&mut contract, &id, 0, &Wallet::near(), 1, T0_MS);
    assert!(near_sdk::test_utils::get_logs().contains(
        &r#"EVENT_JSON:{"standard":"vecrow","version":"1.0.0","event":"milestone_released","data":{"escrow_id":"e1","milestone":0,"amount":"1000000000","fee":"5000000","by":"approve_milestone"}}"#
            .to_string()
    ));
}

#[test]
fn approving_every_milestone_completes_the_escrow() {
    let mut contract = new_contract();
    let wallet = Wallet::solana();
    let id = submitted(&mut contract, &wallet);
    submit(&mut contract, &id, 1, T0_MS);
    approve(&mut contract, &id, 1, &wallet, 1, T0_MS);
    assert_eq!(view(&contract, &id).status, EscrowStatus::Funded);
    approve(&mut contract, &id, 0, &wallet, 2, T0_MS);

    let escrow = view(&contract, &id);
    assert_eq!(escrow.status, EscrowStatus::Completed);
    assert_eq!(escrow.balance, U128(0));
    assert_eq!(
        near_sdk::test_utils::get_logs().last().unwrap(),
        r#"EVENT_JSON:{"standard":"vecrow","version":"1.0.0","event":"escrow_completed","data":{"escrow_id":"e1"}}"#
    );
}

#[test]
fn anyone_may_forward_a_valid_approval() {
    let mut contract = new_contract();
    let wallet = Wallet::evm();
    let id = submitted(&mut contract, &wallet);
    let (nonce, signature) = sign_milestone(&wallet, "approve_milestone", &id, 0, 1, DEADLINE_MS);
    set_context(STRANGER, T0_MS);
    contract.approve_milestone(id.clone(), 0, nonce, DEADLINE_MS, signature);
    assert_eq!(
        view(&contract, &id).milestones[0].status,
        MilestoneStatus::Released
    );
}

#[test]
#[should_panic(expected = "nonce already used")]
fn reused_approval_nonce_is_rejected() {
    let mut contract = new_contract();
    let wallet = Wallet::near();
    let id = submitted(&mut contract, &wallet);
    submit(&mut contract, &id, 1, T0_MS);
    approve(&mut contract, &id, 0, &wallet, 7, T0_MS);
    approve(&mut contract, &id, 1, &wallet, 7, T0_MS);
}

#[test]
#[should_panic(expected = "nonce already used")]
fn approval_nonce_is_used_once_across_escrows() {
    let mut contract = new_contract();
    let wallet = Wallet::evm();
    let first = submitted(&mut contract, &wallet);
    let second = submitted(&mut contract, &wallet);
    approve(&mut contract, &first, 0, &wallet, 7, T0_MS);
    approve(&mut contract, &second, 0, &wallet, 7, T0_MS);
}

#[test]
fn nonces_are_tracked_per_approver() {
    let mut contract = new_contract();
    let first = submitted(&mut contract, &Wallet::evm());
    let second = submitted(&mut contract, &Wallet::solana());
    approve(&mut contract, &first, 0, &Wallet::evm(), 7, T0_MS);
    approve(&mut contract, &second, 0, &Wallet::solana(), 7, T0_MS);
    assert_eq!(
        view(&contract, &second).milestones[0].status,
        MilestoneStatus::Released
    );
}

#[test]
#[should_panic(expected = "signature expired")]
fn expired_approval_is_rejected() {
    let mut contract = new_contract();
    let wallet = Wallet::solana();
    let id = submitted(&mut contract, &wallet);
    approve(&mut contract, &id, 0, &wallet, 1, DEADLINE_MS + 1);
}

#[test]
#[should_panic(expected = "invalid signature")]
fn approval_by_another_wallet_is_rejected() {
    let mut contract = new_contract();
    let id = submitted(&mut contract, &Wallet::evm());
    approve(
        &mut contract,
        &id,
        0,
        &Wallet::Evm(EvmWallet::new(77)),
        1,
        T0_MS,
    );
}

#[test]
#[should_panic(expected = "invalid signature")]
fn approval_by_the_freelancer_key_is_rejected() {
    let mut contract = new_contract();
    let id = submitted(&mut contract, &Wallet::near());
    let freelancer_key = Wallet::Near(NearWallet::new(FREELANCER, 11 + 1));
    approve(&mut contract, &id, 0, &freelancer_key, 1, T0_MS);
}

#[test]
#[should_panic(expected = "invalid signature")]
fn approval_signed_for_another_milestone_is_rejected() {
    let mut contract = new_contract();
    let wallet = Wallet::near();
    let id = submitted(&mut contract, &wallet);
    let (nonce, signature) = sign_milestone(&wallet, "approve_milestone", &id, 1, 1, DEADLINE_MS);
    set_context(RELAYER, T0_MS);
    contract.approve_milestone(id, 0, nonce, DEADLINE_MS, signature);
}

#[test]
#[should_panic(expected = "invalid signature")]
fn revision_signature_cannot_approve() {
    let mut contract = new_contract();
    let wallet = Wallet::evm();
    let id = submitted(&mut contract, &wallet);
    let (nonce, signature) = sign_milestone(&wallet, "request_revision", &id, 0, 1, DEADLINE_MS);
    set_context(RELAYER, T0_MS);
    contract.approve_milestone(id, 0, nonce, DEADLINE_MS, signature);
}

#[test]
#[should_panic(expected = "invalid signature")]
fn approval_signed_for_another_escrow_is_rejected() {
    let mut contract = new_contract();
    let wallet = Wallet::solana();
    let id = submitted(&mut contract, &wallet);
    let (nonce, signature) = sign_milestone(&wallet, "approve_milestone", "e2", 0, 1, DEADLINE_MS);
    set_context(RELAYER, T0_MS);
    contract.approve_milestone(id, 0, nonce, DEADLINE_MS, signature);
}

#[test]
#[should_panic(expected = "invalid signature")]
fn approval_with_a_changed_deadline_is_rejected() {
    let mut contract = new_contract();
    let wallet = Wallet::near();
    let id = submitted(&mut contract, &wallet);
    let (nonce, signature) = sign_milestone(&wallet, "approve_milestone", &id, 0, 1, DEADLINE_MS);
    set_context(RELAYER, T0_MS);
    contract.approve_milestone(id, 0, nonce, DEADLINE_MS + 1, signature);
}

#[test]
#[should_panic(expected = "only a Submitted milestone can be approved or sent back for revision")]
fn pending_milestone_cannot_be_approved() {
    let mut contract = new_contract();
    let wallet = Wallet::near();
    let id = funded_escrow(&mut contract, &wallet);
    approve(&mut contract, &id, 0, &wallet, 1, T0_MS);
}

#[test]
#[should_panic(expected = "only a Submitted milestone can be approved or sent back for revision")]
fn released_milestone_cannot_be_approved_again() {
    let mut contract = new_contract();
    let wallet = Wallet::evm();
    let id = submitted(&mut contract, &wallet);
    approve(&mut contract, &id, 0, &wallet, 1, T0_MS);
    approve(&mut contract, &id, 0, &wallet, 2, T0_MS);
}

#[test]
#[should_panic(expected = "escrow is not funded")]
fn unfunded_escrow_cannot_be_approved() {
    let mut contract = new_contract();
    let wallet = Wallet::evm();
    let id = create_escrow_with(&mut contract, &wallet);
    approve(&mut contract, &id, 0, &wallet, 2, T0_MS);
}

#[test]
fn changing_the_fee_config_does_not_affect_existing_escrows() {
    // R-19: the fee is locked at creation. A redeploy with a new fee (D-15)
    // is simulated by changing the stored configuration.
    let mut contract = new_contract();
    let wallet = Wallet::near();
    let id = submitted(&mut contract, &wallet);
    contract.config.fee_bps = 1_000;
    approve(&mut contract, &id, 0, &wallet, 1, T0_MS);
    assert_eq!(ft_transfers()[0], (TREASURY.to_string(), usdc(5)));
}

#[test]
fn zero_amounts_are_not_transferred() {
    let mut contract = new_contract();
    let wallet = Wallet::near();
    let mut terms = default_terms("e1", &wallet);
    terms.milestones = vec![U128(1)];
    create_with(
        &mut contract,
        sign_terms(terms, &freelancer_wallet(), &wallet),
    );
    let id = "e1".to_string();
    fund(&mut contract, &id, 1);
    submit(&mut contract, &id, 0, T0_MS);
    approve(&mut contract, &id, 0, &wallet, 2, T0_MS);
    // Fee floor(1 × 0.5%) = 0 and the 30% share floors to 0; only the
    // freelancer is paid.
    assert_eq!(ft_transfers(), vec![(FREELANCER.to_string(), 1)]);
    assert_eq!(view(&contract, &id).status, EscrowStatus::Completed);
}

#[test]
#[should_panic(expected = "not enough gas for 3 payouts")]
fn approval_without_enough_gas_for_payouts_fails() {
    let mut contract = new_contract();
    let wallet = Wallet::near();
    let id = submitted(&mut contract, &wallet);
    let (nonce, signature) = sign_milestone(&wallet, "approve_milestone", &id, 0, 1, DEADLINE_MS);
    testing_env!(
        VMContextBuilder::new()
            .current_account_id(account(CONTRACT))
            .predecessor_account_id(account(RELAYER))
            .block_timestamp(T0_MS * 1_000_000)
            .prepaid_gas(near_sdk::Gas::from_tgas(50))
            .build()
    );
    contract.approve_milestone(id, 0, nonce, DEADLINE_MS, signature);
}
