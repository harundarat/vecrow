//! Shared setup for the sandbox tests: a local NEAR node with the mock USDC
//! token, the escrow contract, and the accounts of a typical escrow.
//!
//! The wasm files come from `make build` (cargo-near), so the tests run the
//! same binaries that would be deployed.

#![allow(dead_code)] // Each test binary uses a different subset.

use anyhow::{Context, Result, ensure};
use near_workspaces::network::Sandbox;
use near_workspaces::result::ExecutionFinalResult;
use near_workspaces::types::{Gas, NearToken, SecretKey};
use near_workspaces::{Account, AccountId, Contract, Worker};
use serde_json::{Value, json};
use std::cell::Cell;
use std::path::PathBuf;

pub const FEE_BPS: u16 = 50;
pub const WINDOW_SEC: u64 = 14 * 24 * 60 * 60;
pub const ONE_YOCTO: NearToken = NearToken::from_yoctonear(1);

/// USDC amount in base units (6 decimals).
pub fn usdc(whole: u128) -> u128 {
    whole * 1_000_000
}

/// Reads `target/near/<dir>/<file>`, written by `make build`.
fn wasm(dir: &str, file: &str) -> Result<Vec<u8>> {
    let path = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("../target/near")
        .join(dir)
        .join(file);
    std::fs::read(&path).with_context(|| {
        format!(
            "missing {}: run `make build` in contracts/ first",
            path.display()
        )
    })
}

pub struct Env {
    pub worker: Worker<Sandbox>,
    pub usdc: Contract,
    pub escrow: Contract,
    pub treasury: Account,
    /// Pays `storage_deposit` registrations, like the Vecrow backend.
    pub backend: Account,
    /// Pays gas to forward signed payloads, like the Vecrow relayer.
    pub relayer: Account,
    pub freelancer: Account,
    /// The freelancer's NEAR wallet key, the same key as `freelancer`'s.
    pub freelancer_wallet: wallet_fixtures::NearWallet,
    pub member: Account,
    /// The client's NEAR account: funds the escrow and receives refunds.
    pub client: Account,
    next_escrow: Cell<u32>,
}

/// Deploys and initializes a mock NEP-141 token on a new account `name`.
pub async fn deploy_token(worker: &Worker<Sandbox>, name: &str) -> Result<Contract> {
    let account = subaccount(worker, name, 20).await?;
    let token = account
        .deploy(&wasm("mock-ft", "mock_ft.wasm")?)
        .await?
        .into_result()?;
    token
        .call("new")
        .args_json(json!({ "symbol": "USDC", "decimals": 6 }))
        .transact()
        .await?
        .into_result()?;
    Ok(token)
}

async fn subaccount(worker: &Worker<Sandbox>, name: &str, near: u128) -> Result<Account> {
    let root = worker.root_account()?;
    let account = root
        .create_subaccount(name)
        .initial_balance(NearToken::from_near(near))
        .transact()
        .await?
        .into_result()?;
    Ok(account)
}

/// Starts a sandbox with the default 14-day review window.
pub async fn setup() -> Result<Env> {
    setup_with_window(WINDOW_SEC).await
}

/// Starts a sandbox, deploys the mock USDC and the escrow contract, and
/// registers the escrow, the treasury, the freelancer, the member and the
/// client on the token, as the backend does at deploy and before
/// `create_escrow`.
pub async fn setup_with_window(review_window_sec: u64) -> Result<Env> {
    let worker = near_workspaces::sandbox().await?;
    let root = worker.root_account()?;

    let usdc = deploy_token(&worker, "usdc").await?;
    let treasury = subaccount(&worker, "treasury", 5).await?;
    let escrow_account = subaccount(&worker, "escrow", 30).await?;
    let escrow = escrow_account
        .deploy(&wasm("escrow", "vecrow_escrow.wasm")?)
        .await?
        .into_result()?;
    escrow
        .call("new")
        .args_json(json!({
            "usdc_contract_id": usdc.id(),
            "treasury_account": treasury.id(),
            "fee_bps": FEE_BPS,
            "review_window_sec": review_window_sec,
        }))
        .transact()
        .await?
        .into_result()?;

    // The freelancer's account holds the key its NEAR wallet signs with.
    let freelancer_wallet =
        wallet_fixtures::NearWallet::new(&format!("freelancer.{}", root.id()), 5);
    let freelancer = root
        .create_subaccount("freelancer")
        .initial_balance(NearToken::from_near(5))
        .keys(freelancer_wallet.secret_key().parse::<SecretKey>()?)
        .transact()
        .await?
        .into_result()?;

    let env = Env {
        backend: subaccount(&worker, "backend", 20).await?,
        relayer: subaccount(&worker, "relayer", 20).await?,
        freelancer,
        freelancer_wallet,
        member: subaccount(&worker, "member", 5).await?,
        client: subaccount(&worker, "client", 5).await?,
        worker,
        usdc,
        escrow,
        treasury,
        next_escrow: Cell::new(1),
    };
    for id in [
        env.escrow.id().clone(),
        env.treasury.id().clone(),
        env.freelancer.id().clone(),
        env.member.id().clone(),
        env.client.id().clone(),
    ] {
        env.register(&id).await?;
    }
    Ok(env)
}

