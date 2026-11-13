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
