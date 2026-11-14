import type { WalletScheme } from "./api/auth"
import type { WalletRef } from "./api/escrow"

/** What a visitor keeps after joining an escrow without an account. */
export type GuestAccess = {
  /** Empty once the server refused the token; renew it with the wallet. */
  token: string
  expires_at: string
  /** The wallet that submitted, as the backend stored it. */
  wallet: WalletRef
}

const SCHEMES: WalletScheme[] = ["nep413", "eip191", "ed25519"]

export function guestAccessKey(escrowId: string) {
  return `vecrow.guest.${escrowId}`
}

export function parseGuestAccess(raw: string | null): GuestAccess | null {
  if (!raw) return null
  try {
    const value = JSON.parse(raw) as Partial<GuestAccess> | null
    const wallet = value?.wallet
    if (
      !wallet ||
      typeof wallet.address !== "string" ||
      !SCHEMES.includes(wallet.scheme)
    ) {
      return null
    }
    return {
      token: typeof value.token === "string" ? value.token : "",
      expires_at: typeof value.expires_at === "string" ? value.expires_at : "",
      wallet: { scheme: wallet.scheme, address: wallet.address },
    }
  } catch {
    return null
  }
}

/** Whether two wallet references name the same wallet. */
export function sameWallet(a: WalletRef, b: WalletRef): boolean {
  if (a.scheme !== b.scheme) return false
  return a.scheme === "eip191"
    ? a.address.toLowerCase() === b.address.toLowerCase()
    : a.address === b.address
}
