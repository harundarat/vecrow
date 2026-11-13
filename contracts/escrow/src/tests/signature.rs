use super::*;
use crate::signature::*;

fn key(wallet: &Wallet) -> ApproverKey {
    ApproverKey::parse(&wallet.approver()).unwrap()
}

#[test]
fn terms_message_matches_the_expected_bytes() {
    set_context(RELAYER, T0_MS);
    let wallet = Wallet::evm();
    let terms = default_terms("e7", &wallet);
    let message = terms_message_of(&terms, &wallet);
    assert_eq!(
        message,
        format!(
            "Vecrow create_escrow\nContract: escrow.test.near\nEscrow: e7\nFreelancer: freelancer.test.near\nMilestones: 1000000000,500000000\nSplit: freelancer.test.near:7000,member.test.near:3000\nApprover: {}\nRefund account: refund.test.near\nFee: 50 bps\nDeadline: 1700003600000",
            wallet.label()
        )
    );
    assert_eq!(message, super::terms_message(&terms, &wallet.label()));
    assert_eq!(
        terms_nonce(&message),
        wallet_fixtures::terms_nonce(&message)
    );
}

fn terms_message_of(terms: &EscrowTerms, wallet: &Wallet) -> String {
    let key = ApproverKey::parse(&wallet.approver()).unwrap();
    crate::signature::terms_message(&account(CONTRACT), terms, &key)
}

#[test]
fn action_messages_match_the_expected_bytes() {
    let nonce = wallet_fixtures::nonce(0xff);
    for (action, name) in [
        (Action::Approve, "approve_milestone"),
        (Action::RequestRevision, "request_revision"),
    ] {
        let message = action_message(action, &account(CONTRACT), "e12", Some(3), &nonce, 42);
        assert_eq!(
            message,
            format!(
                "Vecrow {name}\nContract: escrow.test.near\nEscrow: e12\nMilestone: 3\nNonce: {}\nDeadline: 42",
                "ff".repeat(32)
            )
        );
        assert_eq!(
            message,
            wallet_fixtures::milestone_message(name, CONTRACT, "e12", 3, &nonce, 42)
        );
    }
    let message = action_message(Action::Cancel, &account(CONTRACT), "e12", None, &nonce, 42);
    assert_eq!(
        message,
        format!(
            "Vecrow cancel\nContract: escrow.test.near\nEscrow: e12\nNonce: {}\nDeadline: 42",
            "ff".repeat(32)
        )
    );
    assert_eq!(
        message,
        wallet_fixtures::cancel_message(CONTRACT, "e12", &nonce, 42)
    );
}

#[test]
fn nep413_payload_is_tag_then_borsh_fields() {
    let nonce = wallet_fixtures::nonce(0x02);
    let payload = nep413_payload("hi", &nonce, "c.near");
    let mut expected = vec![0x9d, 0x01, 0x00, 0x80]; // 2^31 + 413, little-endian
    expected.extend([2, 0, 0, 0]);
    expected.extend(b"hi");
    expected.extend([0x02; 32]);
    expected.extend([6, 0, 0, 0]);
    expected.extend(b"c.near");
    expected.push(0); // callback_url: None
    assert_eq!(payload, expected);
}

#[test]
fn eip191_payload_prefixes_the_byte_length() {
    assert_eq!(
        eip191_payload("héllo"),
        b"\x19Ethereum Signed Message:\n6h\xc3\xa9llo".to_vec()
    );
}

#[test]
fn each_scheme_verifies_its_wallet_signature() {
    set_context(RELAYER, T0_MS);
    let nonce = wallet_fixtures::nonce(0x03);
    for wallet in Wallet::all() {
        let signature = wallet.sign("Vecrow test", &nonce);
        let key = key(&wallet);
        assert_eq!(
            key.verify("Vecrow test", &nonce, CONTRACT, &signature),
            Ok(true)
        );
        assert_eq!(
            key.verify("Vecrow tesT", &nonce, CONTRACT, &signature),
            Ok(false)
        );
    }
}

#[test]
fn nep413_signature_is_bound_to_nonce_and_recipient() {
    set_context(RELAYER, T0_MS);
    let wallet = Wallet::near();
    let nonce = wallet_fixtures::nonce(0x04);
    let signature = wallet.sign("m", &nonce);
    let key = key(&wallet);
    assert_eq!(key.verify("m", &nonce, CONTRACT, &signature), Ok(true));
    let other_nonce = wallet_fixtures::nonce(0x05);
    assert_eq!(
        key.verify("m", &other_nonce, CONTRACT, &signature),
        Ok(false)
    );
    assert_eq!(key.verify("m", &nonce, "other.near", &signature), Ok(false));
}

