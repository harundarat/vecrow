//! Revenue split rules (R-03).

use crate::{BPS_DENOMINATOR, SplitShare};
use near_sdk::{env, require};

/// Upper bound on split recipients per escrow, to bound the gas of a release.
pub const MAX_SPLIT_RECIPIENTS: usize = 10;

/// Validates a split: 1 to `MAX_SPLIT_RECIPIENTS` distinct recipients, each
/// with a positive share, totalling exactly 10,000 bps. The escrow contract
/// itself cannot be a recipient, because a token transfer to itself always
/// fails.
pub fn validate_split(split: &[SplitShare]) {
    require!(!split.is_empty(), "split must have at least one recipient");
    require!(
        split.len() <= MAX_SPLIT_RECIPIENTS,
        "split has too many recipients"
    );
    let contract = env::current_account_id();
    let mut total: u32 = 0;
    for (i, share) in split.iter().enumerate() {
        require!(share.bps > 0, "split shares must be positive");
        require!(
            share.account_id != contract,
            "the escrow contract cannot be a split recipient"
        );
        require!(
            split[..i].iter().all(|s| s.account_id != share.account_id),
            "split recipients must be distinct"
        );
        total += u32::from(share.bps);
    }
    require!(
        total == u32::from(BPS_DENOMINATOR),
        "split must total exactly 10000 bps"
    );
}

/// How one milestone release is paid out (R-03, R-19).
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct ReleasePlan {
    /// Platform fee, sent to `treasury_account`.
    pub fee: u128,
    /// One share per split recipient, in the order of the split.
    pub shares: Vec<u128>,
}

/// Computes the payout of a released milestone:
///
/// - `fee = floor(amount × fee_bps / 10,000)`;
/// - each recipient gets `floor((amount − fee) × bps / 10,000)`;
/// - the rounding remainder goes to the first recipient.
///
/// Panics unless the fee and all shares add up to exactly `amount`, so no
/// dust is ever left in the contract.
pub fn release_plan(amount: u128, fee_bps: u16, split: &[SplitShare]) -> ReleasePlan {
    let fee = mul_bps(amount, fee_bps);
    let distributable = amount - fee;
    let mut shares: Vec<u128> = split
        .iter()
        .map(|s| mul_bps(distributable, s.bps))
        .collect();
    let assigned: u128 = shares.iter().sum();
    shares[0] += distributable - assigned;
    require!(
        fee + shares.iter().sum::<u128>() == amount,
        "fee and shares must add up to the milestone amount"
    );
    ReleasePlan { fee, shares }
}

/// `floor(amount × bps / 10,000)` for `bps ≤ 10,000`, without overflowing
/// for any `u128` amount: with `amount = q × 10,000 + r`, the result is
/// `q × bps + floor(r × bps / 10,000)`.
pub fn mul_bps(amount: u128, bps: u16) -> u128 {
    let denominator = u128::from(BPS_DENOMINATOR);
    let bps = u128::from(bps);
    (amount / denominator) * bps + (amount % denominator) * bps / denominator
}
