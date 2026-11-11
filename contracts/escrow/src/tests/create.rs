//! R-01 (create on-chain and cancel), with the R-03 split rules, the R-04
//! approver and refund-account rules, and the R-19 fee check of
//! `create_escrow`.

use super::*;

/// Signs the default terms (as modified by `edit`) with the freelancer and
/// `client` wallets, then calls `create_escrow`.
fn create_edited(
    contract: &mut VecrowEscrow,
    client: &Wallet,
    edit: impl FnOnce(&mut EscrowTerms),
) {
    let mut terms = default_terms("e1", client);
    edit(&mut terms);
    create_with(contract, sign_terms(terms, &freelancer_wallet(), client));
}

#[test]
fn create_escrow_stores_the_signed_terms() {
    for wallet in Wallet::all() {
        let mut contract = new_contract();
        let id = create_escrow_with(&mut contract, &wallet);
        assert_eq!(id, "e1");

        let escrow = view(&contract, &id);
        assert_eq!(escrow.escrow_id, "e1");
        assert_eq!(escrow.freelancer, account(FREELANCER));
        assert_eq!(escrow.status, EscrowStatus::Created);
        assert_eq!(escrow.split, split_70_30());
        assert_eq!(escrow.approver, wallet.approver());
        assert_eq!(escrow.refund_account, account(REFUND));
        assert_eq!(escrow.fee_bps, FEE_BPS);
        assert_eq!(escrow.review_window_sec, WINDOW_SEC);
        assert_eq!(escrow.total, U128(usdc(1_500)));
        assert_eq!(escrow.balance, U128(0));
        assert_eq!(escrow.created_at_ms, T0_MS);
        assert_eq!(escrow.milestones.len(), 2);
        for (m, amount) in escrow.milestones.iter().zip([usdc(1_000), usdc(500)]) {
            assert_eq!(m.amount, U128(amount));
            assert_eq!(m.status, MilestoneStatus::Pending);
            assert_eq!(m.submitted_at_ms, None);
            assert_eq!(m.revision_count, 0);
        }
    }
}

#[test]
fn create_escrow_emits_event() {
    let mut contract = new_contract();
    let wallet = Wallet::evm();
    create_escrow_with(&mut contract, &wallet);
    let Approver::Eip191 { address } = wallet.approver() else {
        unreachable!()
    };
    assert_eq!(
        near_sdk::test_utils::get_logs().last().unwrap(),
        &format!(
            r#"EVENT_JSON:{{"standard":"vecrow","version":"1.0.0","event":"escrow_created","data":{{"escrow_id":"e1","freelancer":"freelancer.test.near","approver":{{"scheme":"eip191","address":"{address}"}},"refund_account":"refund.test.near","total":"1500000000"}}}}"#
        )
    );
}