/// A client wallet of one of the three supported schemes.
pub enum Wallet {
    Near(wallet_fixtures::NearWallet),
    Evm(wallet_fixtures::EvmWallet),
    Solana(wallet_fixtures::SolanaWallet),
}

impl Wallet {
    pub fn near(account_id: &AccountId) -> Self {
        Self::Near(wallet_fixtures::NearWallet::new(account_id.as_str(), 11))
    }

    pub fn evm() -> Self {
        Self::Evm(wallet_fixtures::EvmWallet::new(12))
    }

    pub fn solana() -> Self {
        Self::Solana(wallet_fixtures::SolanaWallet::new(13))
    }

    /// The `approver` field of the terms.
    pub fn approver(&self) -> Value {
        match self {
            Self::Near(w) => json!({
                "scheme": "nep413",
                "account_id": w.account_id(),
                "public_key": w.public_key(),
            }),
            Self::Evm(w) => json!({ "scheme": "eip191", "address": w.address() }),
            Self::Solana(w) => json!({ "scheme": "ed25519", "address": w.address() }),
        }
    }

    pub fn label(&self) -> String {
        match self {
            Self::Near(w) => w.approver_label(),
            Self::Evm(w) => w.approver_label(),
            Self::Solana(w) => w.approver_label(),
        }
    }

    /// Signs like the wallet; NEP-413 uses `recipient` (the escrow contract).
    pub fn sign(&self, message: &str, nonce: &[u8; 32], recipient: &AccountId) -> String {
        match self {
            Self::Near(w) => w.sign_message(message, nonce, recipient.as_str()),
            Self::Evm(w) => w.personal_sign(message),
            Self::Solana(w) => w.sign_message(message),
        }
    }
}

/// Escrow terms, before they are signed.
pub struct Terms {
    pub escrow_id: String,
    pub freelancer: AccountId,
    pub milestones: Vec<u128>,
    pub split: Vec<(AccountId, u16)>,
    pub refund_account: AccountId,
    pub fee_bps: u16,
    pub deadline_ms: u64,
}

impl Env {
    /// A new escrow ID, `e1`, `e2`, … as the backend would assign.
    pub fn next_escrow_id(&self) -> String {
        let n = self.next_escrow.get();
        self.next_escrow.set(n + 1);
        format!("e{n}")
    }

    /// Current block time in milliseconds.
    pub async fn now_ms(&self) -> Result<u64> {
        Ok(self.worker.view_block().await?.timestamp() / 1_000_000)
    }

    /// A deadline one hour after the current block time.
    pub async fn deadline_ms(&self) -> Result<u64> {
        Ok(self.now_ms().await? + 3_600_000)
    }

    /// The default terms: 1,000 and 500 USDC milestones, a 70/30 split
    /// between the freelancer and the member, the client's account for
    /// refunds, the production fee and a one-hour deadline.
    pub async fn default_terms(&self) -> Result<Terms> {
        Ok(Terms {
            escrow_id: self.next_escrow_id(),
            freelancer: self.freelancer.id().clone(),
            milestones: vec![usdc(1_000), usdc(500)],
            split: vec![
                (self.freelancer.id().clone(), 7_000),
                (self.member.id().clone(), 3_000),
            ],
            refund_account: self.client.id().clone(),
            fee_bps: FEE_BPS,
            deadline_ms: self.deadline_ms().await?,
        })
    }

