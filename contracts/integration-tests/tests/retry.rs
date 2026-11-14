mod common;

use common::*;
use near_workspaces::types::Gas;
use serde_json::json;

#[tokio::test]
async fn pending_payout_is_retried_after_registration() -> anyhow::Result<()> {
    let env = setup().await?;
    let newcomer = env.new_account("newcomer", 1).await?;
    let wallet = Wallet::solana();
    let id = env
        .create_escrow(
            &[usdc(1_000)],
            &[(env.freelancer.id(), 7_000), (newcomer.id(), 3_000)],
            &wallet,
        )
        .await?;
    env.mint_and_fund(&id, usdc(1_000)).await?;
    env.submit(&id, 0).await?.into_result()?;
    env.approve(&id, 0, &wallet, 1).await?.into_result()?;
    let pending = json!([{ "account_id": newcomer.id(), "amount": "298500000" }]);
    assert_eq!(env.get_escrow(&id).await?["pending_payouts"], pending);

    let retry = |caller| {
        env.call(
            caller,
            "retry_payout",
            json!({ "escrow_id": id, "account_id": newcomer.id() }),
        )
    };

    let paid: bool = retry(&env.relayer).await?.into_result()?.json()?;
    assert!(!paid);
    assert_eq!(env.get_escrow(&id).await?["pending_payouts"], pending);
    assert_eq!(env.balance(env.escrow.id()).await?, 298_500_000);

    env.register(newcomer.id()).await?;
    let paid: bool = retry(&env.relayer).await?.into_result()?.json()?;
    assert!(paid);
    assert_eq!(env.balance(newcomer.id()).await?, 298_500_000);
    assert_eq!(env.balance(env.escrow.id()).await?, 0);
    assert_eq!(env.get_escrow(&id).await?["pending_payouts"], json!([]));

    assert_failure(
        retry(&env.relayer).await?,
        "no pending payout for this account",
    );
    assert_eq!(env.balance(newcomer.id()).await?, 298_500_000);
    Ok(())
}

#[tokio::test]
async fn concurrent_retries_pay_once() -> anyhow::Result<()> {
    let env = setup().await?;
    let refundee = env.new_account("refundee", 1).await?;
    let mut terms = env.default_terms().await?;
    terms.refund_account = refundee.id().clone();
    let id = terms.escrow_id.clone();
    env.create(&terms, &Wallet::evm()).await?.into_result()?;
    env.mint_and_fund(&id, usdc(1_500)).await?;

    env.call(
        &env.freelancer,
        "refund_unsubmitted",
        json!({ "escrow_id": id }),
    )
    .await?
    .into_result()?;
    assert_eq!(
        env.get_escrow(&id).await?["pending_payouts"],
        json!([{ "account_id": refundee.id(), "amount": usdc(1_500).to_string() }])
    );
    env.register(refundee.id()).await?;

    let send = |caller: &near_workspaces::Account| {
        caller
            .call(env.escrow.id(), "retry_payout")
            .args_json(json!({ "escrow_id": id, "account_id": refundee.id() }))
            .gas(Gas::from_tgas(100))
            .transact_async()
    };
    let first = send(&env.relayer).await?;
    let second = send(&env.backend).await?;
    let results = [first.await?, second.await?];
    let succeeded = results.iter().filter(|r| r.is_success()).count();
    assert_eq!(succeeded, 1, "exactly one retry goes through");

    assert_eq!(env.balance(refundee.id()).await?, usdc(1_500));
    assert_eq!(env.balance(env.escrow.id()).await?, 0);
    assert_eq!(env.get_escrow(&id).await?["pending_payouts"], json!([]));
    Ok(())
}
