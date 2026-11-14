mod approve;
mod auto_release;
mod cancel;
mod create;
mod funding;
mod payout;
mod refund;
mod retry;
mod revision;
mod signature;
mod split;
mod state;
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

pub const T0_MS: u64 = 1_700_000_000_000;
pub const FEE_BPS: u16 = 50;
pub const WINDOW_SEC: u64 = 14 * 24 * 60 * 60;

pub fn account(id: &str) -> AccountId {
    id.parse().unwrap()
}

pub fn usdc(whole: u128) -> u128 {
    whole * 1_000_000
}

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
    near_sdk::env::set_blockchain_interface(near_sdk::MockedBlockchain::new(
        VMContextBuilder::new()
            .current_account_id(account(CONTRACT))
            .build(),
        near_sdk::test_vm_config(),
        near_sdk::RuntimeFeesConfig::test(),
        vec![],
        Default::default(),
        Default::default(),
        None,
    ));
    set_context(CONTRACT, T0_MS);
    VecrowEscrow::new(account(USDC), account(TREASURY), FEE_BPS, WINDOW_SEC)
}

pub fn share(id: &str, bps: u16) -> SplitShare {
    SplitShare {
        account_id: account(id),
        bps,
    }
}

pub fn split_70_30() -> Vec<SplitShare> {
    vec![share(FREELANCER, 7_000), share(MEMBER, 3_000)]
}

pub fn freelancer_wallet() -> NearWallet {
    NearWallet::new(FREELANCER, 5)
}

pub fn next_escrow_id(contract: &VecrowEscrow) -> String {
    (1..)
        .map(|n| format!("e{n}"))
        .find(|id| !contract.escrows.contains_key(id))
        .unwrap()
}

pub fn default_terms(escrow_id: &str, wallet: &Wallet) -> EscrowTerms {
    EscrowTerms {
        escrow_id: escrow_id.to_string(),
        freelancer: account(FREELANCER),
        milestones: vec![U128(usdc(1_000)), U128(usdc(500))],
        split: split_70_30(),
        approver: wallet.approver(),
        refund_account: account(REFUND),
        fee_bps: FEE_BPS,
        deadline_ms: DEADLINE_MS,
    }
}

pub struct SignedTerms {
    pub terms: EscrowTerms,
    pub freelancer_signature: FreelancerSignature,
    pub client_signature: String,
}

pub fn terms_message(terms: &EscrowTerms, approver: &str) -> String {
    wallet_fixtures::terms_message(&wallet_fixtures::Terms {
        contract_id: CONTRACT.to_string(),
        escrow_id: terms.escrow_id.clone(),
        freelancer: terms.freelancer.to_string(),
        milestones: terms.milestones.iter().map(|m| m.0).collect(),
        split: terms
            .split
            .iter()
            .map(|s| (s.account_id.to_string(), s.bps))
            .collect(),
        approver: approver.to_string(),
        refund_account: terms.refund_account.to_string(),
        fee_bps: terms.fee_bps,
        deadline_ms: terms.deadline_ms,
    })
}

pub fn sign_terms(terms: EscrowTerms, freelancer: &NearWallet, client: &Wallet) -> SignedTerms {
    let message = terms_message(&terms, &client.label());
    let nonce = wallet_fixtures::terms_nonce(&message);
    SignedTerms {
        freelancer_signature: FreelancerSignature {
            public_key: freelancer.public_key(),
            signature: freelancer.sign_message(&message, &nonce, CONTRACT),
        },
        client_signature: client.sign(&message, &nonce),
        terms,
    }
}

pub fn create_with(contract: &mut VecrowEscrow, signed: SignedTerms) {
    set_context(RELAYER, T0_MS);
    contract.create_escrow(
        signed.terms,
        signed.freelancer_signature,
        signed.client_signature,
    );
}

pub fn create_escrow_with(contract: &mut VecrowEscrow, wallet: &Wallet) -> String {
    let id = next_escrow_id(contract);
    create_with(
        contract,
        sign_terms(default_terms(&id, wallet), &freelancer_wallet(), wallet),
    );
    id
}

