use base64::Engine;
use borsh::BorshSerialize;
use ed25519_dalek::{Signer, SigningKey};
use sha2::{Digest, Sha256};
use sha3::Keccak256;

pub const NEP413_TAG: u32 = (1 << 31) + 413;

pub fn nonce(byte: u8) -> [u8; 32] {
    [byte; 32]
}

pub fn nonce_hex(nonce: &[u8; 32]) -> String {
    hex::encode(nonce)
}

#[derive(Clone, Debug)]
pub struct Terms {
    pub contract_id: String,
    pub escrow_id: String,
    pub freelancer: String,
    pub milestones: Vec<u128>,
    pub split: Vec<(String, u16)>,
    pub approver: String,
    pub refund_account: String,
    pub fee_bps: u16,
    pub deadline_ms: u64,
}

pub fn terms_message(t: &Terms) -> String {
    let milestones: Vec<String> = t.milestones.iter().map(u128::to_string).collect();
    let split: Vec<String> = t
        .split
        .iter()
        .map(|(account, bps)| format!("{account}:{bps}"))
        .collect();
    format!(
        "Vecrow create_escrow\nContract: {}\nEscrow: {}\nFreelancer: {}\nMilestones: {}\nSplit: {}\nApprover: {}\nRefund account: {}\nFee: {} bps\nDeadline: {}",
        t.contract_id,
        t.escrow_id,
        t.freelancer,
        milestones.join(","),
        split.join(","),
        t.approver,
        t.refund_account,
        t.fee_bps,
        t.deadline_ms
    )
}

pub fn terms_nonce(message: &str) -> [u8; 32] {
    Sha256::digest(message.as_bytes()).into()
}

pub fn cancel_message(
    contract_id: &str,
    escrow_id: &str,
    nonce: &[u8; 32],
    deadline_ms: u64,
) -> String {
    format!(
        "Vecrow cancel\nContract: {contract_id}\nEscrow: {escrow_id}\nNonce: {}\nDeadline: {deadline_ms}",
        nonce_hex(nonce)
    )
}

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
    pub fn new(account_id: &str, seed: u8) -> Self {
        Self {
            account_id: account_id.to_string(),
            key: SigningKey::from_bytes(&[seed; 32]),
        }
    }

    pub fn account_id(&self) -> &str {
        &self.account_id
    }

    pub fn public_key(&self) -> String {
        format!(
            "ed25519:{}",
            bs58::encode(self.key.verifying_key().as_bytes()).into_string()
        )
    }

    pub fn secret_key(&self) -> String {
        let mut bytes = self.key.to_bytes().to_vec();
        bytes.extend_from_slice(self.key.verifying_key().as_bytes());
        format!("ed25519:{}", bs58::encode(bytes).into_string())
    }

    pub fn implicit_account_id(&self) -> String {
        hex::encode(self.key.verifying_key().as_bytes())
    }

    pub fn approver_label(&self) -> String {
        format!("nep413:{}", self.account_id)
    }

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

pub struct EvmWallet {
    key: k256::ecdsa::SigningKey,
}

impl EvmWallet {
    pub fn new(seed: u8) -> Self {
        Self::from_secret([seed; 32])
    }

    pub fn from_secret(secret: [u8; 32]) -> Self {
        Self {
            key: k256::ecdsa::SigningKey::from_bytes(&secret.into()).expect("valid scalar"),
        }
    }

    pub fn address(&self) -> String {
        let point = self.key.verifying_key().to_encoded_point(false);
        let hash = Keccak256::digest(&point.as_bytes()[1..]);
        format!("0x{}", hex::encode(&hash[12..]))
    }

    pub fn approver_label(&self) -> String {
        format!("eip191:{}", self.address())
    }

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

pub struct SolanaWallet {
    key: SigningKey,
}

impl SolanaWallet {
    pub fn new(seed: u8) -> Self {
        Self {
            key: SigningKey::from_bytes(&[seed; 32]),
        }
    }

    pub fn address(&self) -> String {
        bs58::encode(self.key.verifying_key().as_bytes()).into_string()
    }

    pub fn implicit_account_id(&self) -> String {
        hex::encode(self.key.verifying_key().as_bytes())
    }

    pub fn approver_label(&self) -> String {
        format!("ed25519:{}", self.address())
    }

    pub fn sign_message(&self, message: &str) -> String {
        bs58::encode(self.key.sign(message.as_bytes()).to_bytes()).into_string()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn evm_address_matches_known_vector() {
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
    fn terms_follow_the_expected_format() {
        let terms = Terms {
            contract_id: "escrow.near".into(),
            escrow_id: "e-1".into(),
            freelancer: "alice.near".into(),
            milestones: vec![1_000_000_000, 500_000_000],
            split: vec![("alice.near".into(), 7_000), ("bob.near".into(), 3_000)],
            approver: "eip191:0xab".into(),
            refund_account: "carol.near".into(),
            fee_bps: 50,
            deadline_ms: 1_700_000_000_000,
        };
        assert_eq!(
            terms_message(&terms),
            "Vecrow create_escrow\nContract: escrow.near\nEscrow: e-1\nFreelancer: alice.near\nMilestones: 1000000000,500000000\nSplit: alice.near:7000,bob.near:3000\nApprover: eip191:0xab\nRefund account: carol.near\nFee: 50 bps\nDeadline: 1700000000000"
        );
    }

    #[test]
    fn messages_follow_the_expected_format() {
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
