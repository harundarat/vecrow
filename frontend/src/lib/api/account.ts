import type { Account, WalletProof } from "./auth"
import type { Requester } from "./client"

/** Fields PATCH /me changes. An email becomes pending; "" removes the email. */
export type AccountUpdate = { username?: string; email?: string }

export function updateAccount(request: Requester, update: AccountUpdate) {
  return request<Account>("/me", { method: "PATCH", body: update })
}

/** Confirms the code emailed to the pending email. */
export function verifyAccountEmail(request: Requester, code: string) {
  return request<Account>("/me/email/verify", {
    method: "POST",
    body: { code },
  })
}

export function linkWallet(request: Requester, proof: WalletProof) {
  return request<Account>("/me/wallets", { method: "POST", body: proof })
}

export function unlinkWallet(request: Requester, walletId: string) {
  return request<void>(`/me/wallets/${encodeURIComponent(walletId)}`, {
    method: "DELETE",
  })
}
