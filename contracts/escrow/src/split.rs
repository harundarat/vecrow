use crate::{BPS_DENOMINATOR, SplitShare};
use near_sdk::{env, require};

pub const MAX_SPLIT_RECIPIENTS: usize = 10;

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

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct ReleasePlan {
    pub fee: u128,
    pub shares: Vec<u128>,
}

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

pub fn mul_bps(amount: u128, bps: u16) -> u128 {
    let denominator = u128::from(BPS_DENOMINATOR);
    let bps = u128::from(bps);
    // Split into quotient and remainder so `amount * bps` cannot overflow.
    (amount / denominator) * bps + (amount % denominator) * bps / denominator
}
