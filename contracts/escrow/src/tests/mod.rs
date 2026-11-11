//! Unit tests for the contract logic, run against near-sdk's mocked
//! blockchain. Cross-contract flows (token transfers and callbacks) are
//! covered by the sandbox tests in `integration-tests/`.

mod create;
mod split;

use crate::*;
use near_sdk::json_types::U128;
use near_sdk::test_utils::VMContextBuilder;
use near_sdk::{NearToken, testing_env};

pub const CONTRACT: &str = "escrow.test.near";
pub const USDC: &str = "usdc.test.near";
pub const TREASURY: &str = "treasury.test.near";
pub const FREELANCER: &str = "freelancer.test.near";
pub const MEMBER: &str = "member.test.near";
pub const MEMBER2: &str = "member2.test.near";
pub const STRANGER: &str = "stranger.test.near";

/// 2023-11-14T22:13:20Z: a realistic block time for the tests.
pub const T0_MS: u64 = 1_700_000_000_000;
/// The production fee: 0.5%.
pub const FEE_BPS: u16 = 50;
/// The production review window: 14 days.
pub const WINDOW_SEC: u64 = 14 * 24 * 60 * 60;

pub fn account(id: &str) -> AccountId {
    id.parse().unwrap()
}

/// USDC amount in base units (6 decimals).
pub fn usdc(whole: u128) -> u128 {
    whole * 1_000_000
}

/// Sets the mocked context: `predecessor` calls the contract at `now_ms`,
/// with no attached deposit, as every user-facing call must allow.
pub fn set_context(predecessor: &str, now_ms: u64) {
    testing_env!(
        VMContextBuilder::new()
            .current_account_id(account(CONTRACT))
            .predecessor_account_id(account(predecessor))
            .signer_account_id(account(predecessor))
            .block_timestamp(now_ms * 1_000_000)
            .attached_deposit(NearToken::from_yoctonear(0))
            .build()
    );
}

pub fn new_contract() -> VecrowEscrow {
    set_context(CONTRACT, T0_MS);
    VecrowEscrow::new(account(USDC), account(TREASURY), FEE_BPS, WINDOW_SEC)
}

pub fn share(id: &str, bps: u16) -> SplitShare {
    SplitShare {
        account_id: account(id),
        bps,
    }
}

/// The default split used by most tests: 70/30 between the freelancer and a
/// team member.
pub fn split_70_30() -> Vec<SplitShare> {
    vec![share(FREELANCER, 7_000), share(MEMBER, 3_000)]
}

/// Creates an escrow as the freelancer with 1,000 and 500 USDC milestones.
pub fn create_default(contract: &mut VecrowEscrow) -> String {
    set_context(FREELANCER, T0_MS);
    contract.create_escrow(vec![U128(usdc(1_000)), U128(usdc(500))], split_70_30())
}

pub fn view(contract: &VecrowEscrow, escrow_id: &str) -> EscrowView {
    contract
        .get_escrow(escrow_id.to_string())
        .expect("escrow exists")
}
