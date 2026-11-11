//! R-02 on a sandbox node: real `ft_transfer_call` funding, with NEP-141
//! refunds of rejected or excess amounts to `sender_id`.

mod common;

use common::*;
use serde_json::json;

#[tokio::test]
async fn exact_below_and_above_total() -> anyhow::Result<()> {
    let env = setup().await?;
    env.mint(env.client.id(), usdc(10_000)).await?;

    // Below the total: returned in full, status unchanged.
    let below = env.create_default_escrow().await?;
    env.bind(&below, &Wallet::evm(), 1).await?.into_result()?;
    assert_eq!(env.fund(&below, usdc(1_499)).await?, 0);
    assert_eq!(env.status(&below).await?, "ApproverBound");
    assert_eq!(env.balance(env.client.id()).await?, usdc(10_000));
    assert_eq!(env.balance(env.escrow.id()).await?, 0);

    // Exactly the total.
    assert_eq!(env.fund(&below, usdc(1_500)).await?, usdc(1_500));
    let escrow = env.get_escrow(&below).await?;
    assert_eq!(escrow["status"], "Funded");
    assert_eq!(escrow["balance"], usdc(1_500).to_string());
    assert_eq!(env.balance(env.escrow.id()).await?, usdc(1_500));

    // Above the total: funded with exactly the total, excess returned.
    let above = env.create_default_escrow().await?;
    env.bind(&above, &Wallet::solana(), 2)
        .await?
        .into_result()?;
    assert_eq!(env.fund(&above, usdc(2_000)).await?, usdc(1_500));
    let escrow = env.get_escrow(&above).await?;
    assert_eq!(escrow["status"], "Funded");
    assert_eq!(escrow["balance"], usdc(1_500).to_string());
    assert_eq!(env.balance(env.escrow.id()).await?, usdc(3_000));
    assert_eq!(env.balance(env.client.id()).await?, usdc(7_000));
    Ok(())
}

#[tokio::test]
async fn transfers_to_escrows_not_awaiting_funds_are_returned() -> anyhow::Result<()> {
    let env = setup().await?;
    env.mint(env.client.id(), usdc(10_000)).await?;

    // Created: no approver yet.
    let created = env.create_default_escrow().await?;
    assert_eq!(env.fund(&created, usdc(1_500)).await?, 0);
    assert_eq!(env.status(&created).await?, "Created");

    // Funded: a second transfer is returned.
    let funded = env.create_default_escrow().await?;
    env.bind(&funded, &Wallet::evm(), 1).await?.into_result()?;
    assert_eq!(env.fund(&funded, usdc(1_500)).await?, usdc(1_500));
    assert_eq!(env.fund(&funded, usdc(1_500)).await?, 0);

    // Cancelled.
    let cancelled = env.create_default_escrow().await?;
    env.bind(&cancelled, &Wallet::solana(), 2)
        .await?
        .into_result()?;
    env.call(&env.freelancer, "cancel", json!({ "escrow_id": cancelled }))
        .await?
        .into_result()?;
    assert_eq!(env.fund(&cancelled, usdc(1_500)).await?, 0);

    // Unknown escrow ID.
    assert_eq!(env.fund("999", usdc(1_500)).await?, 0);
    assert_eq!(env.fund("not-an-id", usdc(1_500)).await?, 0);

    // Only the one accepted transfer stayed in the contract.
    assert_eq!(env.balance(env.escrow.id()).await?, usdc(1_500));
    assert_eq!(env.balance(env.client.id()).await?, usdc(8_500));
    Ok(())
}

#[tokio::test]
async fn other_tokens_are_returned() -> anyhow::Result<()> {
    let env = setup().await?;
    let fake = deploy_token(&env.worker, "fake").await?;
    for id in [env.escrow.id(), env.client.id()] {
        env.backend
            .call(fake.id(), "storage_deposit")
            .args_json(json!({ "account_id": id, "registration_only": true }))
            .deposit(near_workspaces::types::NearToken::from_millinear(10))
            .transact()
            .await?
            .into_result()?;
    }
    fake.call("mint")
        .args_json(json!({ "account_id": env.client.id(), "amount": usdc(1_500).to_string() }))
        .transact()
        .await?
        .into_result()?;

    let id = env.create_default_escrow().await?;
    env.bind(&id, &Wallet::evm(), 1).await?.into_result()?;
    assert_eq!(
        env.transfer_call(&fake, &env.client, &id, usdc(1_500))
            .await?,
        0
    );
    assert_eq!(env.status(&id).await?, "ApproverBound");
    Ok(())
}

#[tokio::test]
async fn funded_escrow_cannot_be_cancelled() -> anyhow::Result<()> {
    let env = setup().await?;
    let id = env.funded_escrow(&Wallet::near(env.client.id()), 1).await?;
    assert_failure(
        env.call(&env.freelancer, "cancel", json!({ "escrow_id": id }))
            .await?,
        "only an unfunded escrow can be cancelled",
    );
    assert_eq!(env.status(&id).await?, "Funded");
    Ok(())
}
