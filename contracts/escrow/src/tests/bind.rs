//! R-04: bind the approver and the refund account, with nonce and deadline
//! checks shared by all signed actions.

use super::*;

#[test]
fn each_wallet_scheme_binds_as_approver() {
    for wallet in Wallet::all() {
        let mut contract = new_contract();
        let id = create_default(&mut contract);
        bind(&mut contract, &id, &wallet, 1);

        let escrow = view(&contract, &id);
        assert_eq!(escrow.status, EscrowStatus::ApproverBound);
        assert_eq!(escrow.approver, Some(wallet.approver()));
        assert_eq!(escrow.refund_account, Some(account(REFUND)));
    }
}

#[test]
fn bind_emits_event() {
    let mut contract = new_contract();
    let id = create_default(&mut contract);
    let wallet = Wallet::evm();
    bind(&mut contract, &id, &wallet, 1);
    let address = match wallet.approver() {
        Approver::Eip191 { address } => address,
        _ => unreachable!(),
    };
    assert_eq!(
        near_sdk::test_utils::get_logs().last().unwrap(),
        &format!(
            r#"EVENT_JSON:{{"standard":"vecrow","version":"1.0.0","event":"approver_bound","data":{{"escrow_id":"1","approver":{{"scheme":"eip191","address":"{address}"}},"refund_account":"refund.test.near"}}}}"#
        )
    );
}

#[test]
#[should_panic(expected = "approver is already bound")]
fn second_bind_is_rejected() {
    let mut contract = new_contract();
    let id = create_default(&mut contract);
    bind(&mut contract, &id, &Wallet::near(), 1);
    bind(&mut contract, &id, &Wallet::evm(), 2);
}

#[test]
#[should_panic(expected = "approver is already bound")]
fn same_wallet_cannot_bind_twice() {
    let mut contract = new_contract();
    let id = create_default(&mut contract);
    bind(&mut contract, &id, &Wallet::solana(), 1);
    bind(&mut contract, &id, &Wallet::solana(), 2);
}

#[test]
#[should_panic(expected = "escrow is cancelled")]
fn cancelled_escrow_cannot_be_bound() {
    let mut contract = new_contract();
    let id = create_default(&mut contract);
    set_context(FREELANCER, T0_MS);
    contract.cancel(id.clone());
    bind(&mut contract, &id, &Wallet::near(), 1);
}

#[test]
fn freelancer_can_cancel_after_bind() {
    let mut contract = new_contract();
    let id = create_default(&mut contract);
    bind(&mut contract, &id, &Wallet::near(), 1);
    set_context(FREELANCER, T0_MS);
    contract.cancel(id.clone());
    assert_eq!(view(&contract, &id).status, EscrowStatus::Cancelled);
}

#[test]
#[should_panic(expected = "approver must not be the freelancer or a split recipient")]
fn approver_equal_to_freelancer_is_rejected() {
    let mut contract = new_contract();
    let id = create_default(&mut contract);
    let wallet = Wallet::Near(NearWallet::new(FREELANCER, 21));
    bind(&mut contract, &id, &wallet, 1);
}

#[test]
#[should_panic(expected = "approver must not be the freelancer or a split recipient")]
fn approver_equal_to_split_recipient_is_rejected() {
    let mut contract = new_contract();
    let id = create_default(&mut contract);
    let wallet = Wallet::Near(NearWallet::new(MEMBER, 21));
    bind(&mut contract, &id, &wallet, 1);
}

