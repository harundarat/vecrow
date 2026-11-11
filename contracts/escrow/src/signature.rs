//! Signed escrow terms and approver actions (R-01, R-04, R-06, R-17; D-02).
//!
//! Both parties sign the escrow terms (`create_escrow`): the freelancer with
//! NEP-413, the client with the approver's wallet. The approver then signs each
//! action (approve, request a revision, cancel). Every payload is plain text;
//! the contract rebuilds the exact message from the call arguments and checks
//! the signature for the signer's scheme:
//!
//! | Scheme | Wallet | Signed bytes | `signature` encoding |
//! | --- | --- | --- | --- |
//! | `nep413` | NEAR | `sha256(borsh(2^31 + 413) ‖ borsh({message, nonce, recipient, callback_url: None}))` | base64, 64 bytes |
//! | `eip191` | EVM | `keccak256("\x19Ethereum Signed Message:\n" ‖ len(message) ‖ message)` | hex (`0x` optional), 65 bytes `r‖s‖v` |
//! | `ed25519` | Solana | the raw message bytes | base58, 64 bytes |
//!
//! The byte-level format is documented in `docs/architecture.md` §5.3.

use crate::SplitShare;
use near_sdk::base64::Engine;
use near_sdk::json_types::U128;
use near_sdk::{AccountId, borsh, env, near};

/// NEP-413 tag (2^31 + 413), prepended so a signed message can never be a
/// valid transaction.
pub const NEP413_TAG: u32 = (1 << 31) + 413;

/// An approver as passed to `bind_approver` and returned by `get_escrow`,
/// tagged by `scheme`:
///
/// - `{"scheme": "nep413", "account_id": "client.near", "public_key": "ed25519:<base58>"}`
/// - `{"scheme": "eip191", "address": "0x<40 hex>"}`
/// - `{"scheme": "ed25519", "address": "<base58 public key>"}`
#[near(serializers = [json])]
#[serde(tag = "scheme", rename_all = "lowercase")]
#[derive(Clone, Debug, PartialEq, Eq)]
pub enum Approver {
    /// A NEAR wallet signing with NEP-413. The contract cannot read access
    /// keys, so `public_key` is part of the terms and every signature of the
    /// approver must be made with it (D-16).
    Nep413 {
        account_id: AccountId,
        public_key: String,
    },
    /// An EVM wallet signing with EIP-191 `personal_sign`.
    Eip191 { address: String },
    /// A Solana wallet signing raw message bytes with ed25519.
    Ed25519 { address: String },
}

/// The approver's verification key, as stored in contract state.
#[near(serializers = [borsh])]
#[derive(Clone, Debug, PartialEq, Eq)]
pub enum ApproverKey {
    Nep413 {
        account_id: AccountId,
        public_key: [u8; 32],
    },
    Eip191 {
        address: [u8; 20],
    },
    Ed25519 {
        public_key: [u8; 32],
    },
}