#[test]
fn statuses_serialize_as_pascal_case() {
    let json = near_sdk::serde_json::to_string(&[
        EscrowStatus::Created,
        EscrowStatus::Funded,
        EscrowStatus::Completed,
        EscrowStatus::Cancelled,
    ])
    .unwrap();
    assert_eq!(json, r#"["Created","Funded","Completed","Cancelled"]"#);
    let json = near_sdk::serde_json::to_string(&[
        MilestoneStatus::Pending,
        MilestoneStatus::Submitted,
        MilestoneStatus::RevisionRequested,
        MilestoneStatus::Released,
        MilestoneStatus::Refunded,
    ])
    .unwrap();
    assert_eq!(
        json,
        r#"["Pending","Submitted","RevisionRequested","Released","Refunded"]"#
    );
}

#[test]
fn escrow_ids_have_one_spelling() {
    for id in [
        "e1",
        "0",
        "a-b_c",
        "6f1c2a5e-8d3b-4c1e-9f00-1234567890ab",
        &"x".repeat(64),
    ] {
        assert!(is_valid_escrow_id(id), "{id}");
    }
    for id in ["", "E1", "e 1", "e1\n", "e:1", "é", &"x".repeat(65)] {
        assert!(!is_valid_escrow_id(id), "{id:?}");
    }
    let mut contract = new_contract();
    create_default(&mut contract);
    assert!(contract.get_escrow("e2".to_string()).is_none());
    assert!(contract.get_escrow("E1".to_string()).is_none());
}

#[test]
#[should_panic(expected = "invalid escrow ID")]
fn malformed_escrow_id_is_rejected() {
    let mut contract = new_contract();
    create_edited(&mut contract, &Wallet::near(), |t| {
        t.escrow_id = "E1".into()
    });
}

#[test]
#[should_panic(expected = "escrow ID already exists")]
fn existing_escrow_id_is_rejected() {
    let mut contract = new_contract();
    create_edited(&mut contract, &Wallet::near(), |_| {});
    // Even the exact same signed terms cannot be put on-chain twice.
    create_edited(&mut contract, &Wallet::near(), |_| {});
}

#[test]
#[should_panic(expected = "escrow ID already exists")]
fn cancelled_escrow_id_cannot_be_reused() {
    let mut contract = new_contract();
    let id = create_default(&mut contract);
    set_context(FREELANCER, T0_MS);
    contract.cancel(id, None, None, None);
    create_edited(&mut contract, &Wallet::evm(), |_| {});
}

#[test]
#[should_panic(expected = "terms expired")]
fn terms_past_their_deadline_are_rejected() {
    let mut contract = new_contract();
    let wallet = Wallet::solana();
    let signed = sign_terms(default_terms("e1", &wallet), &freelancer_wallet(), &wallet);
    set_context(RELAYER, DEADLINE_MS + 1);
    contract.create_escrow(
        signed.terms,
        signed.freelancer_signature,
        signed.client_signature,
    );
}

#[test]
fn terms_at_their_deadline_are_accepted() {
    let mut contract = new_contract();
    let wallet = Wallet::near();
    let signed = sign_terms(default_terms("e1", &wallet), &freelancer_wallet(), &wallet);
    set_context(RELAYER, DEADLINE_MS);
    contract.create_escrow(
        signed.terms,
        signed.freelancer_signature,
        signed.client_signature,
    );
    assert_eq!(view(&contract, "e1").status, EscrowStatus::Created);
}

#[test]
#[should_panic(expected = "fee_bps does not match the current configuration")]
fn terms_with_another_fee_are_rejected() {
    // R-19: terms signed with a fee_bps that no longer matches the
    // configuration (e.g. signed before a redeploy) are rejected.
    let mut contract = new_contract();
    create_edited(&mut contract, &Wallet::near(), |t| t.fee_bps = 40);
}

#[test]
#[should_panic(expected = "invalid freelancer signature")]
fn terms_signed_by_another_freelancer_key_are_rejected() {
    let mut contract = new_contract();
    let wallet = Wallet::evm();
    let mut signed = sign_terms(default_terms("e1", &wallet), &freelancer_wallet(), &wallet);
    let other = sign_terms(
        default_terms("e1", &wallet),
        &NearWallet::new(FREELANCER, 66),
        &wallet,
    );
    signed.freelancer_signature.signature = other.freelancer_signature.signature;
    create_with(&mut contract, signed);
}

#[test]
#[should_panic(expected = "invalid client signature")]
fn terms_not_signed_by_the_approver_wallet_are_rejected() {
    // R-04: the client's signature must come from the approver wallet.
    let mut contract = new_contract();
    let approver = Wallet::evm();
    let other = Wallet::Evm(EvmWallet::new(77));
    let mut signed = sign_terms(
        default_terms("e1", &approver),
        &freelancer_wallet(),
        &approver,
    );
    signed.client_signature =
        sign_terms(default_terms("e1", &approver), &freelancer_wallet(), &other).client_signature;
    create_with(&mut contract, signed);
}

#[test]
fn missing_signatures_are_rejected() {
    for wallet in Wallet::all() {
        let mut contract = new_contract();
        let mut signed = sign_terms(default_terms("e1", &wallet), &freelancer_wallet(), &wallet);
        signed.client_signature = String::new();
        let result = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
            create_with(&mut contract, signed)
        }));
        assert!(result.is_err());

        let mut signed = sign_terms(default_terms("e1", &wallet), &freelancer_wallet(), &wallet);
        signed.freelancer_signature.signature = String::new();
        let result = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
            create_with(&mut contract, signed)
        }));
        assert!(result.is_err());
        assert!(contract.get_escrow("e1".to_string()).is_none());
    }
}

