//! R-04 on a sandbox node: binding with NEP-413, EIP-191 and Solana ed25519
//! signatures verified by the real NEAR runtime host functions.

mod common;

use common::*;

#[tokio::test]
async fn each_wallet_scheme_binds_through_the_relayer() -> anyhow::Result<()> {
    let env = setup().await?;
    let wallets = [
        Wallet::near(env.client.id()),
        Wallet::evm(),
        Wallet::solana(),
    ];
    for (i, wallet) in wallets.iter().enumerate() {
        let id = env.create_default_escrow().await?;
        env.bind(&id, wallet, i as u8).await?.into_result()?;

        let escrow = env.get_escrow(&id).await?;
        assert_eq!(escrow["status"], "ApproverBound");
        assert_eq!(escrow["approver"], wallet.approver());
        assert_eq!(escrow["refund_account"], env.client.id().as_str());
    }
    Ok(())
}

#[tokio::test]
async fn second_bind_expired_bind_and_reused_nonce_are_rejected() -> anyhow::Result<()> {
    let env = setup().await?;
    let first = env.create_default_escrow().await?;
    let second = env.create_default_escrow().await?;
    let third = env.create_default_escrow().await?;

    env.bind(&first, &Wallet::evm(), 1).await?.into_result()?;
    assert_failure(
        env.bind(&first, &Wallet::solana(), 2).await?,
        "approver is already bound",
    );

    // The same nonce cannot be used again by the same approver.
    assert_failure(
        env.bind(&second, &Wallet::evm(), 1).await?,
        "nonce already used",
    );

    // A deadline in the past is rejected.
    let past = env.now_ms().await? - 1;
    let args = env.bind_args(&third, &Wallet::evm(), env.client.id(), 3, past);
    assert_failure(
        env.call(&env.relayer, "bind_approver", args).await?,
        "signature expired",
    );
    assert_eq!(env.status(&third).await?, "Created");
    Ok(())
}

#[tokio::test]
async fn refund_account_is_required() -> anyhow::Result<()> {
    let env = setup().await?;
    let id = env.create_default_escrow().await?;
    let mut args = env.bind_args(
        &id,
        &Wallet::solana(),
        env.client.id(),
        1,
        env.deadline_ms().await?,
    );
    args.as_object_mut().unwrap().remove("refund_account");
    assert_failure(
        env.call(&env.relayer, "bind_approver", args).await?,
        "refund_account",
    );

    // The freelancer cannot be the refund account.
    let args = env.bind_args(
        &id,
        &Wallet::solana(),
        env.freelancer.id(),
        2,
        env.deadline_ms().await?,
    );
    assert_failure(
        env.call(&env.relayer, "bind_approver", args).await?,
        "refund_account must not be the freelancer or a split recipient",
    );
    assert_eq!(env.status(&id).await?, "Created");
    Ok(())
}
