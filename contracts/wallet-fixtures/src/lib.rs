//! Test-only wallet signers for the Vecrow contract tests.
//!
//! Each signer reproduces, byte for byte, what the corresponding wallet signs
//! and returns, using only standard libraries (`ed25519-dalek`, `k256`,
//! `borsh`, `sha2`, `sha3`). None of this code is shared with the contract, so
//! a fixture that verifies proves the contract accepts real wallet output:
//!
//! | Wallet | Call | Signed bytes | Returned signature |
//! | --- | --- | --- | --- |
//! | NEAR (NEP-413) | `signMessage({message, nonce, recipient})` | `sha256(borsh(tag) ‖ borsh(payload))` | base64, 64 bytes |
//! | MetaMask (EIP-191) | `personal_sign(message)` | `keccak256("\x19Ethereum Signed Message:\n" ‖ len ‖ message)` | `0x` hex, 65 bytes `r‖s‖v`, `v` = 27/28 |
//! | Phantom (Solana) | `signMessage(utf8 bytes)` | raw message bytes | base58, 64 bytes |
//!
//! The message builders below follow the Vecrow signed-action format in
//! `docs/architecture.md` §5.3 and are written independently of the contract's
//! own builder.

use base64::Engine;
use borsh::BorshSerialize;
use ed25519_dalek::{Signer, SigningKey};
use sha2::{Digest, Sha256};
use sha3::Keccak256;

/// NEP-413 tag: 2^31 + 413, Borsh-serialised as a little-endian `u32` before
/// the payload so a signed message can never be a valid transaction.
pub const NEP413_TAG: u32 = (1 << 31) + 413;

/// Returns a 32-byte nonce filled with `byte`.
pub fn nonce(byte: u8) -> [u8; 32] {
    [byte; 32]
}

/// Lowercase hex of a nonce, as passed to the contract and written in messages.
pub fn nonce_hex(nonce: &[u8; 32]) -> String {
    hex::encode(nonce)
}

/// Message signed to bind an approver (`bind_approver`).
pub fn bind_message(
    contract_id: &str,
    escrow_id: &str,
    approver: &str,
    refund_account: &str,
    nonce: &[u8; 32],
    deadline_ms: u64,
) -> String {
    format!(
        "Vecrow bind_approver\nContract: {contract_id}\nEscrow: {escrow_id}\nApprover: {approver}\nRefund account: {refund_account}\nNonce: {}\nDeadline: {deadline_ms}",
        nonce_hex(nonce)
    )
}

/// Message signed for a milestone action: `approve_milestone` or
/// `request_revision`.
pub fn milestone_message(
    action: &str,
    contract_id: &str,
    escrow_id: &str,
    milestone: u32,
    nonce: &[u8; 32],
    deadline_ms: u64,
) -> String {
    format!(
        "Vecrow {action}\nContract: {contract_id}\nEscrow: {escrow_id}\nMilestone: {milestone}\nNonce: {}\nDeadline: {deadline_ms}",
        nonce_hex(nonce)
    )
}

/// A NEAR wallet key that signs with NEP-413 `signMessage`.
pub struct NearWallet {
    account_id: String,
    key: SigningKey,
}

#[derive(BorshSerialize)]
struct Nep413Payload {
    message: String,
    nonce: [u8; 32],
    recipient: String,
    callback_url: Option<String>,
}

impl NearWallet {
    /// A deterministic key derived from `seed`, owned by `account_id`.
    pub fn new(account_id: &str, seed: u8) -> Self {
        Self {
            account_id: account_id.to_string(),
            key: SigningKey::from_bytes(&[seed; 32]),
        }
    }

    pub fn account_id(&self) -> &str {
        &self.account_id
    }

    /// The public key in NEAR format: `ed25519:<base58>`.
    pub fn public_key(&self) -> String {
        format!(
            "ed25519:{}",
            bs58::encode(self.key.verifying_key().as_bytes()).into_string()
        )
    }

    /// The implicit account ID of this key: lowercase hex of the public key.
    pub fn implicit_account_id(&self) -> String {
        hex::encode(self.key.verifying_key().as_bytes())
    }

    /// The `approver` argument of `bind_approver` for this wallet.
    pub fn approver_label(&self) -> String {
        format!("nep413:{}", self.account_id)
    }

    /// NEP-413 `signMessage({message, nonce, recipient})` without a
    /// `callbackUrl`. Returns the base64 signature.
    pub fn sign_message(&self, message: &str, nonce: &[u8; 32], recipient: &str) -> String {
        let mut bytes = borsh::to_vec(&NEP413_TAG).expect("borsh u32");
        bytes.extend(
            borsh::to_vec(&Nep413Payload {
                message: message.to_string(),
                nonce: *nonce,
                recipient: recipient.to_string(),
                callback_url: None,
            })
            .expect("borsh payload"),
        );
        let hash = Sha256::digest(&bytes);
        let signature = self.key.sign(&hash);
        base64::engine::general_purpose::STANDARD.encode(signature.to_bytes())
    }
}

/// An EVM wallet key that signs with EIP-191 `personal_sign`, as MetaMask does.
pub struct EvmWallet {
    key: k256::ecdsa::SigningKey,
}