    /// `create_escrow` arguments: the terms with `client` as approver, signed
    /// by the freelancer's NEAR wallet (NEP-413) and by `client`.
    pub fn signed_terms(&self, terms: &Terms, client: &Wallet) -> Value {
        self.signed_terms_by(terms, &self.freelancer_wallet, client)
    }

    /// Like `signed_terms`, with another freelancer wallet.
    pub fn signed_terms_by(
        &self,
        terms: &Terms,
        freelancer: &wallet_fixtures::NearWallet,
        client: &Wallet,
    ) -> Value {
        let message = wallet_fixtures::terms_message(&wallet_fixtures::Terms {
            contract_id: self.escrow.id().to_string(),
            escrow_id: terms.escrow_id.clone(),
            freelancer: terms.freelancer.to_string(),
            milestones: terms.milestones.clone(),
            split: terms
                .split
                .iter()
                .map(|(a, bps)| (a.to_string(), *bps))
                .collect(),
            approver: client.label(),
            refund_account: terms.refund_account.to_string(),
            fee_bps: terms.fee_bps,
            deadline_ms: terms.deadline_ms,
        });
        let nonce = wallet_fixtures::terms_nonce(&message);
        let milestones: Vec<String> = terms.milestones.iter().map(u128::to_string).collect();
        let split: Vec<Value> = terms
            .split
            .iter()
            .map(|(account_id, bps)| json!({ "account_id": account_id, "bps": bps }))
            .collect();
        json!({
            "terms": {
                "escrow_id": terms.escrow_id,
                "freelancer": terms.freelancer,
                "milestones": milestones,
                "split": split,
                "approver": client.approver(),
                "refund_account": terms.refund_account,
                "fee_bps": terms.fee_bps,
                "deadline_ms": terms.deadline_ms,
            },
            "freelancer_signature": {
                "public_key": freelancer.public_key(),
                "signature": freelancer.sign_message(
                    &message,
                    &nonce,
                    self.escrow.id().as_str(),
                ),
            },
            "client_signature": client.sign(&message, &nonce, self.escrow.id()),
        })
    }

    /// The relayer puts `terms` on-chain with `client` as the approver.
    pub async fn create(&self, terms: &Terms, client: &Wallet) -> Result<ExecutionFinalResult> {
        let args = self.signed_terms(terms, client);
        self.call(&self.relayer, "create_escrow", args).await
    }

    /// Puts the default terms on-chain with `client` as the approver.
    pub async fn create_escrow_for(&self, client: &Wallet) -> Result<String> {
        let terms = self.default_terms().await?;
        self.create(&terms, client).await?.into_result()?;
        Ok(terms.escrow_id)
    }

    /// Puts terms with the given milestones and `(account, bps)` split
    /// on-chain, with `client` as the approver.
    pub async fn create_escrow(
        &self,
        milestones: &[u128],
        split: &[(&AccountId, u16)],
        client: &Wallet,
    ) -> Result<String> {
        let mut terms = self.default_terms().await?;
        terms.milestones = milestones.to_vec();
        terms.split = split.iter().map(|(a, bps)| ((*a).clone(), *bps)).collect();
        self.create(&terms, client).await?.into_result()?;
        Ok(terms.escrow_id)
    }

    /// Mints `amount` to the client and funds the escrow with it.
    pub async fn mint_and_fund(&self, escrow_id: &str, amount: u128) -> Result<()> {
        self.mint(self.client.id(), amount).await?;
        ensure!(self.fund(escrow_id, amount).await? == amount);
        Ok(())
    }

    /// Puts the default terms on-chain with `client` as the approver and funds
    /// the escrow with exactly the total.
    pub async fn funded_escrow(&self, client: &Wallet) -> Result<String> {
        let id = self.create_escrow_for(client).await?;
        self.mint_and_fund(&id, usdc(1_500)).await?;
        Ok(id)
    }

    /// Registers `account_id` on the token with `storage_deposit`
    /// (`registration_only`), paid by the backend account.
    pub async fn register(&self, account_id: &AccountId) -> Result<()> {
        let bounds: Value = self.usdc.view("storage_balance_bounds").await?.json()?;
        let min: u128 = bounds["min"].as_str().context("min")?.parse()?;
        self.backend
            .call(self.usdc.id(), "storage_deposit")
            .args_json(json!({ "account_id": account_id, "registration_only": true }))
            .deposit(NearToken::from_yoctonear(min))
            .transact()
            .await?
            .into_result()?;
        Ok(())
    }

