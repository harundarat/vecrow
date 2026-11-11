//! R-02: funding and amount rules in `ft_on_transfer`, and the R-01 rule that
//! a funded escrow cannot be cancelled.

use super::*;

#[test]
fn exact_total_funds_a_created_escrow() {
    let mut contract = new_contract();
    let id = create_default(&mut contract);
    assert_eq!(fund(&mut contract, &id, usdc(1_500)), 0);

    let escrow = view(&contract, &id);
    assert_eq!(escrow.status, EscrowStatus::Funded);
    assert_eq!(escrow.balance, U128(usdc(1_500)));
    assert_eq!(escrow.funded_at_ms, Some(T0_MS));
    assert_eq!(
        near_sdk::test_utils::get_logs().last().unwrap(),
        r#"EVENT_JSON:{"standard":"vecrow","version":"1.0.0","event":"escrow_funded","data":{"escrow_id":"e1","sender_id":"client.test.near","amount":"1500000000"}}"#
    );
}

#[test]
fn amount_below_total_is_returned_in_full() {
    let mut contract = new_contract();
    let id = create_default(&mut contract);
    assert_eq!(fund(&mut contract, &id, usdc(1_500) - 1), usdc(1_500) - 1);

    let escrow = view(&contract, &id);
    assert_eq!(escrow.status, EscrowStatus::Created);
    assert_eq!(escrow.balance, U128(0));
    assert_eq!(escrow.funded_at_ms, None);
}

#[test]
fn excess_above_total_is_returned() {
    let mut contract = new_contract();
    let id = create_default(&mut contract);
    assert_eq!(fund(&mut contract, &id, usdc(2_000) + 7), usdc(500) + 7);

    let escrow = view(&contract, &id);
    assert_eq!(escrow.status, EscrowStatus::Funded);
    assert_eq!(escrow.balance, U128(usdc(1_500)));
}

#[test]
fn funded_escrow_returns_second_transfer() {
    let mut contract = new_contract();
    let id = create_default(&mut contract);
    fund(&mut contract, &id, usdc(1_500));
    assert_eq!(fund(&mut contract, &id, usdc(1_500)), usdc(1_500));
    assert_eq!(view(&contract, &id).balance, U128(usdc(1_500)));
}

#[test]
fn completed_escrow_returns_transfer() {
    let mut contract = new_contract();
    let id = funded_escrow(&mut contract, &Wallet::near());
    refund(&mut contract, FREELANCER, &id, None);
    assert_eq!(view(&contract, &id).status, EscrowStatus::Completed);
    assert_eq!(fund(&mut contract, &id, usdc(1_500)), usdc(1_500));
}

#[test]
fn cancelled_escrow_returns_transfer() {
    let mut contract = new_contract();
    let id = create_default(&mut contract);
    set_context(FREELANCER, T0_MS);
    contract.cancel(id.clone());
    assert_eq!(fund(&mut contract, &id, usdc(1_500)), usdc(1_500));
    assert_eq!(view(&contract, &id).status, EscrowStatus::Cancelled);
}

#[test]
fn other_token_is_returned() {
    let mut contract = new_contract();
    let id = create_default(&mut contract);
    let returned = transfer_call_from(
        &mut contract,
        "fake-usdc.test.near",
        CLIENT,
        usdc(1_500),
        &id,
    );
    assert_eq!(returned, usdc(1_500));
    assert_eq!(view(&contract, &id).status, EscrowStatus::Created);
    assert_eq!(
        near_sdk::test_utils::get_logs().last().unwrap(),
        "returning 1500000000 to client.test.near: unsupported token"
    );
}

#[test]
fn unknown_escrow_id_is_returned() {
    // Includes an escrow the backend proposed but has not put on-chain yet.
    let mut contract = new_contract();
    create_default(&mut contract);
    for msg in ["e2", "E1", "", "e1 ", "{\"escrow_id\":\"e1\"}"] {
        assert_eq!(
            fund(&mut contract, msg, usdc(1_500)),
            usdc(1_500),
            "{msg:?}"
        );
    }
    assert_eq!(view(&contract, "e1").status, EscrowStatus::Created);
}

#[test]
#[should_panic(expected = "only an unfunded escrow can be cancelled")]
fn funded_escrow_cannot_be_cancelled() {
    let mut contract = new_contract();
    let id = funded_escrow(&mut contract, &Wallet::near());
    set_context(FREELANCER, T0_MS);
    contract.cancel(id);
}
