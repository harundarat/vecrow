use super::*;

fn escrow_with_pending_payout(contract: &mut VecrowEscrow) -> String {
    let wallet = Wallet::near();
    let id = funded_escrow(contract, &wallet);
    submit(contract, &id, 0, T0_MS);
    approve(contract, &id, 0, &wallet, 1, T0_MS);
    assert!(!payout_callback(contract, &id, MEMBER, 298_500_000, false));
    id
}

fn retry(contract: &mut VecrowEscrow, escrow_id: &str, account_id: &str) {
    set_context(STRANGER, T0_MS);
    contract
        .retry_payout(escrow_id.to_string(), account(account_id))
        .detach();
}

#[test]
fn retry_sends_the_exact_pending_amount_and_clears_it_first() {
    let mut contract = new_contract();
    let id = escrow_with_pending_payout(&mut contract);
    retry(&mut contract, &id, MEMBER);

    assert_eq!(ft_transfers(), vec![(MEMBER.to_string(), 298_500_000)]);
    assert!(view(&contract, &id).pending_payouts.is_empty());

    assert!(payout_callback(
        &mut contract,
        &id,
        MEMBER,
        298_500_000,
        true
    ));
    assert!(view(&contract, &id).pending_payouts.is_empty());
}

#[test]
#[should_panic(expected = "no pending payout for this account")]
fn second_retry_in_a_row_cannot_pay_twice() {
    let mut contract = new_contract();
    let id = escrow_with_pending_payout(&mut contract);
    retry(&mut contract, &id, MEMBER);
    retry(&mut contract, &id, MEMBER);
}

#[test]
fn failed_retry_restores_the_pending_amount() {
    let mut contract = new_contract();
    let id = escrow_with_pending_payout(&mut contract);
    retry(&mut contract, &id, MEMBER);
    assert!(!payout_callback(
        &mut contract,
        &id,
        MEMBER,
        298_500_000,
        false
    ));
    assert_eq!(
        view(&contract, &id).pending_payouts,
        vec![PendingPayout {
            account_id: account(MEMBER),
            amount: U128(298_500_000),
        }]
    );

    retry(&mut contract, &id, MEMBER);
    assert_eq!(ft_transfers(), vec![(MEMBER.to_string(), 298_500_000)]);
}

#[test]
fn retry_only_touches_the_given_account() {
    let mut contract = new_contract();
    let id = escrow_with_pending_payout(&mut contract);
    assert!(!payout_callback(
        &mut contract,
        &id,
        TREASURY,
        usdc(5),
        false
    ));
    retry(&mut contract, &id, TREASURY);

    assert_eq!(ft_transfers(), vec![(TREASURY.to_string(), usdc(5))]);
    assert_eq!(
        view(&contract, &id).pending_payouts,
        vec![PendingPayout {
            account_id: account(MEMBER),
            amount: U128(298_500_000),
        }]
    );
}

#[test]
#[should_panic(expected = "no pending payout for this account")]
fn retry_without_pending_payout_is_rejected() {
    let mut contract = new_contract();
    let id = funded_escrow(&mut contract, &Wallet::near());
    retry(&mut contract, &id, MEMBER);
}

#[test]
fn retry_works_after_the_escrow_is_completed() {
    let mut contract = new_contract();
    let id = escrow_with_pending_payout(&mut contract);
    refund(&mut contract, FREELANCER, &id, None);
    assert_eq!(view(&contract, &id).status, EscrowStatus::Completed);
    retry(&mut contract, &id, MEMBER);
    assert_eq!(ft_transfers(), vec![(MEMBER.to_string(), 298_500_000)]);
}
