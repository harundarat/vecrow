//! R-18 on a sandbox node: the freelancer refunds unsubmitted milestones to
//! the client's refund account, without a fee.

mod common;

use common::*;
use serde_json::json;

#[tokio::test]
async fn release_one_refund_the_rest_and_complete() -> anyhow::Result<()> {
    let env = setup().await?;
    let wallet = Wallet::near(env.client.id());
    let id = env.funded_escrow(&wallet, 1).await?;
    env.submit(&id, 0).await?.into_result()?;
    env.approve(&id, 0, &wallet, 2).await?.into_result()?;

    // The client cannot trigger a refund, and Released milestones are not
    // refundable.
    let refund = |caller, milestone: Option<u32>| {
        env.call(
            caller,
            "refund_unsubmitted",
            json!({ "escrow_id": id, "milestone": milestone }),
        )
    };
    assert_failure(
        refund(&env.client, None).await?,
        "only the freelancer can call this method",
    );
    assert_failure(
        refund(&env.freelancer, Some(0)).await?,
        "only a Pending milestone can be refunded",
    );

    let client_before = env.balance(env.client.id()).await?;
    let treasury_before = env.balance(env.treasury.id()).await?;
    refund(&env.freelancer, Some(1)).await?.into_result()?;

    // The full 500 USDC goes back to the refund account; no fee.
    assert_eq!(
        env.balance(env.client.id()).await? - client_before,
        usdc(500)
    );
    assert_eq!(env.balance(env.treasury.id()).await?, treasury_before);
    assert_eq!(env.balance(env.escrow.id()).await?, 0);

    let escrow = env.get_escrow(&id).await?;
    assert_eq!(escrow["milestones"][1]["status"], "Refunded");
    assert_eq!(escrow["status"], "Completed");
    assert_eq!(escrow["balance"], "0");

    // R-02: a transfer to a Completed escrow is returned in full.
    env.mint(env.client.id(), usdc(1_500)).await?;
    assert_eq!(env.fund(&id, usdc(1_500)).await?, 0);
    assert_eq!(env.balance(env.escrow.id()).await?, 0);
    Ok(())
}

#[tokio::test]
async fn refund_all_pending_milestones() -> anyhow::Result<()> {
    let env = setup().await?;
    let id = env.funded_escrow(&Wallet::evm(), 1).await?;
    env.call(
        &env.freelancer,
        "refund_unsubmitted",
        json!({ "escrow_id": id }),
    )
    .await?
    .into_result()?;
    assert_eq!(env.balance(env.client.id()).await?, usdc(1_500));
    assert_eq!(env.balance(env.treasury.id()).await?, 0);
    assert_eq!(env.status(&id).await?, "Completed");
    Ok(())
}
