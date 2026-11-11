//! R-17 (request a revision) and the R-05 re-submission rule.

use super::*;

fn submitted(contract: &mut VecrowEscrow, wallet: &Wallet) -> String {
    let id = funded_escrow(contract, wallet);
    submit(contract, &id, 0, T0_MS);
    id
}

#[test]
fn each_wallet_scheme_requests_a_revision() {
    for wallet in Wallet::all() {
        let mut contract = new_contract();
        let id = submitted(&mut contract, &wallet);
        request_revision(&mut contract, &id, 0, &wallet, 1, T0_MS + 1);

        let m = &view(&contract, &id).milestones[0];
        assert_eq!(m.status, MilestoneStatus::RevisionRequested);
        assert_eq!(m.revision_count, 1);
        assert_eq!(m.submitted_at_ms, None, "the review timer stops");
        assert!(ft_transfers().is_empty(), "no funds move");
        assert_eq!(
            near_sdk::test_utils::get_logs().last().unwrap(),
            r#"EVENT_JSON:{"standard":"vecrow","version":"1.0.0","event":"revision_requested","data":{"escrow_id":"e1","milestone":0,"revision_count":1}}"#
        );
    }
}

#[test]
fn resubmission_starts_a_new_review_window() {
    let mut contract = new_contract();
    let wallet = Wallet::near();
    let id = submitted(&mut contract, &wallet);
    request_revision(&mut contract, &id, 0, &wallet, 1, T0_MS + 1_000);
    submit(&mut contract, &id, 0, T0_MS + 5_000);

    let m = &view(&contract, &id).milestones[0];
    assert_eq!(m.status, MilestoneStatus::Submitted);
    assert_eq!(m.submitted_at_ms, Some(T0_MS + 5_000));
    assert_eq!(m.revision_count, 1);
}

#[test]
#[should_panic(expected = "revision limit reached")]
fn third_revision_request_is_rejected() {
    let mut contract = new_contract();
    let wallet = Wallet::evm();
    let id = submitted(&mut contract, &wallet);
    request_revision(&mut contract, &id, 0, &wallet, 1, T0_MS);
    submit(&mut contract, &id, 0, T0_MS);
    request_revision(&mut contract, &id, 0, &wallet, 2, T0_MS);
    submit(&mut contract, &id, 0, T0_MS);
    assert_eq!(view(&contract, &id).milestones[0].revision_count, 2);
    request_revision(&mut contract, &id, 0, &wallet, 3, T0_MS);
}

#[test]
fn revision_limit_is_per_milestone_and_approval_still_works() {
    let mut contract = new_contract();
    let wallet = Wallet::solana();
    let id = submitted(&mut contract, &wallet);
    for nonce in [1, 2] {
        request_revision(&mut contract, &id, 0, &wallet, nonce, T0_MS);
        submit(&mut contract, &id, 0, T0_MS);
    }
    // Milestone 1 still has its own two revisions.
    submit(&mut contract, &id, 1, T0_MS);
    request_revision(&mut contract, &id, 1, &wallet, 3, T0_MS);
    assert_eq!(view(&contract, &id).milestones[1].revision_count, 1);

    approve(&mut contract, &id, 0, &wallet, 4, T0_MS);
    assert_eq!(
        view(&contract, &id).milestones[0].status,
        MilestoneStatus::Released
    );
}

#[test]
#[should_panic(expected = "invalid signature")]
fn revision_not_signed_by_the_approver_is_rejected() {
    let mut contract = new_contract();
    let id = submitted(&mut contract, &Wallet::near());
    request_revision(
        &mut contract,
        &id,
        0,
        &Wallet::Near(NearWallet::new(CLIENT, 99)),
        1,
        T0_MS,
    );
}

#[test]
#[should_panic(expected = "invalid signature")]
fn approval_signature_cannot_request_a_revision() {
    let mut contract = new_contract();
    let wallet = Wallet::near();
    let id = submitted(&mut contract, &wallet);
    let (nonce, signature) = sign_milestone(&wallet, "approve_milestone", &id, 0, 1, DEADLINE_MS);
    set_context(RELAYER, T0_MS);
    contract.request_revision(id, 0, nonce, DEADLINE_MS, signature);
}

#[test]
#[should_panic(expected = "only a Submitted milestone can be approved or sent back for revision")]
fn pending_milestone_cannot_be_sent_back() {
    let mut contract = new_contract();
    let wallet = Wallet::evm();
    let id = funded_escrow(&mut contract, &wallet);
    request_revision(&mut contract, &id, 0, &wallet, 1, T0_MS);
}

#[test]
#[should_panic(expected = "only a Submitted milestone can be approved or sent back for revision")]
fn released_milestone_cannot_be_sent_back() {
    let mut contract = new_contract();
    let wallet = Wallet::evm();
    let id = submitted(&mut contract, &wallet);
    approve(&mut contract, &id, 0, &wallet, 1, T0_MS);
    request_revision(&mut contract, &id, 0, &wallet, 2, T0_MS);
}

#[test]
#[should_panic(expected = "only a Submitted milestone can be approved or sent back for revision")]
fn revision_cannot_be_requested_twice_without_resubmission() {
    let mut contract = new_contract();
    let wallet = Wallet::solana();
    let id = submitted(&mut contract, &wallet);
    request_revision(&mut contract, &id, 0, &wallet, 1, T0_MS);
    request_revision(&mut contract, &id, 0, &wallet, 2, T0_MS);
}

#[test]
#[should_panic(expected = "only a Submitted milestone can be approved or sent back for revision")]
fn milestone_awaiting_revision_cannot_be_approved() {
    let mut contract = new_contract();
    let wallet = Wallet::near();
    let id = submitted(&mut contract, &wallet);
    request_revision(&mut contract, &id, 0, &wallet, 1, T0_MS);
    approve(&mut contract, &id, 0, &wallet, 2, T0_MS);
}

#[test]
#[should_panic(expected = "nonce already used")]
fn reused_revision_nonce_is_rejected() {
    let mut contract = new_contract();
    let wallet = Wallet::evm();
    let id = submitted(&mut contract, &wallet);
    request_revision(&mut contract, &id, 0, &wallet, 1, T0_MS);
    submit(&mut contract, &id, 0, T0_MS);
    request_revision(&mut contract, &id, 0, &wallet, 1, T0_MS);
}

#[test]
#[should_panic(expected = "signature expired")]
fn expired_revision_request_is_rejected() {
    let mut contract = new_contract();
    let wallet = Wallet::near();
    let id = submitted(&mut contract, &wallet);
    request_revision(&mut contract, &id, 0, &wallet, 1, DEADLINE_MS + 1);
}