    /// `sender` calls `ft_transfer_call` on `token` to the escrow contract
    /// with `msg = escrow_id`. The result is the amount the escrow kept.
    pub async fn transfer_call(
        &self,
        token: &Contract,
        sender: &Account,
        escrow_id: &str,
        amount: u128,
    ) -> Result<u128> {
        let used: String = sender
            .call(token.id(), "ft_transfer_call")
            .args_json(json!({
                "receiver_id": self.escrow.id(),
                "amount": amount.to_string(),
                "msg": escrow_id,
            }))
            .deposit(ONE_YOCTO)
            .gas(Gas::from_tgas(100))
            .transact()
            .await?
            .into_result()?
            .json()?;
        Ok(used.parse()?)
    }

    /// The client funds `escrow_id` with USDC; returns the amount kept.
    pub async fn fund(&self, escrow_id: &str, amount: u128) -> Result<u128> {
        self.transfer_call(&self.usdc, &self.client, escrow_id, amount)
            .await
    }

    pub async fn mint(&self, account_id: &AccountId, amount: u128) -> Result<()> {
        self.usdc
            .call("mint")
            .args_json(json!({ "account_id": account_id, "amount": amount.to_string() }))
            .transact()
            .await?
            .into_result()?;
        Ok(())
    }

    pub async fn balance(&self, account_id: &AccountId) -> Result<u128> {
        let balance: String = self
            .usdc
            .view("ft_balance_of")
            .args_json(json!({ "account_id": account_id }))
            .await?
            .json()?;
        Ok(balance.parse()?)
    }

    /// Token balances of the given accounts.
    pub async fn balances(&self, accounts: &[&AccountId]) -> Result<Vec<u128>> {
        let mut balances = Vec::new();
        for account in accounts {
            balances.push(self.balance(account).await?);
        }
        Ok(balances)
    }

    pub async fn get_escrow(&self, escrow_id: &str) -> Result<Value> {
        let escrow: Value = self
            .escrow
            .view("get_escrow")
            .args_json(json!({ "escrow_id": escrow_id }))
            .await?
            .json()?;
        ensure!(!escrow.is_null(), "escrow {escrow_id} not found");
        Ok(escrow)
    }

    pub async fn status(&self, escrow_id: &str) -> Result<String> {
        Ok(self.get_escrow(escrow_id).await?["status"]
            .as_str()
            .context("status")?
            .to_string())
    }

    /// Calls an escrow method as `caller` with no attached deposit.
    pub async fn call(
        &self,
        caller: &Account,
        method: &str,
        args: Value,
    ) -> Result<ExecutionFinalResult> {
        Ok(caller
            .call(self.escrow.id(), method)
            .args_json(args)
            .gas(Gas::from_tgas(300))
            .transact()
            .await?)
    }

    /// Creates a sandbox account with `near` NEAR that is not registered on
    /// the token.
    pub async fn new_account(&self, name: &str, near: u128) -> Result<Account> {
        subaccount(&self.worker, name, near).await
    }

    pub async fn submit(&self, escrow_id: &str, milestone: u32) -> Result<ExecutionFinalResult> {
        self.call(
            &self.freelancer,
            "submit_milestone",
            json!({ "escrow_id": escrow_id, "milestone": milestone }),
        )
        .await
    }

    /// Arguments of `approve_milestone` or `request_revision` signed by
    /// `wallet`.
    pub fn milestone_args(
        &self,
        action: &str,
        escrow_id: &str,
        milestone: u32,
        wallet: &Wallet,
        nonce: u8,
        deadline_ms: u64,
    ) -> Value {
        let nonce = wallet_fixtures::nonce(nonce);
        let message = wallet_fixtures::milestone_message(
            action,
            self.escrow.id().as_str(),
            escrow_id,
            milestone,
            &nonce,
            deadline_ms,
        );
        json!({
            "escrow_id": escrow_id,
            "milestone": milestone,
            "nonce": wallet_fixtures::nonce_hex(&nonce),
            "deadline_ms": deadline_ms,
            "signature": wallet.sign(&message, &nonce, self.escrow.id()),
        })
    }

    /// The relayer forwards `action` (`approve_milestone` or
    /// `request_revision`) signed by `wallet`.
    pub async fn signed_milestone_action(
        &self,
        action: &str,
        escrow_id: &str,
        milestone: u32,
        wallet: &Wallet,
        nonce: u8,
    ) -> Result<ExecutionFinalResult> {
        let args = self.milestone_args(
            action,
            escrow_id,
            milestone,
            wallet,
            nonce,
            self.deadline_ms().await?,
        );
        self.call(&self.relayer, action, args).await
    }