/// R-01: terms that differ in any field from what both parties signed are
/// rejected.
#[test]
fn terms_that_differ_from_what_was_signed_are_rejected() {
    type Edit = Box<dyn Fn(&mut EscrowTerms)>;
    let edits: Vec<(&str, Edit)> = vec![
        ("escrow ID", Box::new(|t| t.escrow_id = "e2".into())),
        ("freelancer", Box::new(|t| t.freelancer = account(MEMBER2))),
        (
            "milestone amount",
            Box::new(|t| t.milestones[0] = U128(usdc(1_001))),
        ),
        ("milestone order", Box::new(|t| t.milestones.reverse())),
        ("extra milestone", Box::new(|t| t.milestones.push(U128(1)))),
        (
            "split bps",
            Box::new(|t| t.split = vec![share(FREELANCER, 6_000), share(MEMBER, 4_000)]),
        ),
        (
            "split recipient",
            Box::new(|t| t.split[1].account_id = account(MEMBER2)),
        ),
        ("split order", Box::new(|t| t.split.reverse())),
        (
            "approver",
            Box::new(|t| t.approver = Wallet::Evm(EvmWallet::new(88)).approver()),
        ),
        (
            "refund account",
            Box::new(|t| t.refund_account = account(STRANGER)),
        ),
        ("deadline", Box::new(|t| t.deadline_ms += 1)),
    ];
    for wallet in Wallet::all() {
        for (field, edit) in &edits {
            let mut contract = new_contract();
            let mut signed =
                sign_terms(default_terms("e1", &wallet), &freelancer_wallet(), &wallet);
            edit(&mut signed.terms);
            let id = signed.terms.escrow_id.clone();
            let result = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
                create_with(&mut contract, signed)
            }));
            assert!(result.is_err(), "changed {field} was accepted");
            assert!(contract.get_escrow(id).is_none());
        }
    }
}

#[test]
#[should_panic(expected = "invalid freelancer signature")]
fn terms_signed_for_another_contract_are_rejected() {
    let mut contract = new_contract();
    let wallet = Wallet::evm();
    let terms = default_terms("e1", &wallet);
    let message = terms_message(&terms, &wallet.label()).replace(CONTRACT, "other.test.near");
    let nonce = wallet_fixtures::terms_nonce(&message);
    let freelancer = freelancer_wallet();
    create_with(
        &mut contract,
        SignedTerms {
            freelancer_signature: FreelancerSignature {
                public_key: freelancer.public_key(),
                signature: freelancer.sign_message(&message, &nonce, "other.test.near"),
            },
            client_signature: wallet.sign(&message, &nonce),
            terms,
        },
    );
}

#[test]
#[should_panic(expected = "split must total exactly 10000 bps")]
fn split_below_10000_bps_is_rejected() {
    let mut contract = new_contract();
    create_edited(&mut contract, &Wallet::near(), |t| {
        t.split = vec![share(FREELANCER, 7_000), share(MEMBER, 2_999)]
    });
}

#[test]
#[should_panic(expected = "split must total exactly 10000 bps")]
fn split_above_10000_bps_is_rejected() {
    let mut contract = new_contract();
    create_edited(&mut contract, &Wallet::near(), |t| {
        t.split = vec![share(FREELANCER, 7_000), share(MEMBER, 3_001)]
    });
}

#[test]
#[should_panic(expected = "split must have at least one recipient")]
fn empty_split_is_rejected() {
    let mut contract = new_contract();
    create_edited(&mut contract, &Wallet::near(), |t| t.split = vec![]);
}

#[test]
#[should_panic(expected = "split shares must be positive")]
fn zero_share_is_rejected() {
    let mut contract = new_contract();
    create_edited(&mut contract, &Wallet::near(), |t| {
        t.split = vec![share(FREELANCER, 10_000), share(MEMBER, 0)]
    });
}

#[test]
#[should_panic(expected = "split recipients must be distinct")]
fn duplicate_recipient_is_rejected() {
    let mut contract = new_contract();
    create_edited(&mut contract, &Wallet::near(), |t| {
        t.split = vec![share(MEMBER, 5_000), share(MEMBER, 5_000)]
    });
}

#[test]
#[should_panic(expected = "split has too many recipients")]
fn too_many_recipients_are_rejected() {
    let mut contract = new_contract();
    create_edited(&mut contract, &Wallet::near(), |t| {
        t.split = (0..=MAX_SPLIT_RECIPIENTS)
            .map(|i| share(&format!("m{i}.test.near"), 1))
            .collect()
    });
}

