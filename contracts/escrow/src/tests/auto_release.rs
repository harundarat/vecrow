use super::*;

const DAY_MS: u64 = 24 * 60 * 60 * 1_000;
const WINDOW_MS: u64 = WINDOW_SEC * 1_000;

fn submitted(contract: &mut VecrowEscrow, wallet: &Wallet) -> String {
    let id = funded_escrow(contract, wallet);
    submit(contract, &id, 0, T0_MS);
    id
}

#[test]
#[should_panic(expected = "review window has not ended")]
fn rejected_before_the_window_ends() {
    let mut contract = new_contract();
    let id = submitted(&mut contract, &Wallet::near());
    auto_release(&mut contract, &id, 0, T0_MS + WINDOW_MS - 1);
}

#[test]
fn anyone_releases_once_the_window_ends() {
    let mut contract = new_contract();
    let id = submitted(&mut contract, &Wallet::near());
    auto_release(&mut contract, &id, 0, T0_MS + WINDOW_MS);

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
    assert_eq!(escrow.balance, U128(usdc(500)));
    assert!(near_sdk::test_utils::get_logs().contains(
        &r#"EVENT_JSON:{"standard":"vecrow","version":"1.0.0","event":"milestone_released","data":{"escrow_id":"e1","milestone":0,"amount":"1000000000","fee":"5000000","by":"auto_release"}}"#
            .to_string()
    ));
}

#[test]
fn window_is_measured_from_the_latest_submission() {
    let mut contract = new_contract();
    let wallet = Wallet::evm();
    let id = submitted(&mut contract, &wallet);
    request_revision(&mut contract, &id, 0, &wallet, 1, T0_MS + 1_800_000);
    let resubmitted_at = T0_MS + 10 * DAY_MS;
    submit(&mut contract, &id, 0, resubmitted_at);

    let early = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
        auto_release(&mut contract, &id, 0, T0_MS + WINDOW_MS)
    }));
    assert!(early.is_err());
    let early = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
        auto_release(&mut contract, &id, 0, resubmitted_at + WINDOW_MS - 1)
    }));
    assert!(early.is_err());
    assert_eq!(
        view(&contract, &id).milestones[0].status,
        MilestoneStatus::Submitted
    );

    auto_release(&mut contract, &id, 0, resubmitted_at + WINDOW_MS);
    assert_eq!(
        view(&contract, &id).milestones[0].status,
        MilestoneStatus::Released
    );
}

#[test]
#[should_panic(expected = "a milestone awaiting revision cannot be auto-released")]
fn rejected_while_a_revision_is_requested() {
    let mut contract = new_contract();
    let wallet = Wallet::solana();
    let id = submitted(&mut contract, &wallet);
    request_revision(&mut contract, &id, 0, &wallet, 1, T0_MS + 1);
    auto_release(&mut contract, &id, 0, T0_MS + 100 * DAY_MS);
}

#[test]
#[should_panic(expected = "only a Submitted milestone can be auto-released")]
fn pending_milestone_cannot_be_auto_released() {
    let mut contract = new_contract();
    let id = funded_escrow(&mut contract, &Wallet::near());
    auto_release(&mut contract, &id, 0, T0_MS + 100 * DAY_MS);
}

#[test]
#[should_panic(expected = "only a Submitted milestone can be auto-released")]
fn released_milestone_cannot_be_released_twice() {
    let mut contract = new_contract();
    let id = submitted(&mut contract, &Wallet::near());
    auto_release(&mut contract, &id, 0, T0_MS + WINDOW_MS);
    auto_release(&mut contract, &id, 0, T0_MS + WINDOW_MS);
}

#[test]
#[should_panic(expected = "escrow is not funded")]
fn unfunded_escrow_cannot_be_auto_released() {
    let mut contract = new_contract();
    let id = create_default(&mut contract);
    auto_release(&mut contract, &id, 0, T0_MS + 100 * DAY_MS);
}

#[test]
fn window_is_configurable_at_deploy_time() {
    set_context(CONTRACT, T0_MS);
    let mut contract = VecrowEscrow::new(account(USDC), account(TREASURY), FEE_BPS, 60);
    let id = funded_escrow(&mut contract, &Wallet::near());
    assert_eq!(view(&contract, &id).review_window_sec, 60);
    submit(&mut contract, &id, 0, T0_MS);
    let early = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
        auto_release(&mut contract, &id, 0, T0_MS + 59_999)
    }));
    assert!(early.is_err());
    auto_release(&mut contract, &id, 0, T0_MS + 60_000);
    assert_eq!(
        view(&contract, &id).milestones[0].status,
        MilestoneStatus::Released
    );
}

#[test]
fn window_is_locked_per_escrow() {
    let mut contract = new_contract();
    let id = submitted(&mut contract, &Wallet::near());
    contract.config.review_window_sec = 60;
    let early = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
        auto_release(&mut contract, &id, 0, T0_MS + 60_000)
    }));
    assert!(early.is_err(), "the escrow keeps its 14-day window");
}

#[test]
fn auto_release_of_the_last_milestone_completes_the_escrow() {
    let mut contract = new_contract();
    let wallet = Wallet::near();
    let id = submitted(&mut contract, &wallet);
    submit(&mut contract, &id, 1, T0_MS);
    approve(&mut contract, &id, 1, &wallet, 1, T0_MS);
    auto_release(&mut contract, &id, 0, T0_MS + WINDOW_MS);
    assert_eq!(view(&contract, &id).status, EscrowStatus::Completed);
}
