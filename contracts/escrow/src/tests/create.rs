//! R-01 (create and cancel) and the R-03 split validation.

use super::*;

#[test]
fn create_escrow_stores_freelancer_milestones_split_and_window() {
    let mut contract = new_contract();
    let id = create_default(&mut contract);
    assert_eq!(id, "1");

    let escrow = view(&contract, &id);
    assert_eq!(escrow.escrow_id, "1");
    assert_eq!(escrow.freelancer, account(FREELANCER));
    assert_eq!(escrow.status, EscrowStatus::Created);
    assert_eq!(escrow.split, split_70_30());
    assert_eq!(escrow.fee_bps, FEE_BPS);
    assert_eq!(escrow.review_window_sec, WINDOW_SEC);
    assert_eq!(escrow.total, U128(usdc(1_500)));
    assert_eq!(escrow.created_at_ms, T0_MS);
    assert_eq!(escrow.milestones.len(), 2);
    for (m, amount) in escrow.milestones.iter().zip([usdc(1_000), usdc(500)]) {
        assert_eq!(m.amount, U128(amount));
        assert_eq!(m.status, MilestoneStatus::Pending);
        assert_eq!(m.submitted_at_ms, None);
        assert_eq!(m.revision_count, 0);
    }
}

#[test]
fn escrow_ids_are_sequential() {
    let mut contract = new_contract();
    assert_eq!(create_default(&mut contract), "1");
    assert_eq!(create_default(&mut contract), "2");
}

#[test]
fn create_escrow_emits_event() {
    let mut contract = new_contract();
    create_default(&mut contract);
    let logs = near_sdk::test_utils::get_logs();
    assert_eq!(
        logs.last().unwrap(),
        r#"EVENT_JSON:{"standard":"vecrow","version":"1.0.0","event":"escrow_created","data":{"escrow_id":"1","freelancer":"freelancer.test.near","total":"1500000000"}}"#
    );
}

#[test]
fn statuses_serialize_as_pascal_case() {
    let json = near_sdk::serde_json::to_string(&[
        EscrowStatus::Created,
        EscrowStatus::ApproverBound,
        EscrowStatus::Funded,
        EscrowStatus::Completed,
        EscrowStatus::Cancelled,
    ])
    .unwrap();
    assert_eq!(
        json,
        r#"["Created","ApproverBound","Funded","Completed","Cancelled"]"#
    );
    let json = near_sdk::serde_json::to_string(&[
        MilestoneStatus::Pending,
        MilestoneStatus::Submitted,
        MilestoneStatus::RevisionRequested,
        MilestoneStatus::Released,
        MilestoneStatus::Refunded,
    ])
    .unwrap();
    assert_eq!(
        json,
        r#"["Pending","Submitted","RevisionRequested","Released","Refunded"]"#
    );
}

#[test]
fn unknown_or_non_canonical_escrow_ids_are_not_found() {
    let mut contract = new_contract();
    create_default(&mut contract);
    for id in [
        "2",
        "0",
        "01",
        "+1",
        " 1",
        "1 ",
        "",
        "x",
        "18446744073709551616",
    ] {
        assert!(contract.get_escrow(id.to_string()).is_none(), "{id:?}");
    }
}

#[test]
#[should_panic(expected = "split must total exactly 10000 bps")]
fn split_below_10000_bps_is_rejected() {
    let mut contract = new_contract();
    set_context(FREELANCER, T0_MS);
    contract.create_escrow(
        vec![U128(usdc(100))],
        vec![share(FREELANCER, 7_000), share(MEMBER, 2_999)],
    );
}

#[test]
#[should_panic(expected = "split must total exactly 10000 bps")]
fn split_above_10000_bps_is_rejected() {
    let mut contract = new_contract();
    set_context(FREELANCER, T0_MS);
    contract.create_escrow(
        vec![U128(usdc(100))],
        vec![share(FREELANCER, 7_000), share(MEMBER, 3_001)],
    );
}

#[test]
#[should_panic(expected = "split must have at least one recipient")]
fn empty_split_is_rejected() {
    let mut contract = new_contract();
    set_context(FREELANCER, T0_MS);
    contract.create_escrow(vec![U128(usdc(100))], vec![]);
}