pub fn create_default(contract: &mut VecrowEscrow) -> String {
    create_escrow_with(contract, &Wallet::near())
}

pub fn view(contract: &VecrowEscrow, escrow_id: &str) -> EscrowView {
    contract
        .get_escrow(escrow_id.to_string())
        .expect("escrow exists")
}

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

    pub fn label(&self) -> String {
        match self {
            Self::Near(w) => w.approver_label(),
            Self::Evm(w) => w.approver_label(),
            Self::Solana(w) => w.approver_label(),
        }
    }

    pub fn sign(&self, message: &str, nonce: &[u8; 32]) -> String {
        match self {
            Self::Near(w) => w.sign_message(message, nonce, CONTRACT),
            Self::Evm(w) => w.personal_sign(message),
            Self::Solana(w) => w.sign_message(message),
        }
    }
}

pub const DEADLINE_MS: u64 = T0_MS + 3_600_000;

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

pub fn fund(contract: &mut VecrowEscrow, escrow_id: &str, amount: u128) -> u128 {
    transfer_call_from(contract, USDC, CLIENT, amount, escrow_id)
}

pub fn funded_escrow(contract: &mut VecrowEscrow, wallet: &Wallet) -> String {
    let id = create_escrow_with(contract, wallet);
    assert_eq!(fund(contract, &id, usdc(1_500)), 0);
    id
}

pub fn submit(contract: &mut VecrowEscrow, escrow_id: &str, milestone: u32, now_ms: u64) {
    set_context(FREELANCER, now_ms);
    contract.submit_milestone(escrow_id.to_string(), milestone);
}

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

#[derive(Debug, PartialEq, Eq)]
pub struct Call {
    pub receiver: String,
    pub method: String,
    pub args: near_sdk::serde_json::Value,
    pub deposit: NearToken,
}

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

pub fn request_revision(
    contract: &mut VecrowEscrow,
    escrow_id: &str,
    milestone: u32,
    wallet: &Wallet,
    nonce: u8,
    now_ms: u64,
) {
    let (nonce, signature) = sign_milestone(
        wallet,
        "request_revision",
        escrow_id,
        milestone,
        nonce,
        DEADLINE_MS,
    );
    set_context(RELAYER, now_ms);
    contract.request_revision(
        escrow_id.to_string(),
        milestone,
        nonce,
        DEADLINE_MS,
        signature,
    );
}

pub fn auto_release(contract: &mut VecrowEscrow, escrow_id: &str, milestone: u32, now_ms: u64) {
    set_context(STRANGER, now_ms);
    contract.auto_release(escrow_id.to_string(), milestone);
}

pub fn refund(contract: &mut VecrowEscrow, caller: &str, escrow_id: &str, milestone: Option<u32>) {
    set_context(caller, T0_MS);
    contract.refund_unsubmitted(escrow_id.to_string(), milestone);
}

pub fn sign_cancel(
    wallet: &Wallet,
    escrow_id: &str,
    nonce: u8,
    deadline_ms: u64,
) -> (String, String) {
    let nonce = wallet_fixtures::nonce(nonce);
    let message = wallet_fixtures::cancel_message(CONTRACT, escrow_id, &nonce, deadline_ms);
    (
        wallet_fixtures::nonce_hex(&nonce),
        wallet.sign(&message, &nonce),
    )
}

pub fn cancel_signed(
    contract: &mut VecrowEscrow,
    caller: &str,
    escrow_id: &str,
    wallet: &Wallet,
    nonce: u8,
    now_ms: u64,
) {
    let (nonce, signature) = sign_cancel(wallet, escrow_id, nonce, DEADLINE_MS);
    set_context(caller, now_ms);
    contract.cancel(
        escrow_id.to_string(),
        Some(nonce),
        Some(DEADLINE_MS),
        Some(signature),
    );
}