#[test]
#[should_panic(expected = "the escrow contract cannot be a split recipient")]
fn contract_as_recipient_is_rejected() {
    let mut contract = new_contract();
    create_edited(&mut contract, &Wallet::near(), |t| {
        t.split = vec![share(CONTRACT, 10_000)]
    });
}

#[test]
#[should_panic(expected = "at least one milestone is required")]
fn no_milestones_is_rejected() {
    let mut contract = new_contract();
    create_edited(&mut contract, &Wallet::near(), |t| t.milestones = vec![]);
}

#[test]
#[should_panic(expected = "milestone amounts must be positive")]
fn zero_milestone_is_rejected() {
    let mut contract = new_contract();
    create_edited(&mut contract, &Wallet::near(), |t| {
        t.milestones.push(U128(0))
    });
}

#[test]
#[should_panic(expected = "too many milestones")]
fn too_many_milestones_are_rejected() {
    let mut contract = new_contract();
    create_edited(&mut contract, &Wallet::near(), |t| {
        t.milestones = vec![U128(1); MAX_MILESTONES + 1]
    });
}

#[test]
#[should_panic(expected = "milestone total overflows")]
fn overflowing_total_is_rejected() {
    let mut contract = new_contract();
    create_edited(&mut contract, &Wallet::near(), |t| {
        t.milestones = vec![U128(u128::MAX), U128(1)]
    });
}

#[test]
#[should_panic(expected = "approver must not be the freelancer or a split recipient")]
fn approver_equal_to_freelancer_is_rejected() {
    let mut contract = new_contract();
    create_edited(
        &mut contract,
        &Wallet::Near(NearWallet::new(FREELANCER, 21)),
        |_| {},
    );
}

#[test]
#[should_panic(expected = "approver must not be the freelancer or a split recipient")]
fn approver_equal_to_split_recipient_is_rejected() {
    let mut contract = new_contract();
    create_edited(
        &mut contract,
        &Wallet::Near(NearWallet::new(MEMBER, 21)),
        |_| {},
    );
}

#[test]
fn approver_with_the_freelancer_signing_key_is_rejected() {
    // A NEAR or Solana approver holding the ed25519 key the freelancer signed
    // the terms with is the freelancer, whatever account name it gives.
    for same_key in [
        Wallet::Near(NearWallet::new(CLIENT, 5)),
        Wallet::Solana(SolanaWallet::new(5)),
    ] {
        let mut contract = new_contract();
        let signed = sign_terms(
            default_terms("e1", &same_key),
            &freelancer_wallet(),
            &same_key,
        );
        let result = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
            create_with(&mut contract, signed)
        }));
        assert!(result.is_err());
        assert!(contract.get_escrow("e1".to_string()).is_none());
    }
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
            let mut terms = default_terms("e1", &wallet);
            if as_freelancer {
                terms.split = vec![share(&implicit, 10_000)];
                terms.freelancer = account(&implicit);
            } else {
                terms.split = vec![share(FREELANCER, 5_000), share(&implicit, 5_000)];
            }
            let signed = sign_terms(terms, &freelancer_wallet(), &wallet);
            let result = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
                create_with(&mut contract, signed)
            }));
            assert!(result.is_err(), "{implicit} as freelancer: {as_freelancer}");
            assert!(contract.get_escrow("e1".to_string()).is_none());
        }
    }
}

#[test]
#[should_panic(expected = "an implicit freelancer account must sign with its own key")]
fn implicit_freelancer_must_sign_with_its_key() {
    let mut contract = new_contract();
    let implicit = NearWallet::new("x", 44).implicit_account_id();
    create_edited(&mut contract, &Wallet::evm(), |t| {
        t.freelancer = account(&implicit);
        t.split = vec![share(&implicit, 10_000)];
    });
}

#[test]
fn implicit_freelancer_signing_with_its_key_is_accepted() {
    let mut contract = new_contract();
    let key = NearWallet::new("x", 44);
    let freelancer = NearWallet::new(&key.implicit_account_id(), 44);
    let wallet = Wallet::evm();
    let mut terms = default_terms("e1", &wallet);
    terms.freelancer = account(freelancer.account_id());
    terms.split = vec![share(freelancer.account_id(), 10_000)];
    create_with(&mut contract, sign_terms(terms, &freelancer, &wallet));
    assert_eq!(view(&contract, "e1").status, EscrowStatus::Created);
}

