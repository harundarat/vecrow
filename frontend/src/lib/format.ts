import type { WalletScheme } from "./api/auth"
import type { EscrowRole } from "./api/escrow"

/** Shortens long wallet addresses for display; short NEAR names stay whole. */
export function shortenAddress(address: string): string {
  if (address.length <= 18) return address
  return `${address.slice(0, 6)}…${address.slice(-4)}`
}

export const WALLET_SCHEME_LABEL: Record<WalletScheme, string> = {
  eip191: "EVM",
  ed25519: "Solana",
  nep413: "NEAR",
}

export const ESCROW_ROLE_LABEL: Record<EscrowRole, string> = {
  freelancer: "Freelancer",
  client: "Client",
  member: "Team member",
}

/** "Nov 13, 2026" in the visitor's locale. */
export function formatDate(iso: string): string {
  return new Intl.DateTimeFormat(undefined, { dateStyle: "medium" }).format(
    new Date(iso)
  )
}

/** "2:05 PM" in the visitor's locale and time zone. */
export function formatTime(iso: string): string {
  return new Intl.DateTimeFormat(undefined, { timeStyle: "short" }).format(
    new Date(iso)
  )
}
