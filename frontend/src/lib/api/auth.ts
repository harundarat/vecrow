import { apiFetch } from "./client"

export type WalletScheme = "eip191" | "ed25519" | "nep413"

export type Wallet = {
  id: string
  scheme: WalletScheme
  address: string
  created_at: string
}

export type Account = {
  id: string
  username: string
  email: string | null
  pending_email: string | null
  created_at: string
  wallets: Wallet[]
}

export type NonceResponse = {
  nonce: string
  message: string
  recipient: string
  expires_at: string
}

/** A wallet's signature over the message of a nonce. */
export type WalletProof = {
  nonce: string
  signature: string
  public_key?: string
}

export type VerifyResponse = {
  token: string
  expires_at: string
  new_account: boolean
  account: Account
}

/**
 * The part of a challenge a wallet signs. `nonce` is hex and `recipient` a
 * plain string; only NEP-413 uses them, but every challenge carries both.
 */
export type SignRequest = Pick<NonceResponse, "message" | "nonce" | "recipient">

export type WalletSignature = Omit<WalletProof, "nonce">

/** A connected wallet that can sign the backend's challenges. */
export type ChallengeSigner = {
  scheme: WalletScheme
  address: string
  sign: (request: SignRequest) => Promise<WalletSignature>
}

export type EmailCodeResponse = {
  message: string
  expires_in_seconds: number
}

export type EmailVerifyResponse = {
  token: string
  expires_at: string
  recovery: boolean
  account: Account
}

export function issueNonce(scheme: WalletScheme, address: string) {
  return apiFetch<NonceResponse>("/auth/nonce", {
    method: "POST",
    body: { scheme, address },
  })
}

export function verifyProof(proof: WalletProof) {
  return apiFetch<VerifyResponse>("/auth/verify", {
    method: "POST",
    body: proof,
  })
}

export function logout(token: string) {
  return apiFetch<void>("/auth/logout", { method: "POST", token })
}

export function getMe(token: string, signal?: AbortSignal) {
  return apiFetch<Account>("/me", { token, signal })
}

/**
 * Gets a challenge for the signer's wallet and has the wallet sign it. The
 * proof works for both POST /auth/verify and POST /me/wallets.
 */
export async function proveWallet(
  signer: ChallengeSigner
): Promise<WalletProof> {
  const { message, nonce, recipient } = await issueNonce(
    signer.scheme,
    signer.address
  )
  const signature = await signer.sign({ message, nonce, recipient })
  return { nonce, ...signature }
}

export function requestEmailCode(email: string) {
  return apiFetch<EmailCodeResponse>("/auth/email/otp", {
    method: "POST",
    body: { email },
  })
}

export function verifyEmailCode(email: string, code: string) {
  return apiFetch<EmailVerifyResponse>("/auth/email/verify", {
    method: "POST",
    body: { email, code },
  })
}
