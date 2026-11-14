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

pub const BPS_DENOMINATOR: u16 = 10_000;
pub const MAX_MILESTONES: usize = 20;
pub const MAX_REVISIONS: u8 = 2;
pub const MAX_ESCROW_ID_LEN: usize = 64;

#[near(serializers = [borsh, json])]
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Config {
    pub usdc_contract_id: AccountId,
    pub treasury_account: AccountId,
    pub fee_bps: u16,
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
    escrows: LookupMap<String, Escrow>,
    used_nonces: LookupSet<[u8; 32]>,
}

#[near]
impl VecrowEscrow {
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

    pub fn get_escrow(&self, escrow_id: String) -> Option<EscrowView> {
        self.escrows
            .get(&escrow_id)
            .map(|e| EscrowView::new(&escrow_id, e))
    }
}

impl VecrowEscrow {
    fn load_escrow(&self, escrow_id: &str) -> Escrow {
        self.escrows
            .get(escrow_id)
            .cloned()
            .unwrap_or_else(|| env::panic_str("unknown escrow"))
    }

    fn save_escrow(&mut self, escrow_id: &str, escrow: Escrow) {
        self.escrows.insert(escrow_id.to_string(), escrow);
    }

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

fn require_freelancer(escrow: &Escrow) {
    require!(
        env::predecessor_account_id() == escrow.freelancer,
        "only the freelancer can call this method"
    );
}

// The ID appears verbatim in signed messages and in the funding `msg`, so it
// must have a single spelling.
pub fn is_valid_escrow_id(escrow_id: &str) -> bool {
    !escrow_id.is_empty()
        && escrow_id.len() <= MAX_ESCROW_ID_LEN
        && escrow_id
            .bytes()
            .all(|b| matches!(b, b'a'..=b'z' | b'0'..=b'9' | b'-' | b'_'))
}
