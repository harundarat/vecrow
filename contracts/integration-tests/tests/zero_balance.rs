//! R-20 / goal 4 of the spec: the freelancer, the client and the team member
//! complete the whole flow with a NEAR balance of 0.
//!
//! - The freelancer signs NEP-366 delegate actions; a relayer pays the gas,
//!   and the contract still sees the freelancer as the predecessor.
//! - The client never sends a NEAR transaction: they sign the bind and the
//!   approval with their wallet, and the relayer forwards them.
//! - The backend pays `storage_deposit` for every account that receives
//!   USDC; the contract pays the 1 yoctoNEAR of each `ft_transfer`.

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
    let freelancer = zero("zfreelancer").transact().await?.into_result()?;
    let member = zero("zmember").transact().await?.into_result()?;
    // The client's NEAR wallet holds the key it signs NEP-413 messages with.
    let wallet = wallet_fixtures::NearWallet::new("zclient.test.near", 41);
    let client = zero("zclient")
        .keys(wallet.secret_key().parse::<SecretKey>()?)
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

    // Freelancer creates the escrow through a meta transaction.
    meta(
        "create_escrow",
        json!({
            "milestones": [usdc(1_000).to_string(), usdc(500).to_string()],
            "split": [
                { "account_id": freelancer.id(), "bps": 7_000 },
                { "account_id": member.id(), "bps": 3_000 },
            ],
        }),
    )
    .await?
    .map_err(anyhow::Error::msg)?;
    let id = "1";
    assert_eq!(
        env.get_escrow(id).await?["freelancer"],
        freelancer.id().as_str()
    );

    // Client binds their NEAR wallet (NEP-413) with themselves as refund
    // account; the relayer forwards the signed payload.
    let args = env.bind_args(id, &wallet, client.id(), 1, env.deadline_ms().await?);
    env.call(&env.relayer, "bind_approver", args)
        .await?
        .into_result()?;

    // A solver (here the default client account) delivers USDC with
    // ft_transfer_call, as 1Click does.
    env.mint(env.client.id(), usdc(1_500)).await?;
    assert_eq!(env.fund(id, usdc(1_500)).await?, usdc(1_500));

    // The relayer cannot submit on its own; the freelancer's meta
    // transaction can.
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
