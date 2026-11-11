//! The storage layout documented in `docs/architecture.md` §5.2.

use super::*;
use crate::signature::ApproverKey;

#[test]
fn escrows_and_used_nonces_use_the_documented_keys() {
    let mut contract = new_contract();
    let wallet = Wallet::near();
    let id = funded_escrow(&mut contract, &wallet);
    submit(&mut contract, &id, 0, T0_MS);
    approve(&mut contract, &id, 0, &wallet, 7, T0_MS);
    contract.escrows.flush();

    // Escrow "e1": key 0x00 ‖ borsh(String), value = borsh(Escrow).
    let key = [vec![0x00], near_sdk::borsh::to_vec("e1").unwrap()].concat();
    assert_eq!(key, vec![0x00, 2, 0, 0, 0, b'e', b'1']);
    let value = near_sdk::env::storage_read(&key).expect("escrow record");
    let escrow: Escrow = near_sdk::borsh::from_slice(&value).unwrap();
    assert_eq!(escrow.freelancer, account(FREELANCER));
    assert_eq!(escrow.status, EscrowStatus::Funded);
    // Two milestones (one submitted), two recipients and a NEP-413 approver:
    // 7 + 247 bytes, plus 40 bytes of record overhead.
    assert_eq!(key.len() + value.len(), 254);

    // Used nonce: key 0x01 ‖ sha256(borsh(approver key) ‖ nonce), empty value.
    let approver = ApproverKey::parse(&wallet.approver()).unwrap();
    let mut preimage = near_sdk::borsh::to_vec(&approver).unwrap();
    preimage.extend([7u8; 32]);
    let key = [vec![0x01], near_sdk::env::sha256_array(&preimage).to_vec()].concat();
    assert_eq!(near_sdk::env::storage_read(&key), Some(vec![]));
}