/// An approver is also rejected when the freelancer or a recipient is the
/// implicit or eth-implicit NEAR account of the approver's key.
#[test]
fn approver_matching_an_implicit_account_is_rejected() {
    let near = NearWallet::new("someone.test.near", 31);
    let solana = SolanaWallet::new(32);
    let evm = EvmWallet::new(33);
    let cases = [
        (near.implicit_account_id(), Wallet::Near(near)),
        (solana.implicit_account_id(), Wallet::Solana(solana)),
        (evm.address(), Wallet::Evm(evm)),
    ];
    for (implicit, wallet) in cases {
        for as_freelancer in [true, false] {
            let mut contract = new_contract();
            let (caller, split) = if as_freelancer {
                (implicit.clone(), vec![share(&implicit, 10_000)])
            } else {
                (
                    FREELANCER.to_string(),
                    vec![share(FREELANCER, 5_000), share(&implicit, 5_000)],
                )
            };
            set_context(&caller, T0_MS);
            let id = contract.create_escrow(vec![U128(usdc(10))], split);
            let args = sign_bind(&wallet, &id, REFUND, 1);
            let result = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
                bind_with(&mut contract, args)
            }));
            assert!(result.is_err(), "{implicit} as freelancer: {as_freelancer}");
            assert_eq!(view(&contract, &id).status, EscrowStatus::Created);
        }
    }
}

#[test]
#[should_panic(expected = "refund_account must not be the freelancer or a split recipient")]
fn refund_account_equal_to_freelancer_is_rejected() {
    let mut contract = new_contract();
    let id = create_default(&mut contract);
    bind_with(
        &mut contract,
        sign_bind(&Wallet::near(), &id, FREELANCER, 1),
    );
}

#[test]
#[should_panic(expected = "refund_account must not be the freelancer or a split recipient")]
fn refund_account_equal_to_split_recipient_is_rejected() {
    let mut contract = new_contract();
    let id = create_default(&mut contract);
    bind_with(&mut contract, sign_bind(&Wallet::evm(), &id, MEMBER, 1));
}

#[test]
#[should_panic(expected = "refund_account must not be the escrow contract")]
fn refund_account_equal_to_contract_is_rejected() {
    let mut contract = new_contract();
    let id = create_default(&mut contract);
    bind_with(&mut contract, sign_bind(&Wallet::evm(), &id, CONTRACT, 1));
}

#[test]
fn refund_account_may_be_the_approver_account() {
    let mut contract = new_contract();
    let id = create_default(&mut contract);
    bind_with(&mut contract, sign_bind(&Wallet::near(), &id, CLIENT, 1));
    assert_eq!(view(&contract, &id).refund_account, Some(account(CLIENT)));
}

#[test]
#[should_panic(expected = "invalid signature")]
fn bind_signed_by_another_wallet_is_rejected() {
    let mut contract = new_contract();
    let id = create_default(&mut contract);
    let mut args = sign_bind(&Wallet::Evm(EvmWallet::new(77)), &id, REFUND, 1);
    args.approver = Wallet::evm().approver();
    bind_with(&mut contract, args);
}

#[test]
#[should_panic(expected = "invalid signature")]
fn bind_with_a_different_refund_account_than_signed_is_rejected() {
    let mut contract = new_contract();
    let id = create_default(&mut contract);
    let mut args = sign_bind(&Wallet::solana(), &id, REFUND, 1);
    args.refund_account = account(STRANGER);
    bind_with(&mut contract, args);
}

#[test]
#[should_panic(expected = "invalid signature")]
fn bind_signed_for_another_escrow_is_rejected() {
    let mut contract = new_contract();
    create_default(&mut contract);
    let id = create_default(&mut contract);
    let mut args = sign_bind(&Wallet::near(), "1", REFUND, 1);
    args.escrow_id = id;
    bind_with(&mut contract, args);
}

#[test]
#[should_panic(expected = "invalid signature")]
fn bind_signed_for_another_contract_is_rejected() {
    let mut contract = new_contract();
    let id = create_default(&mut contract);
    let wallet = EvmWallet::new(12);
    let nonce = wallet_fixtures::nonce(1);
    let message = wallet_fixtures::bind_message(
        "other-escrow.test.near",
        &id,
        &wallet.approver_label(),
        REFUND,
        &nonce,
        DEADLINE_MS,
    );
    let mut args = sign_bind(&Wallet::evm(), &id, REFUND, 1);
    args.signature = wallet.personal_sign(&message);
    bind_with(&mut contract, args);
}