#[test]
fn a_different_wallet_of_the_same_scheme_is_rejected() {
    set_context(RELAYER, T0_MS);
    let nonce = wallet_fixtures::nonce(0x06);
    let pairs = [
        (Wallet::near(), Wallet::Near(NearWallet::new(CLIENT, 99))),
        (Wallet::evm(), Wallet::Evm(EvmWallet::new(99))),
        (Wallet::solana(), Wallet::Solana(SolanaWallet::new(99))),
    ];
    for (bound, other) in pairs {
        let signature = other.sign("m", &nonce);
        assert_eq!(
            key(&bound).verify("m", &nonce, CONTRACT, &signature),
            Ok(false)
        );
    }
}

#[test]
fn eip191_accepts_v_as_0_or_1_and_without_0x() {
    set_context(RELAYER, T0_MS);
    let wallet = Wallet::evm();
    let nonce = wallet_fixtures::nonce(0);
    let signature = wallet.sign("m", &nonce);
    let mut bytes = hex::decode(&signature[2..]).unwrap();
    bytes[64] -= 27;
    assert_eq!(
        key(&wallet).verify("m", &nonce, CONTRACT, &hex::encode(&bytes)),
        Ok(true)
    );
}

#[test]
fn eip191_rejects_high_s_signatures() {
    set_context(RELAYER, T0_MS);
    let wallet = Wallet::evm();
    let nonce = wallet_fixtures::nonce(0);
    let signature = wallet.sign("m", &nonce);
    let high_s = wallet_fixtures::malleate_evm_signature(&signature);
    assert_ne!(signature, high_s);
    assert_eq!(
        key(&wallet).verify("m", &nonce, CONTRACT, &high_s),
        Ok(false)
    );

    let bytes = hex::decode(&high_s[2..]).unwrap();
    let hash = near_sdk::env::keccak256_array(eip191_payload("m"));
    let public_key = near_sdk::env::ecrecover(&hash, &bytes[..64], bytes[64] - 27, false)
        .expect("recovers without the malleability check");
    let address = format!(
        "0x{}",
        hex::encode(&near_sdk::env::keccak256_array(public_key)[12..])
    );
    assert_eq!(address, EvmWallet::new(12).address());
}

#[test]
fn malformed_signatures_are_errors() {
    set_context(RELAYER, T0_MS);
    let nonce = wallet_fixtures::nonce(0);
    let near = key(&Wallet::near());
    let evm = key(&Wallet::evm());
    let sol = key(&Wallet::solana());
    assert!(near.verify("m", &nonce, CONTRACT, "not base64!").is_err());
    assert!(near.verify("m", &nonce, CONTRACT, "AAAA").is_err());
    assert!(evm.verify("m", &nonce, CONTRACT, "0x1234").is_err());
    let bad_v = format!("0x{}{}", "11".repeat(64), "1d");
    assert_eq!(
        evm.verify("m", &nonce, CONTRACT, &bad_v),
        Err("eip191 signature has an invalid recovery id")
    );
    assert!(sol.verify("m", &nonce, CONTRACT, "0OIl").is_err());
    assert!(sol.verify("m", &nonce, CONTRACT, "2g").is_err());
}

#[test]
fn approver_arguments_are_validated_and_canonicalized() {
    let evm = EvmWallet::new(12);
    let upper = Approver::Eip191 {
        address: format!("0x{}", evm.address()[2..].to_uppercase()),
    };
    let key = ApproverKey::parse(&upper).unwrap();
    assert_eq!(
        key.to_approver(),
        Approver::Eip191 {
            address: evm.address()
        }
    );
    assert_eq!(key.label(), format!("eip191:{}", evm.address()));

    let bad = [
        Approver::Eip191 {
            address: "1234".into(),
        },
        Approver::Eip191 {
            address: "0x1234".into(),
        },
        Approver::Ed25519 {
            address: "short".into(),
        },
        Approver::Nep413 {
            account_id: account(CLIENT),
            public_key: SolanaWallet::new(1).address(),
        },
        Approver::Nep413 {
            account_id: account(CLIENT),
            public_key: "secp256k1:abc".into(),
        },
    ];
    for approver in bad {
        assert!(ApproverKey::parse(&approver).is_err(), "{approver:?}");
    }
}

