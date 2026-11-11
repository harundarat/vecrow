//! R-06 and R-19 on a sandbox node: signed approvals forwarded by the
//! relayer, paid out with ft_transfer + on_payout; R-20 pending payouts for
//! an unregistered recipient.

mod common;

use common::*;
use serde_json::json;

#[tokio::test]
async fn approval_from_each_wallet_pays_fee_and_split() -> anyhow::Result<()> {
    let env = setup().await?;
    let wallets = [
        Wallet::near(env.client.id()),
        Wallet::evm(),
        Wallet::solana(),
    ];
    let accounts = [
        env.treasury.id(),
        env.freelancer.id(),
        env.member.id(),
        env.escrow.id(),
    ];
    for (i, wallet) in wallets.iter().enumerate() {
        let id = env.funded_escrow(wallet, i as u8).await?;
        env.submit(&id, 0).await?.into_result()?;
        let before = env.balances(&accounts).await?;

        let result = env.approve(&id, 0, wallet, 10 + i as u8).await?;
        assert!(result.is_success(), "{:?}", result.into_result());

        // 1,000 USDC at 0.5% with a 70/30 split: 5, 696.5 and 298.5.
        let after = env.balances(&accounts).await?;
        assert_eq!(after[0] - before[0], usdc(5));
        assert_eq!(after[1] - before[1], 696_500_000);
        assert_eq!(after[2] - before[2], 298_500_000);
        assert_eq!(before[3] - after[3], usdc(1_000));

        let escrow = env.get_escrow(&id).await?;
        assert_eq!(escrow["milestones"][0]["status"], "Released");
        assert_eq!(escrow["balance"], usdc(500).to_string());
        assert_eq!(escrow["pending_payouts"], json!([]));
    }
    Ok(())
}

#[tokio::test]
async fn approving_every_milestone_pays_exactly_the_total() -> anyhow::Result<()> {
    let env = setup().await?;
    let wallet = Wallet::evm();
    let id = env.funded_escrow(&wallet, 1).await?;
    let accounts = [env.treasury.id(), env.freelancer.id(), env.member.id()];
    let before = env.balances(&accounts).await?;
    for (milestone, nonce) in [(0, 2), (1, 3)] {
        env.submit(&id, milestone).await?.into_result()?;
        env.approve(&id, milestone, &wallet, nonce)
            .await?
            .into_result()?;
    }
    let after = env.balances(&accounts).await?;
    let paid: u128 = after.iter().zip(&before).map(|(a, b)| a - b).sum();
    assert_eq!(paid, usdc(1_500));
    assert_eq!(after[0] - before[0], usdc(5) + 2_500_000);
    assert_eq!(env.status(&id).await?, "Completed");
    assert_eq!(env.balance(env.escrow.id()).await?, 0);
    Ok(())
}

#[tokio::test]
async fn reused_nonce_expired_payload_and_wrong_signer_are_rejected() -> anyhow::Result<()> {
    let env = setup().await?;
    let wallet = Wallet::solana();
    let id = env.funded_escrow(&wallet, 1).await?;
    env.submit(&id, 0).await?.into_result()?;
    env.submit(&id, 1).await?.into_result()?;
    env.approve(&id, 0, &wallet, 2).await?.into_result()?;

    assert_failure(env.approve(&id, 1, &wallet, 2).await?, "nonce already used");

    let past = env.now_ms().await? - 1;
    let args = env.milestone_args("approve_milestone", &id, 1, &wallet, 3, past);
    assert_failure(
        env.call(&env.relayer, "approve_milestone", args).await?,
        "signature expired",
    );

    let other = Wallet::Solana(wallet_fixtures::SolanaWallet::new(99));
    assert_failure(env.approve(&id, 1, &other, 4).await?, "invalid signature");
    assert_eq!(
        env.get_escrow(&id).await?["milestones"][1]["status"],
        "Submitted"
    );
    Ok(())
}

#[tokio::test]
async fn transfer_to_unregistered_recipient_is_kept_as_pending() -> anyhow::Result<()> {
    let env = setup().await?;
    let newcomer = env.new_account("newcomer", 1).await?;
    let id = env
        .create_escrow(
            &[usdc(1_000)],
            &[(env.freelancer.id(), 7_000), (newcomer.id(), 3_000)],
        )
        .await?;
    let wallet = Wallet::evm();
    env.bind_and_fund(&id, &wallet, usdc(1_000)).await?;
    env.submit(&id, 0).await?.into_result()?;
    env.approve(&id, 0, &wallet, 1).await?.into_result()?;

    // The registered accounts are paid; the unregistered share stays in the
    // contract as a pending payout instead of being lost.
    assert_eq!(env.balance(env.treasury.id()).await?, usdc(5));
    assert_eq!(env.balance(env.freelancer.id()).await?, 696_500_000);
    assert_eq!(env.balance(env.escrow.id()).await?, 298_500_000);
    let escrow = env.get_escrow(&id).await?;
    assert_eq!(escrow["status"], "Completed");
    assert_eq!(
        escrow["pending_payouts"],
        json!([{ "account_id": newcomer.id(), "amount": "298500000" }])
    );
    Ok(())
}

#[tokio::test]
async fn on_payout_is_private() -> anyhow::Result<()> {
    let env = setup().await?;
    let id = env.funded_escrow(&Wallet::evm(), 1).await?;
    let result = env
        .call(
            &env.member,
            "on_payout",
            json!({ "escrow_id": id, "account_id": env.member.id(), "amount": "1" }),
        )
        .await?;
    assert_failure(result, "Method on_payout is private");
    Ok(())
}
