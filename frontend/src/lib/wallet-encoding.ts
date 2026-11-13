import {
  decodeBase58,
  decodeBase64,
  encodeBase58,
  encodeBase64,
  getBytes,
  hexlify,
  isHexString,
  sha256,
  toBeArray,
  toUtf8Bytes,
  zeroPadValue,
} from "ethers"

const SIGNATURE_BYTES = 64
const EVM_SIGNATURE_BYTES = 65
const ED25519_KEY_BYTES = 32
const NEP413_NONCE_BYTES = 32

const BASE58 = /^[1-9A-HJ-NP-Za-km-z]+$/
const BASE64 = /^[A-Za-z0-9+/]+={0,2}$/

/** Decodes base58 into exactly `size` bytes, keeping leading zero bytes. */
export function base58ToBytes(value: string, size: number): Uint8Array {
  if (!BASE58.test(value)) throw new Error("Invalid base58 string")
  const leadingZeros = value.length - value.replace(/^1+/, "").length
  const n = decodeBase58(value)
  const body = n === BigInt(0) ? new Uint8Array() : toBeArray(n)
  if (leadingZeros + body.length !== size) {
    throw new Error(`Expected ${size} bytes of base58`)
  }
  return getBytes(zeroPadValue(body, size))
}

export function bytesToBase58(bytes: Uint8Array): string {
  return encodeBase58(bytes)
}

export function bytesToBase64(bytes: Uint8Array): string {
  return encodeBase64(bytes)
}

export function base64ToBytes(value: string): Uint8Array {
  if (!BASE64.test(value) || value.length % 4 !== 0) {
    throw new Error("Invalid base64 string")
  }
  return decodeBase64(value)
}

/** `personal_sign` result as the backend expects it: `0x` + 65 bytes, lowercase. */
export function toEip191Signature(signature: string): string {
  const hex = signature.startsWith("0x") ? signature : `0x${signature}`
  if (!isHexString(hex, EVM_SIGNATURE_BYTES)) {
    throw new Error("EVM signature must be 65 bytes of hex")
  }
  return hex.toLowerCase()
}

/** Solana `signMessage` bytes as base58. */
export function toSolanaSignature(signature: Uint8Array): string {
  if (signature.length !== SIGNATURE_BYTES) {
    throw new Error("Solana signature must be 64 bytes")
  }
  return bytesToBase58(signature)
}

/** The NEP-413 nonce is the backend's hex nonce decoded to 32 bytes. */
export function nep413Nonce(hexNonce: string): Uint8Array {
  const hex = hexNonce.startsWith("0x") ? hexNonce : `0x${hexNonce}`
  if (!isHexString(hex, NEP413_NONCE_BYTES)) {
    throw new Error("NEP-413 nonce must be 32 bytes of hex")
  }
  return getBytes(hex)
}

/** The NEP-413 nonce of signed terms: sha256 of their text, as lowercase hex. */
export function termsNonce(message: string): string {
  return sha256(toUtf8Bytes(message)).slice(2)
}

export type Nep413Proof = { signature: string; public_key: string }

/**
 * Normalizes a NEP-413 `signMessage` result into base64 signature and
 * `ed25519:<base58>` public key. Wallets disagree on the signature encoding:
 * the standard says base64, but some return base58, with or without an
 * `ed25519:` prefix.
 */
export function toNep413Proof(signed: {
  signature: string
  publicKey: string
}): Nep413Proof {
  return {
    signature: bytesToBase64(nep413SignatureBytes(signed.signature)),
    public_key: toNearPublicKey(signed.publicKey),
  }
}

function nep413SignatureBytes(signature: string): Uint8Array {
  if (signature.startsWith("ed25519:")) {
    return base58ToBytes(signature.slice("ed25519:".length), SIGNATURE_BYTES)
  }
  // A 64-byte value is 88 characters in base64 and at most 88 in base58,
  // so try the standard encoding first.
  try {
    const bytes = base64ToBytes(signature)
    if (bytes.length === SIGNATURE_BYTES) return bytes
  } catch {}
  return base58ToBytes(signature, SIGNATURE_BYTES)
}

export function toNearPublicKey(publicKey: string): string {
  const key = publicKey.startsWith("ed25519:")
    ? publicKey.slice("ed25519:".length)
    : publicKey
  base58ToBytes(key, ED25519_KEY_BYTES)
  return `ed25519:${key}`
}

export function bytesToHex(bytes: Uint8Array): string {
  return hexlify(bytes)
}
