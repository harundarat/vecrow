use super::*;

#[test]
fn freelancer_refunds_one_pending_milestone_without_fee() {
    let mut contract = new_contract();
    let id = funded_escrow(&mut contract, &Wallet::near());
    refund(&mut contract, FREELANCER, &id, Some(1));

    assert_eq!(ft_transfers(), vec![(REFUND.to_string(), usdc(500))]);
    let escrow = view(&contract, &id);
    assert_eq!(escrow.milestones[1].status, MilestoneStatus::Refunded);
    assert_eq!(escrow.milestones[0].status, MilestoneStatus::Pending);
    assert_eq!(escrow.balance, U128(usdc(1_000)));
    assert_eq!(escrow.status, EscrowStatus::Funded);
    assert!(near_sdk::test_utils::get_logs().contains(
        &r#"EVENT_JSON:{"standard":"vecrow","version":"1.0.0","event":"milestones_refunded","data":{"escrow_id":"e1","milestones":[1],"amount":"500000000","refund_account":"refund.test.near"}}"#
            .to_string()
    ));
}

#[test]
fn refunding_all_pending_milestones_sends_one_transfer_and_completes() {
    let mut contract = new_contract();
    let id = funded_escrow(&mut contract, &Wallet::evm());
    refund(&mut contract, FREELANCER, &id, None);

    assert_eq!(ft_transfers(), vec![(REFUND.to_string(), usdc(1_500))]);
    let escrow = view(&contract, &id);
    assert!(
        escrow
            .milestones
            .iter()
            .all(|m| m.status == MilestoneStatus::Refunded)
    );
    assert_eq!(escrow.balance, U128(0));
    assert_eq!(escrow.status, EscrowStatus::Completed);
}

#[test]
fn refund_all_skips_milestones_that_are_not_pending() {
    let mut contract = new_contract();
    let id = funded_escrow(&mut contract, &Wallet::solana());
    submit(&mut contract, &id, 0, T0_MS);
    refund(&mut contract, FREELANCER, &id, None);

    assert_eq!(ft_transfers(), vec![(REFUND.to_string(), usdc(500))]);
    let escrow = view(&contract, &id);
    assert_eq!(escrow.milestones[0].status, MilestoneStatus::Submitted);
    assert_eq!(escrow.milestones[1].status, MilestoneStatus::Refunded);
    assert_eq!(escrow.status, EscrowStatus::Funded);
}

#[test]
fn released_and_refunded_milestones_complete_the_escrow() {
    let mut contract = new_contract();
    let wallet = Wallet::near();
    let id = funded_escrow(&mut contract, &wallet);
    submit(&mut contract, &id, 0, T0_MS);
    approve(&mut contract, &id, 0, &wallet, 1, T0_MS);
    refund(&mut contract, FREELANCER, &id, Some(1));
    assert_eq!(view(&contract, &id).status, EscrowStatus::Completed);
}

#[test]
fn only_the_freelancer_can_refund() {
    let mut contract = new_contract();
    let id = funded_escrow(&mut contract, &Wallet::near());
    for caller in [CLIENT, REFUND, MEMBER, RELAYER, STRANGER] {
        let result = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
            refund(&mut contract, caller, &id, None)
        }));
        assert!(result.is_err(), "{caller}");
    }
    assert!(
        view(&contract, &id)
            .milestones
            .iter()
            .all(|m| m.status == MilestoneStatus::Pending)
    );
}

#[test]
#[should_panic(expected = "only the freelancer can call this method")]
fn client_cannot_trigger_a_refund() {
    let mut contract = new_contract();
    let id = funded_escrow(&mut contract, &Wallet::near());
    refund(&mut contract, CLIENT, &id, Some(0));
}

#[test]
#[should_panic(expected = "only a Pending milestone can be refunded")]
fn submitted_milestone_cannot_be_refunded() {
    let mut contract = new_contract();
    let id = funded_escrow(&mut contract, &Wallet::near());
    submit(&mut contract, &id, 0, T0_MS);
    refund(&mut contract, FREELANCER, &id, Some(0));
}

#[test]
#[should_panic(expected = "only a Pending milestone can be refunded")]
fn milestone_awaiting_revision_cannot_be_refunded() {
    let mut contract = new_contract();
    let wallet = Wallet::evm();
    let id = funded_escrow(&mut contract, &wallet);
    submit(&mut contract, &id, 0, T0_MS);
    request_revision(&mut contract, &id, 0, &wallet, 1, T0_MS);
    refund(&mut contract, FREELANCER, &id, Some(0));
}

#[test]
#[should_panic(expected = "only a Pending milestone can be refunded")]
fn released_milestone_cannot_be_refunded() {
    let mut contract = new_contract();
    let wallet = Wallet::solana();
    let id = funded_escrow(&mut contract, &wallet);
    submit(&mut contract, &id, 0, T0_MS);
    approve(&mut contract, &id, 0, &wallet, 1, T0_MS);
    refund(&mut contract, FREELANCER, &id, Some(0));
}

#[test]
#[should_panic(expected = "only a Pending milestone can be refunded")]
fn refunded_milestone_cannot_be_refunded_again() {
    let mut contract = new_contract();
    let id = funded_escrow(&mut contract, &Wallet::near());
    refund(&mut contract, FREELANCER, &id, Some(1));
    refund(&mut contract, FREELANCER, &id, Some(1));
}

#[test]
#[should_panic(expected = "no Pending milestone to refund")]
fn refund_all_without_pending_milestones_is_rejected() {
    let mut contract = new_contract();
    let id = funded_escrow(&mut contract, &Wallet::near());
    submit(&mut contract, &id, 0, T0_MS);
    submit(&mut contract, &id, 1, T0_MS);
    refund(&mut contract, FREELANCER, &id, None);
}

#[test]
#[should_panic(expected = "escrow is not funded")]
fn unfunded_escrow_cannot_be_refunded() {
    let mut contract = new_contract();
    let id = create_default(&mut contract);
    refund(&mut contract, FREELANCER, &id, None);
}
