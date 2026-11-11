//! Unit tests for the contract logic, run against near-sdk's mocked
//! blockchain. Cross-contract flows (token transfers and callbacks) are
//! covered by the sandbox tests in `integration-tests/`.

mod approve;
mod bind;
mod create;
mod funding;
mod payout;
mod signature;
mod split;
mod submit;

use crate::*;
use near_sdk::json_types::U128;
use near_sdk::test_utils::VMContextBuilder;
use near_sdk::{NearToken, testing_env};
use wallet_fixtures::{EvmWallet, NearWallet, SolanaWallet};

pub const CONTRACT: &str = "escrow.test.near";
pub const USDC: &str = "usdc.test.near";
pub const TREASURY: &str = "treasury.test.near";
pub const FREELANCER: &str = "freelancer.test.near";
pub const MEMBER: &str = "member.test.near";
pub const MEMBER2: &str = "member2.test.near";
pub const STRANGER: &str = "stranger.test.near";
pub const RELAYER: &str = "relayer.test.near";
pub const CLIENT: &str = "client.test.near";
pub const REFUND: &str = "refund.test.near";

/// 2023-11-14T22:13:20Z: a realistic block time for the tests.
pub const T0_MS: u64 = 1_700_000_000_000;
/// The production fee: 0.5%.
pub const FEE_BPS: u16 = 50;
/// The production review window: 14 days.
pub const WINDOW_SEC: u64 = 14 * 24 * 60 * 60;

pub fn account(id: &str) -> AccountId {
    id.parse().unwrap()
}

/// USDC amount in base units (6 decimals).
pub fn usdc(whole: u128) -> u128 {
    whole * 1_000_000
}

/// Sets the mocked context: `predecessor` calls the contract at `now_ms`,
/// with no attached deposit, as every user-facing call must allow.
pub fn set_context(predecessor: &str, now_ms: u64) {
    testing_env!(
        VMContextBuilder::new()
            .current_account_id(account(CONTRACT))
            .predecessor_account_id(account(predecessor))
            .signer_account_id(account(predecessor))
            .block_timestamp(now_ms * 1_000_000)
            .attached_deposit(NearToken::from_yoctonear(0))
            .build()
    );
}

pub fn new_contract() -> VecrowEscrow {
    set_context(CONTRACT, T0_MS);
    VecrowEscrow::new(account(USDC), account(TREASURY), FEE_BPS, WINDOW_SEC)
}

pub fn share(id: &str, bps: u16) -> SplitShare {
    SplitShare {
        account_id: account(id),
        bps,
    }
}

/// The default split used by most tests: 70/30 between the freelancer and a
/// team member.
pub fn split_70_30() -> Vec<SplitShare> {
    vec![share(FREELANCER, 7_000), share(MEMBER, 3_000)]
}

/// Creates an escrow as the freelancer with 1,000 and 500 USDC milestones.
pub fn create_default(contract: &mut VecrowEscrow) -> String {
    set_context(FREELANCER, T0_MS);
    contract.create_escrow(vec![U128(usdc(1_000)), U128(usdc(500))], split_70_30())
}

pub fn view(contract: &VecrowEscrow, escrow_id: &str) -> EscrowView {
    contract
        .get_escrow(escrow_id.to_string())
        .expect("escrow exists")
}

/// A client wallet of one of the three supported schemes.
pub enum Wallet {
    Near(NearWallet),
    Evm(EvmWallet),
    Solana(SolanaWallet),
}

impl Wallet {
    pub fn near() -> Self {
        Self::Near(NearWallet::new(CLIENT, 11))
    }

    pub fn evm() -> Self {
        Self::Evm(EvmWallet::new(12))
    }

    pub fn solana() -> Self {
        Self::Solana(SolanaWallet::new(13))
    }

    pub fn all() -> [Self; 3] {
        [Self::near(), Self::evm(), Self::solana()]
    }

    /// The `approver` argument of `bind_approver`.
    pub fn approver(&self) -> Approver {
        match self {
            Self::Near(w) => Approver::Nep413 {
                account_id: account(w.account_id()),
                public_key: w.public_key(),
            },
            Self::Evm(w) => Approver::Eip191 {
                address: w.address(),
            },
            Self::Solana(w) => Approver::Ed25519 {
                address: w.address(),
            },
        }
    }

