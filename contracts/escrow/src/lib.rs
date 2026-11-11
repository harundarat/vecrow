//! Vecrow escrow contract.
//!
//! Holds USDC per escrow and releases it per milestone, minus a platform fee,
//! split between team accounts by basis points. Requirements are in
//! `docs/spec.md` (R-01 – R-07, R-17 – R-20); the method list, state layout and
//! signed-payload format are in `docs/architecture.md` §5.
//!
//! Users never attach a deposit: storage and the 1 yoctoNEAR that each
//! `ft_transfer` requires are paid from the contract's own NEAR balance.

mod create;
mod events;
mod split;
mod types;

#[cfg(test)]
mod tests;

pub use split::MAX_SPLIT_RECIPIENTS;
pub use types::*;

use near_sdk::store::LookupMap;
use near_sdk::{AccountId, BorshStorageKey, PanicOnDefault, env, near, require};

/// Basis points in 100%.
pub const BPS_DENOMINATOR: u16 = 10_000;
/// Upper bound on milestones per escrow, to bound storage and gas.
pub const MAX_MILESTONES: usize = 20;

/// Deploy-time configuration (architecture §5.2). There is no setter: the
/// values change only by redeploying the contract, and `fee_bps` and
/// `review_window_sec` are copied into each escrow at creation (D-14).
#[near(serializers = [borsh, json])]
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Config {
    /// The NEP-141 token accepted for funding and used for every payout.
    pub usdc_contract_id: AccountId,
    /// Receives the platform fee of every release.
    pub treasury_account: AccountId,
    /// Platform fee for new escrows, in basis points (50 = 0.5%).
    pub fee_bps: u16,
    /// Review window for new escrows, in seconds (14 days in production).
    pub review_window_sec: u64,
}

#[derive(BorshStorageKey)]
#[near]
enum StorageKey {
    Escrows,
}

#[near(contract_state)]
#[derive(PanicOnDefault)]
pub struct VecrowEscrow {
    config: Config,
    /// ID of the next escrow; IDs start at 1.
    next_escrow_id: u64,
    escrows: LookupMap<u64, Escrow>,
}

#[near]
impl VecrowEscrow {
    /// Initializes the contract. `fee_bps` is 50 (0.5%) in production;
    /// `review_window_sec` is 1,209,600 (14 days) and may be shortened for a
    /// demo deployment.
    #[init]
    pub fn new(
        usdc_contract_id: AccountId,
        treasury_account: AccountId,
        fee_bps: u16,
        review_window_sec: u64,
    ) -> Self {
        require!(fee_bps <= BPS_DENOMINATOR, "fee_bps must be at most 10000");
        require!(review_window_sec > 0, "review_window_sec must be positive");
        require!(
            treasury_account != env::current_account_id(),
            "treasury_account must not be the escrow contract"
        );
        Self {
            config: Config {
                usdc_contract_id,
                treasury_account,
                fee_bps,
                review_window_sec,
            },
            next_escrow_id: 1,
            escrows: LookupMap::new(StorageKey::Escrows),
        }
    }

    /// Returns an escrow with its milestones, or `None` for an unknown ID.
    pub fn get_escrow(&self, escrow_id: String) -> Option<EscrowView> {
        let id = parse_escrow_id(&escrow_id)?;
        self.escrows.get(&id).map(|e| EscrowView::new(id, e))
    }
}

impl VecrowEscrow {
    /// Loads an escrow by its string ID, or panics.
    fn load_escrow(&self, escrow_id: &str) -> (u64, Escrow) {
        let id = parse_escrow_id(escrow_id).unwrap_or_else(|| env::panic_str("unknown escrow"));
        match self.escrows.get(&id) {
            Some(escrow) => (id, escrow.clone()),
            None => env::panic_str("unknown escrow"),
        }
    }

    fn save_escrow(&mut self, id: u64, escrow: Escrow) {
        self.escrows.insert(id, escrow);
    }
}

/// Requires the caller to be the escrow's freelancer. Meta transactions
/// (NEP-366) keep the freelancer as the predecessor, so the relayer cannot
/// act as the freelancer.
fn require_freelancer(escrow: &Escrow) {
    require!(
        env::predecessor_account_id() == escrow.freelancer,
        "only the freelancer can call this method"
    );
}

/// Parses an escrow ID in its canonical form: the decimal `u64` without sign
/// or leading zeros, as returned by `create_escrow`. Anything else is unknown,
/// so an ID in a signed message or a funding `msg` has exactly one spelling.
pub fn parse_escrow_id(escrow_id: &str) -> Option<u64> {
    let canonical = !escrow_id.is_empty()
        && escrow_id.len() <= 20
        && escrow_id.bytes().all(|b| b.is_ascii_digit())
        && !escrow_id.starts_with('0');
    if canonical {
        escrow_id.parse().ok()
    } else {
        None
    }
}
