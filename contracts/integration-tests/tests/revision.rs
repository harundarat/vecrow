mod common;

use common::*;

#[tokio::test]
async fn two_revisions_then_approval() -> anyhow::Result<()> {
    let env = setup().await?;
    let wallet = Wallet::near(env.client.id());
    let id = env.funded_escrow(&wallet).await?;
    env.submit(&id, 0).await?.into_result()?;

    let other = Wallet::Near(wallet_fixtures::NearWallet::new(
        env.client.id().as_str(),
        99,
    ));
    assert_failure(
        env.signed_milestone_action("request_revision", &id, 0, &other, 2)
            .await?,
        "invalid signature",
    );

    for (round, nonce) in [(1, 3), (2, 4)] {
        env.signed_milestone_action("request_revision", &id, 0, &wallet, nonce)
            .await?
            .into_result()?;
        let m = &env.get_escrow(&id).await?["milestones"][0];
        assert_eq!(m["status"], "RevisionRequested");
        assert_eq!(m["revision_count"], round);
        assert!(m["submitted_at_ms"].is_null());
        env.submit(&id, 0).await?.into_result()?;
    }

    assert_failure(
        env.signed_milestone_action("request_revision", &id, 0, &wallet, 5)
            .await?,
        "revision limit reached",
    );
    assert_eq!(env.balance(env.freelancer.id()).await?, 0);

    env.approve(&id, 0, &wallet, 6).await?.into_result()?;
    assert_eq!(env.balance(env.freelancer.id()).await?, 696_500_000);
    Ok(())
}
