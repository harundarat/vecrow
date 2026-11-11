//! R-07 on a sandbox node, with a review window shortened at deploy time.

mod common;

use common::*;
use serde_json::json;

const DEMO_WINDOW_SEC: u64 = 5;

/// Produces blocks until the block time reaches `target_ms`.
async fn wait_until(env: &Env, target_ms: u64) -> anyhow::Result<()> {
    for _ in 0..120 {
        if env.now_ms().await? > target_ms {
            return Ok(());
        }
        env.worker.fast_forward(5).await?;
        tokio::time::sleep(std::time::Duration::from_millis(250)).await;
    }
    anyhow::bail!("block time did not reach {target_ms}")
}

#[tokio::test]
async fn auto_release_after_a_short_demo_window() -> anyhow::Result<()> {
    let env = setup_with_window(DEMO_WINDOW_SEC).await?;
    let wallet = Wallet::evm();
    let id = env.funded_escrow(&wallet, 1).await?;
    assert_eq!(
        env.get_escrow(&id).await?["review_window_sec"],
        DEMO_WINDOW_SEC
    );
    env.submit(&id, 0).await?.into_result()?;
    let submitted_at = env.get_escrow(&id).await?["milestones"][0]["submitted_at_ms"]
        .as_u64()
        .unwrap();

    let auto_release = || {
        env.call(
            &env.relayer,
            "auto_release",
            json!({ "escrow_id": id, "milestone": 0 }),
        )
    };
    assert_failure(auto_release().await?, "review window has not ended");

    wait_until(&env, submitted_at + DEMO_WINDOW_SEC * 1_000).await?;
    auto_release().await?.into_result()?;
    assert_eq!(env.balance(env.treasury.id()).await?, usdc(5));
    assert_eq!(env.balance(env.freelancer.id()).await?, 696_500_000);
    assert_eq!(env.balance(env.member.id()).await?, 298_500_000);
    assert_eq!(
        env.get_escrow(&id).await?["milestones"][0]["status"],
        "Released"
    );
    Ok(())
}

#[tokio::test]
async fn revision_stops_the_timer_until_resubmission() -> anyhow::Result<()> {
    let env = setup_with_window(DEMO_WINDOW_SEC).await?;
    let wallet = Wallet::solana();
    let id = env.funded_escrow(&wallet, 1).await?;
    env.submit(&id, 0).await?.into_result()?;
    env.signed_milestone_action("request_revision", &id, 0, &wallet, 2)
        .await?
        .into_result()?;

    let auto_release = || {
        env.call(
            &env.relayer,
            "auto_release",
            json!({ "escrow_id": id, "milestone": 0 }),
        )
    };
    let later = env.now_ms().await? + DEMO_WINDOW_SEC * 1_000;
    wait_until(&env, later).await?;
    assert_failure(
        auto_release().await?,
        "a milestone awaiting revision cannot be auto-released",
    );

    // The window restarts at re-submission.
    env.submit(&id, 0).await?.into_result()?;
    let resubmitted_at = env.get_escrow(&id).await?["milestones"][0]["submitted_at_ms"]
        .as_u64()
        .unwrap();
    assert_failure(auto_release().await?, "review window has not ended");
    wait_until(&env, resubmitted_at + DEMO_WINDOW_SEC * 1_000).await?;
    auto_release().await?.into_result()?;
    assert_eq!(env.balance(env.freelancer.id()).await?, 696_500_000);
    Ok(())
}