#[test]
#[should_panic(expected = "refund_account must not be the freelancer or a split recipient")]
fn refund_account_equal_to_freelancer_is_rejected() {
    let mut contract = new_contract();
    create_edited(&mut contract, &Wallet::near(), |t| {
        t.refund_account = account(FREELANCER)
    });
}

#[test]
#[should_panic(expected = "refund_account must not be the freelancer or a split recipient")]
fn refund_account_equal_to_split_recipient_is_rejected() {
    let mut contract = new_contract();
    create_edited(&mut contract, &Wallet::evm(), |t| {
        t.refund_account = account(MEMBER)
    });
}

#[test]
#[should_panic(expected = "refund_account must not be the escrow contract")]
fn refund_account_equal_to_contract_is_rejected() {
    let mut contract = new_contract();
    create_edited(&mut contract, &Wallet::evm(), |t| {
        t.refund_account = account(CONTRACT)
    });
}

#[test]
fn refund_account_may_be_the_approver_account() {
    let mut contract = new_contract();
    create_edited(&mut contract, &Wallet::near(), |t| {
        t.refund_account = account(CLIENT)
    });
    assert_eq!(view(&contract, "e1").refund_account, account(CLIENT));
}

#[test]
fn freelancer_cancels_created_escrow() {
    let mut contract = new_contract();
    let id = create_default(&mut contract);
    set_context(FREELANCER, T0_MS);
    contract.cancel(id.clone(), None, None, None);
    assert_eq!(view(&contract, &id).status, EscrowStatus::Cancelled);
}

#[test]
#[should_panic(expected = "only the freelancer or the approver can cancel")]
fn stranger_cannot_cancel() {
    let mut contract = new_contract();
    let id = create_default(&mut contract);
    set_context(STRANGER, T0_MS);
    contract.cancel(id, None, None, None);
}

#[test]
#[should_panic(expected = "only an unfunded escrow can be cancelled")]
fn cancelled_escrow_cannot_be_cancelled_again() {
    let mut contract = new_contract();
    let id = create_default(&mut contract);
    set_context(FREELANCER, T0_MS);
    contract.cancel(id.clone(), None, None, None);
    contract.cancel(id, None, None, None);
}

#[test]
#[should_panic(expected = "unknown escrow")]
fn cancel_unknown_escrow_is_rejected() {
    let mut contract = new_contract();
    set_context(FREELANCER, T0_MS);
    contract.cancel("e42".to_string(), None, None, None);
}

#[test]
#[should_panic(expected = "fee_bps must be at most 10000")]
fn init_rejects_fee_above_100_percent() {
    set_context(CONTRACT, T0_MS);
    VecrowEscrow::new(account(USDC), account(TREASURY), 10_001, WINDOW_SEC);
}

#[test]
#[should_panic(expected = "review_window_sec must be positive")]
fn init_rejects_zero_review_window() {
    set_context(CONTRACT, T0_MS);
    VecrowEscrow::new(account(USDC), account(TREASURY), FEE_BPS, 0);
}

#[test]
fn approver_and_refund_account_never_change_after_creation() {
    // R-04: no method changes them. Run every state-changing method on an
    // escrow and check both after each step.
    let mut contract = new_contract();
    let wallet = Wallet::evm();
    let id = funded_escrow(&mut contract, &wallet);
    let unchanged = |contract: &VecrowEscrow| {
        let escrow = view(contract, &id);
        assert_eq!(escrow.approver, wallet.approver());
        assert_eq!(escrow.refund_account, account(REFUND));
    };
    unchanged(&contract);
    submit(&mut contract, &id, 0, T0_MS);
    unchanged(&contract);
    request_revision(&mut contract, &id, 0, &wallet, 1, T0_MS);
    unchanged(&contract);
    submit(&mut contract, &id, 0, T0_MS);
    approve(&mut contract, &id, 0, &wallet, 2, T0_MS);
    unchanged(&contract);
    assert!(!payout_callback(&mut contract, &id, MEMBER, 1, false));
    set_context(STRANGER, T0_MS);
    contract.retry_payout(id.clone(), account(MEMBER)).detach();
    unchanged(&contract);
    refund(&mut contract, FREELANCER, &id, None);
    unchanged(&contract);
    assert_eq!(view(&contract, &id).status, EscrowStatus::Completed);
}
