//! Approver identities and signed actions (R-04, R-06, R-17; D-02).
//!
//! The approver signs a plain-text message with the wallet they already own.
//! The contract rebuilds the exact message from the call arguments and checks
//! the signature for the approver's scheme:
//!
//! | Scheme | Wallet | Signed bytes | `signature` encoding |
//! | --- | --- | --- | --- |
//! | `nep413` | NEAR | `sha256(borsh(2^31 + 413) ‖ borsh({message, nonce, recipient, callback_url: None}))` | base64, 64 bytes |
//! | `eip191` | EVM | `keccak256("\x19Ethereum Signed Message:\n" ‖ len(message) ‖ message)` | hex (`0x` optional), 65 bytes `r‖s‖v` |
//! | `ed25519` | Solana | the raw message bytes | base58, 64 bytes |
//!
//! The byte-level format is documented in `docs/architecture.md` §5.3.

use near_sdk::base64::Engine;
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
    /// keys, so `public_key` is bound here and every later signature must be
    /// made with it (D-15).
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
            } => {
                let encoded = public_key
                    .strip_prefix("ed25519:")
                    .ok_or("public_key must be ed25519:<base58>")?;
                Ok(Self::Nep413 {
                    account_id: account_id.clone(),
                    public_key: decode_base58_32(encoded)
                        .ok_or("public_key must be ed25519:<base58>")?,
                })
            }
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

    /// `{scheme}:{address}` as written on the `Approver:` line of the bind
    /// message, e.g. `eip191:0xab…` or `nep413:client.near`.
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
                let signature = decode_base64_64(signature)
                    .ok_or("nep413 signature must be base64 of 64 bytes")?;
                let hash = env::sha256_array(nep413_payload(message, nonce, recipient));
                Ok(env::ed25519_verify(&signature, hash, public_key))
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

/// The action an approver signs for a milestone.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum MilestoneAction {
    Approve,
    RequestRevision,
}

impl MilestoneAction {
    /// The method name, written on the first line of the message.
    pub fn name(self) -> &'static str {
        match self {
            Self::Approve => "approve_milestone",
            Self::RequestRevision => "request_revision",
        }
    }
}

/// The message signed to bind an approver. Lines are separated by `\n`, with
/// no trailing newline.
pub fn bind_message(
    contract_id: &AccountId,
    escrow_id: u64,
    approver: &ApproverKey,
    refund_account: &AccountId,
    nonce: &[u8; 32],
    deadline_ms: u64,
) -> String {
    format!(
        "Vecrow bind_approver\nContract: {contract_id}\nEscrow: {escrow_id}\nApprover: {}\nRefund account: {refund_account}\nNonce: {}\nDeadline: {deadline_ms}",
        approver.label(),
        hex::encode(nonce),
    )
}

/// The message signed to approve a milestone or request a revision.
pub fn milestone_message(
    action: MilestoneAction,
    contract_id: &AccountId,
    escrow_id: u64,
    milestone: u32,
    nonce: &[u8; 32],
    deadline_ms: u64,
) -> String {
    format!(
        "Vecrow {}\nContract: {contract_id}\nEscrow: {escrow_id}\nMilestone: {milestone}\nNonce: {}\nDeadline: {deadline_ms}",
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