impl EvmWallet {
    /// A deterministic key derived from `seed` (must be non-zero).
    pub fn new(seed: u8) -> Self {
        Self::from_secret([seed; 32])
    }

    /// A key from a raw 32-byte secret scalar.
    pub fn from_secret(secret: [u8; 32]) -> Self {
        Self {
            key: k256::ecdsa::SigningKey::from_bytes(&secret.into()).expect("valid scalar"),
        }
    }

    /// The address: `0x` + lowercase hex of the last 20 bytes of
    /// `keccak256(uncompressed public key without the 0x04 prefix)`.
    pub fn address(&self) -> String {
        let point = self.key.verifying_key().to_encoded_point(false);
        let hash = Keccak256::digest(&point.as_bytes()[1..]);
        format!("0x{}", hex::encode(&hash[12..]))
    }

    /// The `approver` argument of `bind_approver` for this wallet.
    pub fn approver_label(&self) -> String {
        format!("eip191:{}", self.address())
    }

    /// EIP-191 `personal_sign(message)`. Returns `0x` + hex of `r‖s‖v` with
    /// `v` = 27 or 28, like MetaMask.
    pub fn personal_sign(&self, message: &str) -> String {
        let mut prefixed = format!("\x19Ethereum Signed Message:\n{}", message.len()).into_bytes();
        prefixed.extend_from_slice(message.as_bytes());
        let hash = Keccak256::digest(&prefixed);
        let (signature, recovery_id) = self
            .key
            .sign_prehash_recoverable(&hash)
            .expect("sign prehash");
        let mut bytes = signature.to_bytes().to_vec();
        bytes.push(27 + recovery_id.to_byte());
        format!("0x{}", hex::encode(bytes))
    }
}

/// Turns a low-s EIP-191 signature into its high-s twin (`s` → `n − s`,
/// recovery bit flipped). Both recover the same address, but EIP-2 forbids
/// high-s signatures and wallets never produce them.
pub fn malleate_evm_signature(signature: &str) -> String {
    let bytes = hex::decode(signature.trim_start_matches("0x")).expect("hex signature");
    let sig = k256::ecdsa::Signature::from_slice(&bytes[..64]).expect("r and s");
    let (r, s) = sig.split_scalars();
    let high = k256::ecdsa::Signature::from_scalars(r.to_bytes(), (-*s).to_bytes())
        .expect("valid scalars");
    let mut out = high.to_bytes().to_vec();
    out.push(if bytes[64] == 27 { 28 } else { 27 });
    format!("0x{}", hex::encode(out))
}

/// A Solana wallet key that signs raw message bytes, as Phantom's
/// `signMessage` does.
pub struct SolanaWallet {
    key: SigningKey,
}

impl SolanaWallet {
    /// A deterministic key derived from `seed`.
    pub fn new(seed: u8) -> Self {
        Self {
            key: SigningKey::from_bytes(&[seed; 32]),
        }
    }

    /// The address: base58 of the ed25519 public key.
    pub fn address(&self) -> String {
        bs58::encode(self.key.verifying_key().as_bytes()).into_string()
    }

    /// The NEAR implicit account ID of the same key: lowercase hex.
    pub fn implicit_account_id(&self) -> String {
        hex::encode(self.key.verifying_key().as_bytes())
    }

    /// The `approver` argument of `bind_approver` for this wallet.
    pub fn approver_label(&self) -> String {
        format!("ed25519:{}", self.address())
    }

    /// `signMessage(utf8 bytes of message)`. Returns the base58 signature.
    pub fn sign_message(&self, message: &str) -> String {
        bs58::encode(self.key.sign(message.as_bytes()).to_bytes()).into_string()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn evm_address_matches_known_vector() {
        // Private key 1 is a well-known test vector.
        let mut secret = [0u8; 32];
        secret[31] = 1;
        assert_eq!(
            EvmWallet::from_secret(secret).address(),
            "0x7e5f4552091a69125d5dfcb7b8c2659029395bdf"
        );
    }

    #[test]
    fn evm_signature_has_legacy_v() {
        let sig = EvmWallet::new(7).personal_sign("hello");
        let bytes = hex::decode(sig.trim_start_matches("0x")).unwrap();
        assert_eq!(bytes.len(), 65);
        assert!(bytes[64] == 27 || bytes[64] == 28);
    }

    #[test]
    fn nep413_tag_is_borsh_little_endian() {
        assert_eq!(
            borsh::to_vec(&NEP413_TAG).unwrap(),
            vec![0x9d, 0x01, 0x00, 0x80]
        );
    }

    #[test]
    fn messages_follow_the_documented_format() {
        let n = nonce(0xab);
        assert_eq!(
            milestone_message(
                "approve_milestone",
                "escrow.test.near",
                "7",
                1,
                &n,
                1_700_000_000_000
            ),
            format!(
                "Vecrow approve_milestone\nContract: escrow.test.near\nEscrow: 7\nMilestone: 1\nNonce: {}\nDeadline: 1700000000000",
                "ab".repeat(32)
            )
        );
    }
}