    pub async fn approve(
        &self,
        escrow_id: &str,
        milestone: u32,
        wallet: &Wallet,
        nonce: u8,
    ) -> Result<ExecutionFinalResult> {
        self.signed_milestone_action("approve_milestone", escrow_id, milestone, wallet, nonce)
            .await
    }
}

/// Asserts that a transaction failed and that its error mentions `expected`.
pub fn assert_failure(result: ExecutionFinalResult, expected: &str) {
    match result.into_result() {
        Ok(_) => panic!("expected failure containing {expected:?}, but the call succeeded"),
        Err(err) => {
            let message = format!("{err:?}");
            assert!(
                message.contains(expected),
                "expected failure containing {expected:?}, got {message}"
            );
        }
    }
}

pub mod meta {
    use super::*;
    use near_crypto::{InMemorySigner, SecretKey, Signer};
    use near_jsonrpc_client::{JsonRpcClient, methods};
    use near_primitives::action::delegate::{
        DelegateAction, NonDelegateAction, SignedDelegateAction,
    };
    use near_primitives::hash::CryptoHash;
    use near_primitives::transaction::{
        Action, FunctionCallAction, SignedTransaction, Transaction, TransactionV0,
    };
    use near_primitives::views::{ExecutionStatusView, FinalExecutionOutcomeView};

    fn signer(account: &Account) -> Result<Signer> {
        let key: SecretKey = account.secret_key().to_string().parse()?;
        Ok(InMemorySigner::from_secret_key(account.id().clone(), key))
    }

    async fn next_nonce(worker: &Worker<Sandbox>, account: &Account) -> Result<u64> {
        let key = account.secret_key().public_key();
        Ok(worker.view_access_key(account.id(), &key).await?.nonce + 1)
    }

    /// `sender` signs a delegate action calling `method` on `receiver` with
    /// no deposit; `relayer` wraps it in a transaction and pays for it.
    /// Returns the error of the first failed receipt, if any.
    pub async fn call(
        worker: &Worker<Sandbox>,
        relayer: &Account,
        sender: &Account,
        receiver: &AccountId,
        method: &str,
        args: Value,
    ) -> Result<std::result::Result<(), String>> {
        let block = worker.view_block().await?;
        let sender_signer = signer(sender)?;
        let delegate = DelegateAction {
            sender_id: sender.id().clone(),
            receiver_id: receiver.clone(),
            actions: vec![
                NonDelegateAction::try_from(Action::FunctionCall(Box::new(FunctionCallAction {
                    method_name: method.to_string(),
                    args: serde_json::to_vec(&args)?,
                    gas: near_primitives::types::Gas::from_teragas(250),
                    deposit: NearToken::from_yoctonear(0),
                })))
                .map_err(|_| anyhow::anyhow!("not a delegate-able action"))?,
            ],
            nonce: next_nonce(worker, sender).await?,
            max_block_height: block.height() + 100,
            public_key: sender_signer.public_key(),
        };
        let signed_delegate = SignedDelegateAction::sign(&sender_signer, delegate);

        let relayer_signer = signer(relayer)?;
        let tx = Transaction::V0(TransactionV0 {
            signer_id: relayer.id().clone(),
            public_key: relayer_signer.public_key(),
            nonce: next_nonce(worker, relayer).await?,
            receiver_id: sender.id().clone(),
            block_hash: CryptoHash(block.hash().0),
            actions: vec![Action::Delegate(Box::new(signed_delegate))],
        });
        let (hash, _) = tx.get_hash_and_size();
        let signed = SignedTransaction::new(relayer_signer.sign(hash.as_ref()), tx);

        let rpc = JsonRpcClient::connect(worker.rpc_addr());
        let outcome: FinalExecutionOutcomeView = rpc
            .call(methods::broadcast_tx_commit::RpcBroadcastTxCommitRequest {
                signed_transaction: signed,
            })
            .await?;
        for receipt in &outcome.receipts_outcome {
            if let ExecutionStatusView::Failure(err) = &receipt.outcome.status {
                return Ok(Err(format!("{err:?}")));
            }
        }
        Ok(Ok(()))
    }
}
