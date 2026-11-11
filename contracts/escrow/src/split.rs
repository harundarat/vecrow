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
