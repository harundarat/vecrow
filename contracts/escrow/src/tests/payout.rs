//! R-20: the `on_payout` callback records failed transfers as pending.
//! That only the contract itself may call it (`#[private]`, enforced by the
//! generated wasm entry point) is checked in the sandbox tests.

use super::*;

#[test]
fn successful_transfer_leaves_nothing_pending() {
    let mut contract = new_contract();
    let id = funded_escrow(&mut contract, &Wallet::near());
    assert!(payout_callback(&mut contract, &id, MEMBER, 10, true));
    assert!(view(&contract, &id).pending_payouts.is_empty());
    assert_eq!(
        near_sdk::test_utils::get_logs(),
        vec![
            r#"EVENT_JSON:{"standard":"vecrow","version":"1.0.0","event":"payout","data":{"escrow_id":"e1","account_id":"member.test.near","amount":"10","paid":true}}"#
        ]
    );
}

#[test]
fn failed_transfers_are_recorded_per_account() {
    let mut contract = new_contract();
    let id = funded_escrow(&mut contract, &Wallet::near());
    assert!(!payout_callback(&mut contract, &id, MEMBER, 10, false));
    assert!(!payout_callback(&mut contract, &id, TREASURY, 3, false));
    assert!(!payout_callback(&mut contract, &id, MEMBER, 5, false));

    assert_eq!(
        view(&contract, &id).pending_payouts,
        vec![
            PendingPayout {
                account_id: account(MEMBER),
                amount: U128(15),
            },
            PendingPayout {
                account_id: account(TREASURY),
                amount: U128(3),
            },
        ]
    );
    assert_eq!(
        near_sdk::test_utils::get_logs(),
        vec![
            r#"EVENT_JSON:{"standard":"vecrow","version":"1.0.0","event":"payout","data":{"escrow_id":"e1","account_id":"member.test.near","amount":"5","paid":false}}"#
        ]
    );
}
