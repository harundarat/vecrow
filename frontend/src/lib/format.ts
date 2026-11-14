import type { WalletScheme } from "./api/auth"
import type { EscrowRole } from "./api/escrow"

/**
 * Shortens hex and base58 addresses for display. NEAR names such as
 * `alice.testnet` stay whole: cut in the middle, they no longer say whose
 * account it is.
 */
export function shortenAddress(address: string): string {
  if (address.length <= 18 || address.includes(".")) return address
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

const MINUTE = 60_000
const HOUR = 60 * MINUTE
const DAY = 24 * HOUR

function plural(n: number, unit: string) {
  return `${n} ${unit}${n === 1 ? "" : "s"}`
}

/** A rough length of time: "13 days 4 hours", "2 hours 5 minutes", "1 minute". */
export function formatDuration(ms: number): string {
  if (ms < MINUTE) return "less than a minute"
  const days = Math.floor(ms / DAY)
  const hours = Math.floor((ms % DAY) / HOUR)
  const minutes = Math.floor((ms % HOUR) / MINUTE)
  if (days > 0) {
    return hours > 0
      ? `${plural(days, "day")} ${plural(hours, "hour")}`
      : plural(days, "day")
  }
  if (hours > 0) {
    return minutes > 0
      ? `${plural(hours, "hour")} ${plural(minutes, "minute")}`
      : plural(hours, "hour")
  }
  return plural(minutes, "minute")
}