    /// `{scheme}:{address}` as written in the bind message.
    pub fn label(&self) -> String {
        match self {
            Self::Near(w) => w.approver_label(),
            Self::Evm(w) => w.approver_label(),
            Self::Solana(w) => w.approver_label(),
        }
    }

    /// Signs `message` the way the wallet does; NEP-413 uses the contract as
    /// the recipient.
    pub fn sign(&self, message: &str, nonce: &[u8; 32]) -> String {
        match self {
            Self::Near(w) => w.sign_message(message, nonce, CONTRACT),
            Self::Evm(w) => w.personal_sign(message),
            Self::Solana(w) => w.sign_message(message),
        }
    }
}

/// A deadline one hour after `T0_MS`.
pub const DEADLINE_MS: u64 = T0_MS + 3_600_000;

/// Arguments of a signed `bind_approver` call.
pub struct BindArgs {
    pub escrow_id: String,
    pub approver: Approver,
    pub refund_account: AccountId,
    pub nonce: String,
    pub deadline_ms: u64,
    pub signature: String,
}

/// Signs a bind payload with `wallet` for `refund_account`.
pub fn sign_bind(wallet: &Wallet, escrow_id: &str, refund: &str, nonce: u8) -> BindArgs {
    let nonce = wallet_fixtures::nonce(nonce);
    let message = wallet_fixtures::bind_message(
        CONTRACT,
        escrow_id,
        &wallet.label(),
        refund,
        &nonce,
        DEADLINE_MS,
    );
    BindArgs {
        escrow_id: escrow_id.to_string(),
        approver: wallet.approver(),
        refund_account: account(refund),
        nonce: wallet_fixtures::nonce_hex(&nonce),
        deadline_ms: DEADLINE_MS,
        signature: wallet.sign(&message, &nonce),
    }
}

/// Calls `bind_approver` as the relayer at `T0_MS`.
pub fn bind_with(contract: &mut VecrowEscrow, args: BindArgs) {
    set_context(RELAYER, T0_MS);
    contract.bind_approver(
        args.escrow_id,
        args.approver,
        args.refund_account,
        args.nonce,
        args.deadline_ms,
        args.signature,
    );
}

/// Binds `wallet` as the approver with `REFUND` as the refund account.
pub fn bind(contract: &mut VecrowEscrow, escrow_id: &str, wallet: &Wallet, nonce: u8) {
    bind_with(contract, sign_bind(wallet, escrow_id, REFUND, nonce));
}

/// The token contract calls `ft_on_transfer` for `sender` and `amount`;
/// returns the amount the token gives back to the sender.
pub fn transfer_call_from(
    contract: &mut VecrowEscrow,
    token: &str,
    sender: &str,
    amount: u128,
    msg: &str,
) -> u128 {
    set_context(token, T0_MS);
    match contract.ft_on_transfer(account(sender), U128(amount), msg.to_string()) {
        near_sdk::PromiseOrValue::Value(returned) => returned.0,
        near_sdk::PromiseOrValue::Promise(_) => panic!("ft_on_transfer returned a promise"),
    }
}

/// The client funds `escrow_id` with `amount` USDC base units.
pub fn fund(contract: &mut VecrowEscrow, escrow_id: &str, amount: u128) -> u128 {
    transfer_call_from(contract, USDC, CLIENT, amount, escrow_id)
}

/// Creates the default escrow (1,000 + 500 USDC, 70/30), binds `wallet` and
/// funds it with exactly the total.
pub fn funded_escrow(contract: &mut VecrowEscrow, wallet: &Wallet) -> String {
    let id = create_default(contract);
    bind(contract, &id, wallet, 200);
    assert_eq!(fund(contract, &id, usdc(1_500)), 0);
    id
}

/// The freelancer submits `milestone` at `now_ms`.
pub fn submit(contract: &mut VecrowEscrow, escrow_id: &str, milestone: u32, now_ms: u64) {
    set_context(FREELANCER, now_ms);
    contract.submit_milestone(escrow_id.to_string(), milestone);
}