#[test]
fn approver_json_is_tagged_by_scheme() {
    let json = near_sdk::serde_json::to_value(Approver::Eip191 {
        address: "0xab".into(),
    })
    .unwrap();
    assert_eq!(
        json,
        near_sdk::serde_json::json!({ "scheme": "eip191", "address": "0xab" })
    );
    let parsed: Approver = near_sdk::serde_json::from_value(near_sdk::serde_json::json!({
        "scheme": "nep413", "account_id": "client.near", "public_key": "ed25519:x"
    }))
    .unwrap();
    assert_eq!(
        parsed,
        Approver::Nep413 {
            account_id: account("client.near"),
            public_key: "ed25519:x".into()
        }
    );
}

#[test]
fn equivalent_near_accounts_cover_implicit_forms() {
    let near = NearWallet::new(CLIENT, 11);
    let sol = SolanaWallet::new(13);
    let evm = EvmWallet::new(12);
    assert_eq!(
        key(&Wallet::Near(NearWallet::new(CLIENT, 11))).near_account_ids(),
        vec![CLIENT.to_string(), near.implicit_account_id()]
    );
    assert_eq!(
        key(&Wallet::Solana(sol)).near_account_ids(),
        vec![SolanaWallet::new(13).implicit_account_id()]
    );
    assert_eq!(
        key(&Wallet::Evm(evm)).near_account_ids(),
        vec![EvmWallet::new(12).address()]
    );
}

#[test]
fn nonces_must_be_64_lowercase_hex() {
    assert_eq!(parse_nonce(&"ab".repeat(32)), Ok([0xab; 32]));
    for bad in [
        "AB".repeat(32),
        "ab".repeat(31),
        "ab".repeat(33),
        "zz".repeat(32),
    ] {
        assert!(parse_nonce(&bad).is_err(), "{bad}");
    }
}

#[test]
fn signatures_from_independent_js_implementations_verify() {
    use near_sdk::serde_json::Value;
    set_context(RELAYER, T0_MS);
    let v: Value =
        near_sdk::serde_json::from_str(include_str!("testdata/walletsig_vectors.json")).unwrap();
    let s = |path: &[&str]| -> String {
        path.iter()
            .fold(&v, |value, key| &value[*key])
            .as_str()
            .unwrap()
            .to_string()
    };
    let no_nonce = [0u8; 32];

    let evm = ApproverKey::parse(&Approver::Eip191 {
        address: s(&["evm", "address"]),
    })
    .unwrap();
    let message = s(&["evm", "message"]);
    let signature = s(&["evm", "signature"]);
    assert_eq!(
        evm.verify(&message, &no_nonce, CONTRACT, &signature),
        Ok(true)
    );
    assert_eq!(
        evm.verify(
            &message.replacen('N', "n", 1),
            &no_nonce,
            CONTRACT,
            &signature
        ),
        Ok(false)
    );

    let solana = ApproverKey::parse(&Approver::Ed25519 {
        address: s(&["solana", "address"]),
    })
    .unwrap();
    let message = s(&["solana", "message"]);
    let signature = s(&["solana", "signature"]);
    assert_eq!(
        solana.verify(&message, &no_nonce, CONTRACT, &signature),
        Ok(true)
    );
    assert_eq!(
        solana.verify(
            &message.replacen('N', "n", 1),
            &no_nonce,
            CONTRACT,
            &signature
        ),
        Ok(false)
    );

    let near = ApproverKey::parse(&Approver::Nep413 {
        account_id: account(&s(&["nep413", "account_id"])),
        public_key: s(&["nep413", "public_key"]),
    })
    .unwrap();
    let message = s(&["nep413", "message"]);
    let mut nonce = [0u8; 32];
    hex::decode_to_slice(s(&["nep413", "nonce"]), &mut nonce).unwrap();
    let recipient = s(&["nep413", "recipient"]);
    let signature = s(&["nep413", "signature"]);
    assert_eq!(
        near.verify(&message, &nonce, &recipient, &signature),
        Ok(true)
    );
    assert_eq!(
        near.verify(&message, &nonce, "other.recipient", &signature),
        Ok(false)
    );
    nonce[0] ^= 1;
    assert_eq!(
        near.verify(&message, &nonce, &recipient, &signature),
        Ok(false)
    );
}