#[test]
#[should_panic(expected = "split shares must be positive")]
fn zero_share_is_rejected() {
    let mut contract = new_contract();
    set_context(FREELANCER, T0_MS);
    contract.create_escrow(
        vec![U128(usdc(100))],
        vec![share(FREELANCER, 10_000), share(MEMBER, 0)],
    );
}

#[test]
#[should_panic(expected = "split recipients must be distinct")]
fn duplicate_recipient_is_rejected() {
    let mut contract = new_contract();
    set_context(FREELANCER, T0_MS);
    contract.create_escrow(
        vec![U128(usdc(100))],
        vec![share(MEMBER, 5_000), share(MEMBER, 5_000)],
    );
}

#[test]
#[should_panic(expected = "split has too many recipients")]
fn too_many_recipients_are_rejected() {
    let mut contract = new_contract();
    set_context(FREELANCER, T0_MS);
    let split = (0..=MAX_SPLIT_RECIPIENTS)
        .map(|i| share(&format!("m{i}.test.near"), 1))
        .collect();
    contract.create_escrow(vec![U128(usdc(100))], split);
}

#[test]
#[should_panic(expected = "the escrow contract cannot be a split recipient")]
fn contract_as_recipient_is_rejected() {
    let mut contract = new_contract();
    set_context(FREELANCER, T0_MS);
    contract.create_escrow(vec![U128(usdc(100))], vec![share(CONTRACT, 10_000)]);
}

#[test]
#[should_panic(expected = "at least one milestone is required")]
fn no_milestones_is_rejected() {
    let mut contract = new_contract();
    set_context(FREELANCER, T0_MS);
    contract.create_escrow(vec![], split_70_30());
}

#[test]
#[should_panic(expected = "milestone amounts must be positive")]
fn zero_milestone_is_rejected() {
    let mut contract = new_contract();
    set_context(FREELANCER, T0_MS);
    contract.create_escrow(vec![U128(usdc(100)), U128(0)], split_70_30());
}

#[test]
#[should_panic(expected = "too many milestones")]
fn too_many_milestones_are_rejected() {
    let mut contract = new_contract();
    set_context(FREELANCER, T0_MS);
    contract.create_escrow(vec![U128(1); MAX_MILESTONES + 1], split_70_30());
}

#[test]
#[should_panic(expected = "milestone total overflows")]
fn overflowing_total_is_rejected() {
    let mut contract = new_contract();
    set_context(FREELANCER, T0_MS);
    contract.create_escrow(vec![U128(u128::MAX), U128(1)], split_70_30());
}

#[test]
fn freelancer_cancels_created_escrow() {
    let mut contract = new_contract();
    let id = create_default(&mut contract);
    set_context(FREELANCER, T0_MS);
    contract.cancel(id.clone());
    assert_eq!(view(&contract, &id).status, EscrowStatus::Cancelled);
}

#[test]
#[should_panic(expected = "only the freelancer can call this method")]
fn stranger_cannot_cancel() {
    let mut contract = new_contract();
    let id = create_default(&mut contract);
    set_context(STRANGER, T0_MS);
    contract.cancel(id);
}

#[test]
#[should_panic(expected = "only an unfunded escrow can be cancelled")]
fn cancelled_escrow_cannot_be_cancelled_again() {
    let mut contract = new_contract();
    let id = create_default(&mut contract);
    set_context(FREELANCER, T0_MS);
    contract.cancel(id.clone());
    contract.cancel(id);
}

#[test]
#[should_panic(expected = "unknown escrow")]
fn cancel_unknown_escrow_is_rejected() {
    let mut contract = new_contract();
    set_context(FREELANCER, T0_MS);
    contract.cancel("42".to_string());
}

#[test]
#[should_panic(expected = "fee_bps must be at most 10000")]
fn init_rejects_fee_above_100_percent() {
    set_context(CONTRACT, T0_MS);
    VecrowEscrow::new(account(USDC), account(TREASURY), 10_001, WINDOW_SEC);
}

#[test]
#[should_panic(expected = "review_window_sec must be positive")]
fn init_rejects_zero_review_window() {
    set_context(CONTRACT, T0_MS);
    VecrowEscrow::new(account(USDC), account(TREASURY), FEE_BPS, 0);
}