/// A milestone action signed by `wallet`: `(nonce hex, signature)`.
pub fn sign_milestone(
    wallet: &Wallet,
    action: &str,
    escrow_id: &str,
    milestone: u32,
    nonce: u8,
    deadline_ms: u64,
) -> (String, String) {
    let nonce = wallet_fixtures::nonce(nonce);
    let message = wallet_fixtures::milestone_message(
        action,
        CONTRACT,
        escrow_id,
        milestone,
        &nonce,
        deadline_ms,
    );
    (
        wallet_fixtures::nonce_hex(&nonce),
        wallet.sign(&message, &nonce),
    )
}

/// The relayer forwards an approval of `milestone` signed by `wallet`.
pub fn approve(
    contract: &mut VecrowEscrow,
    escrow_id: &str,
    milestone: u32,
    wallet: &Wallet,
    nonce: u8,
    now_ms: u64,
) {
    let (nonce, signature) = sign_milestone(
        wallet,
        "approve_milestone",
        escrow_id,
        milestone,
        nonce,
        DEADLINE_MS,
    );
    set_context(RELAYER, now_ms);
    contract.approve_milestone(
        escrow_id.to_string(),
        milestone,
        nonce,
        DEADLINE_MS,
        signature,
    );
}

/// A function call scheduled by the contract in the current mocked call.
#[derive(Debug, PartialEq, Eq)]
pub struct Call {
    pub receiver: String,
    pub method: String,
    pub args: near_sdk::serde_json::Value,
    pub deposit: NearToken,
}

/// Function calls scheduled by the last contract call.
pub fn scheduled_calls() -> Vec<Call> {
    near_sdk::test_utils::get_created_receipts()
        .into_iter()
        .flat_map(|receipt| {
            let receiver = receipt.receiver_id.to_string();
            receipt
                .actions
                .into_iter()
                .filter_map(move |action| match action {
                    near_sdk::mock::MockAction::FunctionCallWeight {
                        method_name,
                        args,
                        attached_deposit,
                        ..
                    } => Some(Call {
                        receiver: receiver.clone(),
                        method: String::from_utf8(method_name).unwrap(),
                        args: near_sdk::serde_json::from_slice(&args).unwrap(),
                        deposit: attached_deposit,
                    }),
                    _ => None,
                })
        })
        .collect()
}

/// `(receiver, amount)` of each `ft_transfer` scheduled by the last call, in
/// order. Asserts that each attaches exactly 1 yoctoNEAR and is followed by
/// an `on_payout` callback for the same receiver and amount.
pub fn ft_transfers() -> Vec<(String, u128)> {
    let calls = scheduled_calls();
    let transfers: Vec<_> = calls.iter().filter(|c| c.method == "ft_transfer").collect();
    let callbacks: Vec<_> = calls.iter().filter(|c| c.method == "on_payout").collect();
    assert_eq!(
        transfers.len(),
        callbacks.len(),
        "each transfer has a callback"
    );
    transfers
        .iter()
        .zip(callbacks)
        .map(|(t, cb)| {
            assert_eq!(t.receiver, USDC);
            assert_eq!(t.deposit, NearToken::from_yoctonear(1));
            assert_eq!(cb.receiver, CONTRACT);
            assert_eq!(cb.deposit, NearToken::from_yoctonear(0));
            assert_eq!(cb.args["account_id"], t.args["receiver_id"]);
            assert_eq!(cb.args["amount"], t.args["amount"]);
            (
                t.args["receiver_id"].as_str().unwrap().to_string(),
                t.args["amount"].as_str().unwrap().parse().unwrap(),
            )
        })
        .collect()
}

/// Runs the `on_payout` callback as the contract itself, with the given
/// result of the `ft_transfer`.
pub fn payout_callback(
    contract: &mut VecrowEscrow,
    escrow_id: &str,
    account_id: &str,
    amount: u128,
    transfer_succeeded: bool,
) -> bool {
    let result = if transfer_succeeded {
        near_sdk::PromiseResult::Successful(vec![])
    } else {
        near_sdk::PromiseResult::Failed
    };
    testing_env!(
        VMContextBuilder::new()
            .current_account_id(account(CONTRACT))
            .predecessor_account_id(account(CONTRACT))
            .block_timestamp(T0_MS * 1_000_000)
            .build(),
        near_sdk::test_vm_config(),
        near_sdk::RuntimeFeesConfig::test(),
        Default::default(),
        vec![result],
    );
    contract.on_payout(escrow_id.to_string(), account(account_id), U128(amount))
}
