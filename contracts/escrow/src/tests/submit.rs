//! R-05: submit a milestone.

use super::*;

#[test]
fn freelancer_submits_a_pending_milestone() {
    let mut contract = new_contract();
    let id = funded_escrow(&mut contract, &Wallet::near());
    submit(&mut contract, &id, 1, T0_MS + 5);

    let escrow = view(&contract, &id);
    assert_eq!(escrow.milestones[1].status, MilestoneStatus::Submitted);
    assert_eq!(escrow.milestones[1].submitted_at_ms, Some(T0_MS + 5));
    assert_eq!(escrow.milestones[0].status, MilestoneStatus::Pending);
    assert_eq!(
        near_sdk::test_utils::get_logs().last().unwrap(),
        r#"EVENT_JSON:{"standard":"vecrow","version":"1.0.0","event":"milestone_submitted","data":{"escrow_id":"1","milestone":1,"submitted_at_ms":1700000000005}}"#
    );
}

#[test]
#[should_panic(expected = "only the freelancer can call this method")]
fn only_the_freelancer_can_submit() {
    let mut contract = new_contract();
    let id = funded_escrow(&mut contract, &Wallet::near());
    set_context(MEMBER, T0_MS);
    contract.submit_milestone(id, 0);
}

#[test]
#[should_panic(expected = "only the freelancer can call this method")]
fn relayer_cannot_submit_for_the_freelancer() {
    let mut contract = new_contract();
    let id = funded_escrow(&mut contract, &Wallet::near());
    set_context(RELAYER, T0_MS);
    contract.submit_milestone(id, 0);
}

#[test]
fn unfunded_escrows_reject_submission() {
    let mut contract = new_contract();
    let created = create_default(&mut contract);
    let bound = create_default(&mut contract);
    bind(&mut contract, &bound, &Wallet::evm(), 1);
    let cancelled = create_default(&mut contract);
    set_context(FREELANCER, T0_MS);
    contract.cancel(cancelled.clone());

    for id in [created, bound, cancelled] {
        let result = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
            submit(&mut contract, &id, 0, T0_MS)
        }));
        assert!(result.is_err(), "escrow {id}");
        assert_eq!(
            view(&contract, &id).milestones[0].status,
            MilestoneStatus::Pending
        );
    }
}

#[test]
#[should_panic(expected = "escrow is not funded")]
fn approver_bound_escrow_rejects_submission() {
    let mut contract = new_contract();
    let id = create_default(&mut contract);
    bind(&mut contract, &id, &Wallet::evm(), 1);
    submit(&mut contract, &id, 0, T0_MS);
}

#[test]
#[should_panic(expected = "only a Pending or RevisionRequested milestone can be submitted")]
fn submitted_milestone_cannot_be_submitted_again() {
    let mut contract = new_contract();
    let id = funded_escrow(&mut contract, &Wallet::near());
    submit(&mut contract, &id, 0, T0_MS);
    submit(&mut contract, &id, 0, T0_MS + 1);
}

#[test]
#[should_panic(expected = "unknown milestone")]
fn unknown_milestone_is_rejected() {
    let mut contract = new_contract();
    let id = funded_escrow(&mut contract, &Wallet::near());
    submit(&mut contract, &id, 2, T0_MS);
}