impl ApproverKey {
    /// Parses and validates an approver argument.
    pub fn parse(approver: &Approver) -> Result<Self, &'static str> {
        match approver {
            Approver::Nep413 {
                account_id,
                public_key,
            } => Ok(Self::Nep413 {
                account_id: account_id.clone(),
                public_key: parse_near_public_key(public_key)?,
            }),
            Approver::Eip191 { address } => {
                let hex_part = address
                    .strip_prefix("0x")
                    .ok_or("address must be 0x followed by 40 hex characters")?;
                let mut bytes = [0u8; 20];
                hex::decode_to_slice(hex_part, &mut bytes)
                    .map_err(|_| "address must be 0x followed by 40 hex characters")?;
                Ok(Self::Eip191 { address: bytes })
            }
            Approver::Ed25519 { address } => Ok(Self::Ed25519 {
                public_key: decode_base58_32(address)
                    .ok_or("address must be a base58 ed25519 public key")?,
            }),
        }
    }

    /// The canonical JSON form: lowercase EVM address, canonical base58.
    pub fn to_approver(&self) -> Approver {
        match self {
            Self::Nep413 {
                account_id,
                public_key,
            } => Approver::Nep413 {
                account_id: account_id.clone(),
                public_key: format!(
                    "ed25519:{}",
                    near_sdk::bs58::encode(public_key).into_string()
                ),
            },
            Self::Eip191 { address } => Approver::Eip191 {
                address: format!("0x{}", hex::encode(address)),
            },
            Self::Ed25519 { public_key } => Approver::Ed25519 {
                address: near_sdk::bs58::encode(public_key).into_string(),
            },
        }
    }

    /// `{scheme}:{address}` as written on the `Approver:` line of the terms,
    /// e.g. `eip191:0xab…` or `nep413:client.near`.
    pub fn label(&self) -> String {
        match self.to_approver() {
            Approver::Nep413 { account_id, .. } => format!("nep413:{account_id}"),
            Approver::Eip191 { address } => format!("eip191:{address}"),
            Approver::Ed25519 { address } => format!("ed25519:{address}"),
        }
    }

    /// NEAR account IDs controlled by the same key or name: the NEAR account,
    /// the implicit account of an ed25519 key (64 hex), or the eth-implicit
    /// account of an EVM address (`0x` + 40 hex). Used to reject an approver
    /// that is the freelancer or a split recipient under another name.
    pub fn near_account_ids(&self) -> Vec<String> {
        match self {
            Self::Nep413 {
                account_id,
                public_key,
            } => vec![account_id.to_string(), hex::encode(public_key)],
            Self::Eip191 { address } => vec![format!("0x{}", hex::encode(address))],
            Self::Ed25519 { public_key } => vec![hex::encode(public_key)],
        }
    }

    /// The ed25519 public key of a NEAR or Solana approver.
    pub fn ed25519_key(&self) -> Option<&[u8; 32]> {
        match self {
            Self::Nep413 { public_key, .. } | Self::Ed25519 { public_key } => Some(public_key),
            Self::Eip191 { .. } => None,
        }
    }

    /// Checks `signature` over `message`. `nonce` and `recipient` are the
    /// NEP-413 fields; the other schemes carry the nonce and the contract ID
    /// inside the message text. Returns an error for a malformed signature and
    /// `Ok(false)` for one that does not match.
    pub fn verify(
        &self,
        message: &str,
        nonce: &[u8; 32],
        recipient: &str,
        signature: &str,
    ) -> Result<bool, &'static str> {
        match self {
            Self::Nep413 { public_key, .. } => {
                verify_nep413(public_key, message, nonce, recipient, signature)
            }
            Self::Eip191 { address } => {
                let signature = signature.strip_prefix("0x").unwrap_or(signature);
                let mut bytes = [0u8; 65];
                hex::decode_to_slice(signature, &mut bytes)
                    .map_err(|_| "eip191 signature must be hex of 65 bytes")?;
                // Wallets return v as 27/28 (legacy) or 0/1.
                let v = match bytes[64] {
                    0 | 27 => 0,
                    1 | 28 => 1,
                    _ => return Err("eip191 signature has an invalid recovery id"),
                };
                let hash = env::keccak256_array(eip191_payload(message));
                // malleability_flag rejects high-s signatures (EIP-2).
                Ok(match env::ecrecover(&hash, &bytes[..64], v, true) {
                    Some(public_key) => env::keccak256_array(public_key)[12..] == address[..],
                    None => false,
                })
            }
            Self::Ed25519 { public_key } => {
                let signature = decode_base58_64(signature)
                    .ok_or("ed25519 signature must be base58 of 64 bytes")?;
                Ok(env::ed25519_verify(&signature, message, public_key))
            }
        }
    }
}

/// Checks a NEP-413 `signMessage` signature (base64, 64 bytes, no
/// `callbackUrl`) by `public_key`.
pub fn verify_nep413(
    public_key: &[u8; 32],
    message: &str,
    nonce: &[u8; 32],
    recipient: &str,
    signature: &str,
) -> Result<bool, &'static str> {
    let signature =
        decode_base64_64(signature).ok_or("nep413 signature must be base64 of 64 bytes")?;
    let hash = env::sha256_array(nep413_payload(message, nonce, recipient));
    Ok(env::ed25519_verify(&signature, hash, public_key))
}

/// Parses a NEAR public key `ed25519:<base58>`.
pub fn parse_near_public_key(public_key: &str) -> Result<[u8; 32], &'static str> {
    public_key
        .strip_prefix("ed25519:")
        .and_then(decode_base58_32)
        .ok_or("public_key must be ed25519:<base58>")
}

/// `borsh(NEP413_TAG) ‖ borsh(message, nonce, recipient, callback_url = None)`.
pub fn nep413_payload(message: &str, nonce: &[u8; 32], recipient: &str) -> Vec<u8> {
    #[derive(near_sdk::borsh::BorshSerialize)]
    #[borsh(crate = "near_sdk::borsh")]
    struct Payload<'a> {
        message: &'a str,
        nonce: &'a [u8; 32],
        recipient: &'a str,
        callback_url: Option<&'a str>,
    }
    let mut bytes = borsh::to_vec(&NEP413_TAG).expect("u32 serializes");
    borsh::to_writer(
        &mut bytes,
        &Payload {
            message,
            nonce,
            recipient,
            callback_url: None,
        },
    )
    .expect("payload serializes");
    bytes
}

