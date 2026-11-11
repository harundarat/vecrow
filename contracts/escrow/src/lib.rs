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
mod funding;
mod milestones;
pub mod payout;
mod refund;
pub mod signature;
pub mod split;
mod types;

#[cfg(test)]
mod tests;

pub use signature::{Approver, EscrowTerms, FreelancerSignature};
pub use split::MAX_SPLIT_RECIPIENTS;
pub use types::*;

use near_sdk::store::{LookupMap, LookupSet};
use near_sdk::{AccountId, BorshStorageKey, PanicOnDefault, borsh, env, near, require};
use signature::ApproverKey;

/// Basis points in 100%.
pub const BPS_DENOMINATOR: u16 = 10_000;
/// Upper bound on milestones per escrow, to bound storage and gas.
pub const MAX_MILESTONES: usize = 20;
/// Revision requests allowed per milestone (R-17, D-03).
pub const MAX_REVISIONS: u8 = 2;
/// Longest escrow ID accepted by `create_escrow`.
pub const MAX_ESCROW_ID_LEN: usize = 64;

/// Deploy-time configuration (architecture §5.2). There is no setter: the
/// values change only by redeploying the contract. `create_escrow` requires
/// the signed `fee_bps` to match, and copies it and `review_window_sec` into
/// the escrow (D-15).
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
    UsedNonces,
}

#[near(contract_state)]
#[derive(PanicOnDefault)]
pub struct VecrowEscrow {
    config: Config,
    /// Escrows by the ID the backend chose when the escrow was proposed.
    escrows: LookupMap<String, Escrow>,
    /// `sha256(borsh(approver key) ‖ nonce)` of every accepted signed action,
    /// so each nonce is used at most once per approver.
    used_nonces: LookupSet<[u8; 32]>,
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
            escrows: LookupMap::new(StorageKey::Escrows),
            used_nonces: LookupSet::new(StorageKey::UsedNonces),
        }
    }

    /// Returns an on-chain escrow with its signed terms and milestones, or
    /// `None` for an unknown ID (including an escrow not on-chain yet).
    pub fn get_escrow(&self, escrow_id: String) -> Option<EscrowView> {
        self.escrows
            .get(&escrow_id)
            .map(|e| EscrowView::new(&escrow_id, e))
    }
}

impl VecrowEscrow {
    /// Loads an escrow by ID, or panics.
    fn load_escrow(&self, escrow_id: &str) -> Escrow {
        self.escrows
            .get(escrow_id)
            .cloned()
            .unwrap_or_else(|| env::panic_str("unknown escrow"))
    }

    fn save_escrow(&mut self, escrow_id: &str, escrow: Escrow) {
        self.escrows.insert(escrow_id.to_string(), escrow);
    }

    /// Accepts a signed action from `approver`, or panics: the deadline has
    /// not passed, the signature over `message` is valid for the approver's
    /// scheme, and the nonce has not been used by this approver before. The
    /// nonce is then recorded. The caller (usually the relayer) is irrelevant:
    /// approver rights come only from the signature (D-02).
    fn accept_signed_action(
        &mut self,
        approver: &ApproverKey,
        message: &str,
        nonce: &[u8; 32],
        deadline_ms: u64,
        signature: &str,
    ) {
        require!(
            env::block_timestamp_ms() <= deadline_ms,
            "signature expired"
        );
        let recipient = env::current_account_id();
        let valid = approver
            .verify(message, nonce, recipient.as_str(), signature)
            .unwrap_or_else(|err| env::panic_str(err));
        require!(valid, "invalid signature");
        let mut preimage = borsh::to_vec(approver).expect("approver serializes");
        preimage.extend_from_slice(nonce);
        require!(
            self.used_nonces.insert(env::sha256_array(&preimage)),
            "nonce already used"
        );
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

/// Whether `escrow_id` is a valid escrow ID: 1 to 64 characters from
/// `a-z`, `0-9`, `-` and `_`, e.g. a lowercase UUID chosen by the backend.
/// The ID is written in signed messages and sent as the funding `msg`, so it
/// has no whitespace, separators or uppercase variants.
pub fn is_valid_escrow_id(escrow_id: &str) -> bool {
    !escrow_id.is_empty()
        && escrow_id.len() <= MAX_ESCROW_ID_LEN
        && escrow_id
            .bytes()
            .all(|b| matches!(b, b'a'..=b'z' | b'0'..=b'9' | b'-' | b'_'))
}
