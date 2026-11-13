// The rules the backend applies to a new escrow, checked in the form so
// mistakes show before submitting. The backend stays the authority: its
// errors are still shown.

import type { WalletScheme } from "./api/auth"
import { TOTAL_BPS } from "./payout"
import { parseUsdc } from "./usdc"
import { base58ToBytes } from "./wallet-encoding"

export const MAX_MILESTONES = 20
export const MAX_SPLIT_RECIPIENTS = 10

const MAX_U128 = (BigInt(1) << BigInt(128)) - BigInt(1)

// Named accounts: lowercase parts separated by dots, - or _ only between
// letters and digits. Implicit accounts: 64 lowercase hex characters.
const NEAR_NAMED = /^(([a-z\d]+[-_])*[a-z\d]+\.)*([a-z\d]+[-_])*[a-z\d]+$/
const NEAR_IMPLICIT = /^[0-9a-f]{64}$/
const EVM_ADDRESS = /^0x[0-9a-fA-F]{40}$/
const SOLANA_ADDRESS = /^[1-9A-HJ-NP-Za-km-z]{32,44}$/
const USERNAME = /^[a-z][a-z0-9_]{2,29}$/
const MAX_EMAIL_LENGTH = 254

export function isNearAccount(value: string): boolean {
  if (value.length < 2 || value.length > 64) return false
  return NEAR_IMPLICIT.test(value) || NEAR_NAMED.test(value)
}

function isSolanaAddress(value: string): boolean {
  if (!SOLANA_ADDRESS.test(value)) return false
  try {
    base58ToBytes(value, 32)
    return true
  } catch {
    return false
  }
}

/** An error for a NEAR account ID, or null when it is valid. */
export function nearAccountError(value: string): string | null {
  if (!value.trim()) return "Enter a NEAR account."
  if (value !== value.trim()) return "Remove the spaces."
  if (!isNearAccount(value)) {
    return "Not a NEAR account. Use lowercase, like alice.near, or a 64-character hex ID."
  }
  return null
}

/** An error for a wallet address of `scheme`, or null when it is valid. */
export function walletAddressError(
  scheme: WalletScheme,
  value: string
): string | null {
  const address = value.trim()
  if (!address) return "Enter a wallet address."
  switch (scheme) {
    case "nep413":
      return isNearAccount(address) ? null : "Not a NEAR account."
    case "eip191":
      return EVM_ADDRESS.test(address)
        ? null
        : "Not an EVM address. It starts with 0x and has 40 hex characters."
    case "ed25519":
      return isSolanaAddress(address) ? null : "Not a Solana address."
  }
}

export function usernameError(value: string): string | null {
  const username = value.trim().toLowerCase()
  if (!username) return "Enter a username."
  if (!USERNAME.test(username)) {
    return "Usernames have 3–30 characters: a-z, 0-9 or _, and start with a letter."
  }
  return null
}

export function emailError(value: string): string | null {
  const email = value.trim()
  if (!email) return "Enter an email address."
  const [local, host, ...rest] = email.split("@")
  const valid =
    email.length <= MAX_EMAIL_LENGTH &&
    rest.length === 0 &&
    Boolean(local) &&
    Boolean(host) &&
    host.includes(".") &&
    !host.startsWith(".") &&
    !host.endsWith(".") &&
    !/\s/.test(email)
  return valid ? null : "Enter an email like name@example.com."
}

export type MilestonesCheck = {
  /** Base units of each milestone, or null where the amount is invalid. */
  amounts: Array<string | null>
  /** An error per milestone, or null. */
  errors: Array<string | null>
  /** An error about the list as a whole, or null. */
  listError: string | null
}

export function checkMilestones(inputs: string[]): MilestonesCheck {
  const parsed = inputs.map(parseUsdc)
  const amounts = parsed.map((p) => (p.ok ? p.baseUnits : null))
  const errors = parsed.map((p) => (p.ok ? null : p.error))
  let listError: string | null = null
  if (inputs.length === 0) listError = "Add at least one milestone."
  else if (inputs.length > MAX_MILESTONES) {
    listError = `Use at most ${MAX_MILESTONES} milestones.`
  } else if (
    amounts.every((a) => a !== null) &&
    amounts.reduce((sum, a) => sum + BigInt(a ?? 0), BigInt(0)) > MAX_U128
  ) {
    listError = "The total is too large."
  }
  return { amounts, errors, listError }
}

export type ParsedPercent =
  { ok: true; bps: number } | { ok: false; error: string }

const PERCENT_INPUT = /^(\d{1,3})(?:\.(\d{0,2}))?$/

/** Reads a share typed in percent ("70" or "33.33") as basis points. */
export function parsePercent(input: string): ParsedPercent {
  const text = input.trim().replace(/%$/, "").trim()
  if (!text) return { ok: false, error: "Enter a share." }
  const match = PERCENT_INPUT.exec(text)
  if (!match) {
    return {
      ok: false,
      error: "Use a number with up to 2 decimals, like 70 or 33.33.",
    }
  }
  const [, whole, fraction = ""] = match
  const bps = Number(whole) * 100 + Number(fraction.padEnd(2, "0"))
  if (bps === 0) return { ok: false, error: "A share must be above 0%." }
  if (bps > TOTAL_BPS)
    return { ok: false, error: "A share cannot be above 100%." }
  return { ok: true, bps }
}

/** Basis points as a percent for display: 3333 → "33.33%". */
export function formatPercent(bps: number): string {
  const whole = Math.floor(bps / 100)
  const fraction = String(bps % 100)
    .padStart(2, "0")
    .replace(/0+$/, "")
  return fraction ? `${whole}.${fraction}%` : `${whole}%`
}

export type SplitInput = { account: string; percent: string }

export type SplitCheck = {
  /** Basis points of each recipient, or null where the share is invalid. */
  bps: Array<number | null>
  accountErrors: Array<string | null>
  shareErrors: Array<string | null>
  /** Sum of the valid shares. */
  totalBps: number
  /** An error about the split as a whole, or null. */
  listError: string | null
}

export function checkSplit(rows: SplitInput[]): SplitCheck {
  const parsed = rows.map((row) => parsePercent(row.percent))
  const bps = parsed.map((p) => (p.ok ? p.bps : null))
  const shareErrors = parsed.map((p) => (p.ok ? null : p.error))
  const accountErrors = rows.map((row, i) => {
    const error = nearAccountError(row.account)
    if (error) return error
    const first = rows.findIndex((other) => other.account === row.account)
    return first < i ? "This account is already in the split." : null
  })
  const totalBps = bps.reduce<number>((sum, b) => sum + (b ?? 0), 0)

  let listError: string | null = null
  if (rows.length === 0) listError = "Add at least one recipient."
  else if (rows.length > MAX_SPLIT_RECIPIENTS) {
    listError = `Use at most ${MAX_SPLIT_RECIPIENTS} recipients.`
  } else if (bps.every((b) => b !== null) && totalBps !== TOTAL_BPS) {
    listError = `Shares must add up to 100%. They add up to ${formatPercent(totalBps)}.`
  }
  return { bps, accountErrors, shareErrors, totalBps, listError }
}
