mod common;

use common::*;
use near_workspaces::types::{NearToken, SecretKey};
use serde_json::json;

#[tokio::test]
async fn full_flow_with_zero_near_users() -> anyhow::Result<()> {
    let env = setup().await?;
    let root = env.worker.root_account()?;
    let zero = |name: &'static str| {
        root.create_subaccount(name)
            .initial_balance(NearToken::from_near(0))
    };
    let freelancer_key = wallet_fixtures::NearWallet::new("zfreelancer", 40);
    let freelancer = zero("zfreelancer")
        .keys(freelancer_key.secret_key().parse::<SecretKey>()?)
        .transact()
        .await?
        .into_result()?;
    let freelancer_wallet = wallet_fixtures::NearWallet::new(freelancer.id().as_str(), 40);
    let member = zero("zmember").transact().await?.into_result()?;
    let client_key = wallet_fixtures::NearWallet::new("zclient", 41);
    let client = zero("zclient")
        .keys(client_key.secret_key().parse::<SecretKey>()?)
        .transact()
        .await?
        .into_result()?;
    let wallet = Wallet::Near(wallet_fixtures::NearWallet::new(client.id().as_str(), 41));
    let relayer = env.new_account("metarelayer", 10).await?;
    for account in [&freelancer, &member, &client] {
        assert_eq!(
            account.view_account().await?.balance,
            NearToken::from_near(0)
        );
        env.register(account.id()).await?;
    }

    let meta = |method: &'static str, args: serde_json::Value| {
        meta::call(
            &env.worker,
            &relayer,
            &freelancer,
            env.escrow.id(),
            method,
            args,
        )
    };

    let terms = Terms {
        escrow_id: env.next_escrow_id(),
        freelancer: freelancer.id().clone(),
        milestones: vec![usdc(1_000), usdc(500)],
        split: vec![
            (freelancer.id().clone(), 7_000),
            (member.id().clone(), 3_000),
        ],
        refund_account: client.id().clone(),
        fee_bps: FEE_BPS,
        deadline_ms: env.deadline_ms().await?,
    };
    let id = terms.escrow_id.as_str();
    let args = env.signed_terms_by(&terms, &freelancer_wallet, &wallet);
    env.call(&env.relayer, "create_escrow", args)
        .await?
        .into_result()?;
    assert_eq!(
        env.get_escrow(id).await?["freelancer"],
        freelancer.id().as_str()
    );

    env.mint(env.client.id(), usdc(1_500)).await?;
    assert_eq!(env.fund(id, usdc(1_500)).await?, usdc(1_500));

    assert_failure(env.submit(id, 0).await?, "only the freelancer");
    meta(
        "submit_milestone",
        json!({ "escrow_id": id, "milestone": 0 }),
    )
    .await?
    .map_err(anyhow::Error::msg)?;

    env.approve(id, 0, &wallet, 2).await?.into_result()?;
    meta("refund_unsubmitted", json!({ "escrow_id": id }))
        .await?
        .map_err(anyhow::Error::msg)?;

    assert_eq!(env.status(id).await?, "Completed");
    assert_eq!(env.balance(env.treasury.id()).await?, usdc(5));
    assert_eq!(env.balance(freelancer.id()).await?, 696_500_000);
    assert_eq!(env.balance(member.id()).await?, 298_500_000);
    assert_eq!(env.balance(client.id()).await?, usdc(500));
    for account in [&freelancer, &member, &client] {
        assert_eq!(
            account.view_account().await?.balance,
            NearToken::from_near(0),
            "{} spent or needed no NEAR",
            account.id()
        );
    }
    Ok(())
}