/// `"\x19Ethereum Signed Message:\n" ‖ decimal byte length ‖ message`.
pub fn eip191_payload(message: &str) -> Vec<u8> {
    let mut bytes = format!("\x19Ethereum Signed Message:\n{}", message.len()).into_bytes();
    bytes.extend_from_slice(message.as_bytes());
    bytes
}

/// An action the approver signs after the escrow is on-chain.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum Action {
    Approve,
    RequestRevision,
    Cancel,
}

impl Action {
    /// The method name, written on the first line of the message.
    pub fn name(self) -> &'static str {
        match self {
            Self::Approve => "approve_milestone",
            Self::RequestRevision => "request_revision",
            Self::Cancel => "cancel",
        }
    }
}

/// The escrow terms both parties sign, as passed to `create_escrow`.
#[near(serializers = [json])]
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct EscrowTerms {
    /// Chosen by the backend; see `crate::is_valid_escrow_id`.
    pub escrow_id: String,
    pub freelancer: AccountId,
    /// Milestone amounts in USDC base units.
    pub milestones: Vec<U128>,
    pub split: Vec<SplitShare>,
    pub approver: Approver,
    pub refund_account: AccountId,
    pub fee_bps: u16,
    /// The terms can be put on-chain until this time (ms since the Unix epoch).
    pub deadline_ms: u64,
}

/// The freelancer's NEP-413 signature of the terms, as returned by the NEAR
/// wallet's `signMessage`.
#[near(serializers = [json])]
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct FreelancerSignature {
    /// `ed25519:<base58>`: the key the freelancer signed with.
    pub public_key: String,
    /// base64, 64 bytes.
    pub signature: String,
}

/// The message both parties sign for `create_escrow`. Lines are separated by
/// `\n`, with no trailing newline.
pub fn terms_message(
    contract_id: &AccountId,
    terms: &EscrowTerms,
    approver: &ApproverKey,
) -> String {
    let milestones: Vec<String> = terms.milestones.iter().map(|m| m.0.to_string()).collect();
    let split: Vec<String> = terms
        .split
        .iter()
        .map(|s| format!("{}:{}", s.account_id, s.bps))
        .collect();
    format!(
        "Vecrow create_escrow\nContract: {contract_id}\nEscrow: {}\nFreelancer: {}\nMilestones: {}\nSplit: {}\nApprover: {}\nRefund account: {}\nFee: {} bps\nDeadline: {}",
        terms.escrow_id,
        terms.freelancer,
        milestones.join(","),
        split.join(","),
        approver.label(),
        terms.refund_account,
        terms.fee_bps,
        terms.deadline_ms,
    )
}

/// The NEP-413 nonce of a terms signature: `sha256(message)`. The terms have
/// no nonce of their own; a new escrow ID makes them unique.
pub fn terms_nonce(message: &str) -> [u8; 32] {
    env::sha256_array(message.as_bytes())
}

/// The message the approver signs for an action. `milestone` is written for
/// `approve_milestone` and `request_revision` and omitted for `cancel`.
pub fn action_message(
    action: Action,
    contract_id: &AccountId,
    escrow_id: &str,
    milestone: Option<u32>,
    nonce: &[u8; 32],
    deadline_ms: u64,
) -> String {
    let milestone = milestone
        .map(|m| format!("Milestone: {m}\n"))
        .unwrap_or_default();
    format!(
        "Vecrow {}\nContract: {contract_id}\nEscrow: {escrow_id}\n{milestone}Nonce: {}\nDeadline: {deadline_ms}",
        action.name(),
        hex::encode(nonce),
    )
}

/// Parses a nonce: exactly 64 lowercase hex characters (32 bytes), so the
/// `Nonce:` line has a single spelling.
pub fn parse_nonce(nonce: &str) -> Result<[u8; 32], &'static str> {
    let mut bytes = [0u8; 32];
    let lowercase = nonce
        .bytes()
        .all(|b| matches!(b, b'0'..=b'9' | b'a'..=b'f'));
    if !lowercase || hex::decode_to_slice(nonce, &mut bytes).is_err() {
        return Err("nonce must be 64 lowercase hex characters");
    }
    Ok(bytes)
}

fn decode_base58_32(s: &str) -> Option<[u8; 32]> {
    near_sdk::bs58::decode(s).into_vec().ok()?.try_into().ok()
}

fn decode_base58_64(s: &str) -> Option<[u8; 64]> {
    near_sdk::bs58::decode(s).into_vec().ok()?.try_into().ok()
}

fn decode_base64_64(s: &str) -> Option<[u8; 64]> {
    near_sdk::base64::engine::general_purpose::STANDARD
        .decode(s)
        .ok()?
        .try_into()
        .ok()
}
