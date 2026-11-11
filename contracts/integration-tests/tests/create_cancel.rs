//! R-01 / R-04 / R-23 on a sandbox node: the relayer puts on-chain terms
//! that the freelancer (NEP-413) and the client (any scheme) both signed,
//! verified by the real runtime host functions; then cancellation.

mod common;

use common::*;
use serde_json::json;

#[tokio::test]
async fn terms_signed_by_both_parties_go_on_chain() -> anyhow::Result<()> {
    let env = setup().await?;
    let wallets = [
        Wallet::near(env.client.id()),
        Wallet::evm(),
        Wallet::solana(),
    ];
    for wallet in &wallets {
        let id = env.create_escrow_for(wallet).await?;
        let escrow = env.get_escrow(&id).await?;
        assert_eq!(escrow["escrow_id"], id);
        assert_eq!(escrow["status"], "Created");
        assert_eq!(escrow["freelancer"], env.freelancer.id().as_str());
        assert_eq!(escrow["approver"], wallet.approver());
        assert_eq!(escrow["refund_account"], env.client.id().as_str());
        assert_eq!(escrow["total"], usdc(1_500).to_string());
        assert_eq!(escrow["fee_bps"], FEE_BPS);
        assert_eq!(escrow["review_window_sec"], WINDOW_SEC);
        assert_eq!(escrow["milestones"][0]["status"], "Pending");
        assert_eq!(escrow["split"][1]["bps"], 3_000);
    }
    Ok(())
}

#[tokio::test]
async fn invalid_or_changed_terms_are_rejected() -> anyhow::Result<()> {
    let env = setup().await?;
    let wallet = Wallet::evm();

    // A missing client signature.
    let terms = env.default_terms().await?;
    let mut args = env.signed_terms(&terms, &wallet);
    args.as_object_mut().unwrap().remove("client_signature");
    assert_failure(
        env.call(&env.relayer, "create_escrow", args).await?,
        "client_signature",
    );

    // A client signature from another wallet than the approver (R-04).
    let mut args = env.signed_terms(&terms, &wallet);
    let other = env.signed_terms(&terms, &Wallet::Evm(wallet_fixtures::EvmWallet::new(77)));
    args["client_signature"] = other["client_signature"].clone();
    assert_failure(
        env.call(&env.relayer, "create_escrow", args).await?,
        "invalid client signature",
    );

    // A freelancer signature by another key.
    let mut args = env.signed_terms(&terms, &wallet);
    args["freelancer_signature"]["signature"] =
        json!(wallet_fixtures::NearWallet::new("x", 66).sign_message("other", &[0; 32], "x"));
    assert_failure(
        env.call(&env.relayer, "create_escrow", args).await?,
        "invalid freelancer signature",
    );

    // Terms that differ from what both parties signed: here the split.
    let mut args = env.signed_terms(&terms, &wallet);
    args["terms"]["split"][0]["bps"] = json!(6_000);
    args["terms"]["split"][1]["bps"] = json!(4_000);
    assert_failure(
        env.call(&env.relayer, "create_escrow", args).await?,
        "invalid freelancer signature",
    );

    // A fee that does not match the configuration (R-19).
    let mut fee_terms = env.default_terms().await?;
    fee_terms.fee_bps = 40;
    assert_failure(
        env.create(&fee_terms, &wallet).await?,
        "fee_bps does not match the current configuration",
    );

    // Terms past their deadline.
    let mut expired = env.default_terms().await?;
    expired.deadline_ms = env.now_ms().await? - 1;
    assert_failure(env.create(&expired, &wallet).await?, "terms expired");

    // A refund account is required (R-04).
    let mut args = env.signed_terms(&terms, &wallet);
    args["terms"]
        .as_object_mut()
        .unwrap()
        .remove("refund_account");
    assert_failure(
        env.call(&env.relayer, "create_escrow", args).await?,
        "refund_account",
    );

    // An approver that is a split recipient (R-04).
    let member_terms = env.default_terms().await?;
    let member_wallet = Wallet::Near(wallet_fixtures::NearWallet::new(
        env.member.id().as_str(),
        21,
    ));
    assert_failure(
        env.create(&member_terms, &member_wallet).await?,
        "approver must not be the freelancer or a split recipient",
    );

    // None of these went on-chain; the valid terms still can, once.
    env.create(&terms, &wallet).await?.into_result()?;
    assert_failure(
        env.create(&terms, &wallet).await?,
        "escrow ID already exists",
    );
    assert_eq!(env.status(&terms.escrow_id).await?, "Created");
    Ok(())
}

#[tokio::test]
async fn freelancer_or_approver_cancels_before_funding() -> anyhow::Result<()> {
    let env = setup().await?;
    let wallet = Wallet::evm();
    let by_freelancer = env.create_escrow_for(&wallet).await?;
    let by_approver = env.create_escrow_for(&wallet).await?;

    // Neither a stranger nor the relayer can cancel on its own.
    for caller in [&env.member, &env.relayer] {
        assert_failure(
            env.call(caller, "cancel", json!({ "escrow_id": by_freelancer }))
                .await?,
            "only the freelancer or the approver can cancel",
        );
    }

    // The freelancer cancels as the predecessor.
    env.call(
        &env.freelancer,
        "cancel",
        json!({ "escrow_id": by_freelancer }),
    )
    .await?
    .into_result()?;
    assert_eq!(env.status(&by_freelancer).await?, "Cancelled");

    // The approver signs a cancellation and the relayer forwards it; a
    // signature from another wallet is rejected.
    let deadline = env.deadline_ms().await?;
    let other = Wallet::Evm(wallet_fixtures::EvmWallet::new(77));
    assert_failure(
        env.call(
            &env.relayer,
            "cancel",
            env.cancel_args(&by_approver, &other, 1, deadline),
        )
        .await?,
        "invalid signature",
    );
    env.call(
        &env.relayer,
        "cancel",
        env.cancel_args(&by_approver, &wallet, 1, deadline),
    )
    .await?
    .into_result()?;
    assert_eq!(env.status(&by_approver).await?, "Cancelled");

    // A funded escrow cannot be cancelled by the approver either.
    let funded = env.funded_escrow(&wallet).await?;
    assert_failure(
        env.call(
            &env.relayer,
            "cancel",
            env.cancel_args(&funded, &wallet, 2, deadline),
        )
        .await?,
        "only an unfunded escrow can be cancelled",
    );
    Ok(())
}
