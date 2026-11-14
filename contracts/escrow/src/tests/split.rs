use super::*;
use crate::split::{ReleasePlan, mul_bps, release_plan};

#[test]
fn fee_and_70_30_split_of_1000_usdc() {
    let plan = release_plan(usdc(1_000), FEE_BPS, &split_70_30());
    assert_eq!(
        plan,
        ReleasePlan {
            fee: usdc(5),
            shares: vec![696_500_000, 298_500_000],
        }
    );
    assert_eq!(plan.fee + plan.shares.iter().sum::<u128>(), usdc(1_000));
}

#[test]
fn three_recipients_get_exact_shares() {
    let split = vec![
        share(FREELANCER, 5_000),
        share(MEMBER, 3_000),
        share(MEMBER2, 2_000),
    ];
    let plan = release_plan(usdc(1_000), FEE_BPS, &split);
    assert_eq!(plan.fee, usdc(5));
    assert_eq!(plan.shares, vec![497_500_000, 298_500_000, 199_000_000]);
}

#[test]
fn rounding_remainder_goes_to_first_recipient() {
    let split = vec![
        share(FREELANCER, 5_000),
        share(MEMBER, 3_000),
        share(MEMBER2, 2_000),
    ];
    // Fee 5_000; the floored shares of 995_001 sum to 995_000, and the
    // remaining 1 goes to the first recipient.
    let plan = release_plan(1_000_001, FEE_BPS, &split);
    assert_eq!(plan.fee, 5_000);
    assert_eq!(plan.shares, vec![497_501, 298_500, 199_000]);
}

#[test]
fn small_amounts_floor_the_fee_to_zero() {
    let plan = release_plan(199, FEE_BPS, &split_70_30());
    assert_eq!(plan.fee, 0);
    assert_eq!(plan.shares, vec![140, 59]);
    let plan = release_plan(1, FEE_BPS, &split_70_30());
    assert_eq!(
        plan,
        ReleasePlan {
            fee: 0,
            shares: vec![1, 0]
        }
    );
}

#[test]
fn total_paid_out_always_equals_the_milestone_amount() {
    let splits = [
        vec![share(FREELANCER, 10_000)],
        split_70_30(),
        vec![
            share(FREELANCER, 3_333),
            share(MEMBER, 3_333),
            share(MEMBER2, 3_334),
        ],
        vec![
            share(FREELANCER, 1),
            share(MEMBER, 9_998),
            share(MEMBER2, 1),
        ],
    ];
    let mut amount: u128 = 1;
    while amount < 10u128.pow(30) {
        for fee_bps in [0, 1, FEE_BPS, 9_999, 10_000] {
            for split in &splits {
                for delta in [0, 1, 7, 9_999] {
                    let total = amount + delta;
                    let plan = release_plan(total, fee_bps, split);
                    assert_eq!(plan.fee + plan.shares.iter().sum::<u128>(), total);
                    assert_eq!(plan.fee, total * u128::from(fee_bps) / 10_000);
                    let exact = (total - plan.fee) * u128::from(split[0].bps) / 10_000;
                    assert!(plan.shares[0] - exact < split.len() as u128);
                }
            }
        }
        amount = amount * 3 + 1;
    }
}

#[test]
fn mul_bps_does_not_overflow() {
    assert_eq!(mul_bps(u128::MAX, 10_000), u128::MAX);
    assert_eq!(mul_bps(u128::MAX, 0), 0);
    assert_eq!(mul_bps(u128::MAX, 5_000), u128::MAX / 2);
    let plan = release_plan(u128::MAX, FEE_BPS, &split_70_30());
    assert_eq!(plan.fee + plan.shares[0] + plan.shares[1], u128::MAX);
}

#[test]
fn create_escrow_locks_the_fee_and_window_of_the_time() {
    let mut contract = new_contract();
    let old = create_default(&mut contract);
    contract.config.fee_bps = 100;
    contract.config.review_window_sec = 60;
    let wallet = Wallet::near();
    let mut terms = default_terms("e2", &wallet);
    terms.fee_bps = 100;
    create_with(
        &mut contract,
        sign_terms(terms, &freelancer_wallet(), &wallet),
    );

    assert_eq!(view(&contract, &old).fee_bps, FEE_BPS);
    assert_eq!(view(&contract, &old).review_window_sec, WINDOW_SEC);
    assert_eq!(view(&contract, "e2").fee_bps, 100);
    assert_eq!(view(&contract, "e2").review_window_sec, 60);
}
