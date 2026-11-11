//! R-01: create and cancel an escrow on a sandbox node.

mod common;

use common::*;
use serde_json::json;

#[tokio::test]
async fn create_escrow_and_cancel_before_funding() -> anyhow::Result<()> {
    let env = setup().await?;
    let id = env.create_default_escrow().await?;
    assert_eq!(id, "1");

    let escrow = env.get_escrow(&id).await?;
    assert_eq!(escrow["status"], "Created");
    assert_eq!(escrow["freelancer"], env.freelancer.id().as_str());
    assert_eq!(escrow["total"], usdc(1_500).to_string());
    assert_eq!(escrow["fee_bps"], FEE_BPS);
    assert_eq!(escrow["review_window_sec"], WINDOW_SEC);
    assert_eq!(escrow["milestones"][0]["status"], "Pending");
    assert_eq!(escrow["split"][1]["bps"], 3_000);

    // Only the freelancer can cancel.
    let result = env
        .call(&env.member, "cancel", json!({ "escrow_id": id }))
        .await?;
    assert_failure(result, "only the freelancer can call this method");

    env.call(&env.freelancer, "cancel", json!({ "escrow_id": id }))
        .await?
        .into_result()?;
    assert_eq!(env.status(&id).await?, "Cancelled");
    Ok(())
}

#[tokio::test]
async fn invalid_split_is_rejected() -> anyhow::Result<()> {
    let env = setup().await?;
    let result = env
        .call(
            &env.freelancer,
            "create_escrow",
            json!({
                "milestones": [usdc(100).to_string()],
                "split": [
                    { "account_id": env.freelancer.id(), "bps": 6_000 },
                    { "account_id": env.member.id(), "bps": 3_000 },
                ],
            }),
        )
        .await?;
    assert_failure(result, "split must total exactly 10000 bps");
    Ok(())
}