#[test]
#[should_panic(expected = "invalid signature")]
fn nep413_bind_signed_for_another_recipient_is_rejected() {
    let mut contract = new_contract();
    let id = create_default(&mut contract);
    let wallet = NearWallet::new(CLIENT, 11);
    let nonce = wallet_fixtures::nonce(1);
    let message = wallet_fixtures::bind_message(
        CONTRACT,
        &id,
        &wallet.approver_label(),
        REFUND,
        &nonce,
        DEADLINE_MS,
    );
    let mut args = sign_bind(&Wallet::near(), &id, REFUND, 1);
    args.signature = wallet.sign_message(&message, &nonce, "other-escrow.test.near");
    bind_with(&mut contract, args);
}

#[test]
#[should_panic(expected = "signature expired")]
fn expired_bind_is_rejected() {
    let mut contract = new_contract();
    let id = create_default(&mut contract);
    let args = sign_bind(&Wallet::near(), &id, REFUND, 1);
    set_context(RELAYER, DEADLINE_MS + 1);
    contract.bind_approver(
        args.escrow_id,
        args.approver,
        args.refund_account,
        args.nonce,
        args.deadline_ms,
        args.signature,
    );
}

#[test]
fn bind_at_the_deadline_is_accepted() {
    let mut contract = new_contract();
    let id = create_default(&mut contract);
    let args = sign_bind(&Wallet::near(), &id, REFUND, 1);
    set_context(RELAYER, DEADLINE_MS);
    contract.bind_approver(
        args.escrow_id,
        args.approver,
        args.refund_account,
        args.nonce,
        args.deadline_ms,
        args.signature,
    );
    assert_eq!(view(&contract, &id).status, EscrowStatus::ApproverBound);
}

#[test]
#[should_panic(expected = "nonce already used")]
fn reused_nonce_is_rejected_across_escrows() {
    let mut contract = new_contract();
    let first = create_default(&mut contract);
    let second = create_default(&mut contract);
    bind(&mut contract, &first, &Wallet::evm(), 9);
    bind(&mut contract, &second, &Wallet::evm(), 9);
}

#[test]
fn nonces_are_tracked_per_approver() {
    let mut contract = new_contract();
    let first = create_default(&mut contract);
    let second = create_default(&mut contract);
    bind(&mut contract, &first, &Wallet::evm(), 9);
    bind(&mut contract, &second, &Wallet::solana(), 9);
    assert_eq!(view(&contract, &second).status, EscrowStatus::ApproverBound);
}

#[test]
#[should_panic(expected = "nonce must be 64 lowercase hex characters")]
fn uppercase_nonce_is_rejected() {
    let mut contract = new_contract();
    let id = create_default(&mut contract);
    let mut args = sign_bind(&Wallet::near(), &id, REFUND, 0xab);
    args.nonce = args.nonce.to_uppercase();
    bind_with(&mut contract, args);
}

#[test]
#[should_panic(expected = "unknown escrow")]
fn bind_to_unknown_escrow_is_rejected() {
    let mut contract = new_contract();
    bind(&mut contract, "5", &Wallet::near(), 1);
}

#[test]
fn failed_bind_does_not_consume_the_nonce() {
    let mut contract = new_contract();
    let id = create_default(&mut contract);
    let mut bad = sign_bind(&Wallet::near(), &id, REFUND, 1);
    bad.refund_account = account(STRANGER);
    let result = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
        bind_with(&mut contract, bad)
    }));
    assert!(result.is_err());
    // A failed call is reverted on chain; here the panic happened before the
    // nonce was recorded, so the same nonce still works.
    bind(&mut contract, &id, &Wallet::near(), 1);
    assert_eq!(view(&contract, &id).status, EscrowStatus::ApproverBound);
}
