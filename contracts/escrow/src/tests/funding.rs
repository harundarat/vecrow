//! R-02: funding and amount rules in `ft_on_transfer`, and the R-01 rule that
//! a funded escrow cannot be cancelled.

use super::*;

fn bound_escrow(contract: &mut VecrowEscrow) -> String {
    let id = create_default(contract);
    bind(contract, &id, &Wallet::evm(), 1);
    id
}

#[test]
fn exact_total_funds_the_escrow() {
    let mut contract = new_contract();
    let id = bound_escrow(&mut contract);
    assert_eq!(fund(&mut contract, &id, usdc(1_500)), 0);

    let escrow = view(&contract, &id);
    assert_eq!(escrow.status, EscrowStatus::Funded);
    assert_eq!(escrow.balance, U128(usdc(1_500)));
    assert_eq!(escrow.funded_at_ms, Some(T0_MS));
    assert_eq!(
        near_sdk::test_utils::get_logs().last().unwrap(),
        r#"EVENT_JSON:{"standard":"vecrow","version":"1.0.0","event":"escrow_funded","data":{"escrow_id":"1","sender_id":"client.test.near","amount":"1500000000"}}"#
    );
}

#[test]
fn amount_below_total_is_returned_in_full() {
    let mut contract = new_contract();
    let id = bound_escrow(&mut contract);
    assert_eq!(fund(&mut contract, &id, usdc(1_500) - 1), usdc(1_500) - 1);

    let escrow = view(&contract, &id);
    assert_eq!(escrow.status, EscrowStatus::ApproverBound);
    assert_eq!(escrow.balance, U128(0));
    assert_eq!(escrow.funded_at_ms, None);
}

#[test]
fn excess_above_total_is_returned() {
    let mut contract = new_contract();
    let id = bound_escrow(&mut contract);
    assert_eq!(fund(&mut contract, &id, usdc(2_000) + 7), usdc(500) + 7);

    let escrow = view(&contract, &id);
    assert_eq!(escrow.status, EscrowStatus::Funded);
    assert_eq!(escrow.balance, U128(usdc(1_500)));
}

#[test]
fn escrow_without_approver_returns_transfer() {
    let mut contract = new_contract();
    let id = create_default(&mut contract);
    assert_eq!(fund(&mut contract, &id, usdc(1_500)), usdc(1_500));
    assert_eq!(view(&contract, &id).status, EscrowStatus::Created);
}

#[test]
fn funded_escrow_returns_second_transfer() {
    let mut contract = new_contract();
    let id = bound_escrow(&mut contract);
    fund(&mut contract, &id, usdc(1_500));
    assert_eq!(fund(&mut contract, &id, usdc(1_500)), usdc(1_500));
    assert_eq!(view(&contract, &id).balance, U128(usdc(1_500)));
}

#[test]
fn completed_escrow_returns_transfer() {
    let mut contract = new_contract();
    let id = bound_escrow(&mut contract);
    fund(&mut contract, &id, usdc(1_500));
    // Reaching `Completed` needs payouts (covered in the release, refund and
    // sandbox tests); here only the funding rule is under test.
    contract.escrows.get_mut(&1).unwrap().status = EscrowStatus::Completed;
    assert_eq!(fund(&mut contract, &id, usdc(1_500)), usdc(1_500));
}

#[test]
fn cancelled_escrow_returns_transfer() {
    let mut contract = new_contract();
    let id = bound_escrow(&mut contract);
    set_context(FREELANCER, T0_MS);
    contract.cancel(id.clone());
    assert_eq!(fund(&mut contract, &id, usdc(1_500)), usdc(1_500));
    assert_eq!(view(&contract, &id).status, EscrowStatus::Cancelled);
}

#[test]
fn other_token_is_returned() {
    let mut contract = new_contract();
    let id = bound_escrow(&mut contract);
    let returned = transfer_call_from(
        &mut contract,
        "fake-usdc.test.near",
        CLIENT,
        usdc(1_500),
        &id,
    );
    assert_eq!(returned, usdc(1_500));
    assert_eq!(view(&contract, &id).status, EscrowStatus::ApproverBound);
    assert_eq!(
        near_sdk::test_utils::get_logs().last().unwrap(),
        "returning 1500000000 to client.test.near: unsupported token"
    );
}

#[test]
fn unknown_or_malformed_escrow_id_is_returned() {
    let mut contract = new_contract();
    bound_escrow(&mut contract);
    for msg in ["2", "01", "", "1 ", "escrow-1", "{\"escrow_id\":\"1\"}"] {
        assert_eq!(
            fund(&mut contract, msg, usdc(1_500)),
            usdc(1_500),
            "{msg:?}"
        );
    }
    assert_eq!(view(&contract, "1").status, EscrowStatus::ApproverBound);
}

#[test]
#[should_panic(expected = "only an unfunded escrow can be cancelled")]
fn funded_escrow_cannot_be_cancelled() {
    let mut contract = new_contract();
    let id = funded_escrow(&mut contract, &Wallet::near());
    set_context(FREELANCER, T0_MS);
    contract.cancel(id);
}

#[test]
#[should_panic(expected = "approver is already bound")]
fn funded_escrow_cannot_be_rebound() {
    let mut contract = new_contract();
    let id = funded_escrow(&mut contract, &Wallet::near());
    bind(&mut contract, &id, &Wallet::evm(), 1);
}
