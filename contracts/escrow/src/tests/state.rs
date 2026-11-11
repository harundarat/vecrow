//! The storage layout documented in `docs/architecture.md` §5.2.

use super::*;
use crate::signature::ApproverKey;

#[test]
fn escrows_and_used_nonces_use_the_documented_keys() {
    let mut contract = new_contract();
    let id = create_default(&mut contract);
    let wallet = Wallet::near();
    bind(&mut contract, &id, &wallet, 7);
    contract.escrows.flush();

    // Escrow 1: key 0x00 ‖ u64 little-endian ID, value = borsh(Escrow).
    let mut key = vec![0x00];
    key.extend(1u64.to_le_bytes());
    let value = near_sdk::env::storage_read(&key).expect("escrow record");
    let escrow: Escrow = near_sdk::borsh::from_slice(&value).unwrap();
    assert_eq!(escrow.freelancer, account(FREELANCER));
    assert_eq!(escrow.status, EscrowStatus::ApproverBound);
    // A two-milestone, two-recipient escrow with a NEP-413 approver, not yet
    // funded, is 9 + 233 bytes, plus 40 bytes of record overhead.
    assert_eq!(key.len() + value.len(), 242);

    // Used nonce: key 0x01 ‖ sha256(borsh(approver key) ‖ nonce), empty value.
    let approver = ApproverKey::parse(&wallet.approver()).unwrap();
    let mut preimage = near_sdk::borsh::to_vec(&approver).unwrap();
    preimage.extend([7u8; 32]);
    let mut key = vec![0x01];
    key.extend(near_sdk::env::sha256_array(&preimage));
    assert_eq!(near_sdk::env::storage_read(&key), Some(vec![]));
}
