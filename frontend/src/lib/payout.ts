// How a released milestone is paid out. This mirrors the escrow contract
// exactly, so the preview shows what recipients will really receive.

export const TOTAL_BPS = 10_000

/**
 * The platform fee in basis points (0.5%). The backend sets the real value
 * and returns it as `fee_bps`; this is only for the preview before an escrow
 * exists.
 */
export const PLATFORM_FEE_BPS = 50

export type Share = { account_id: string; bps: number }

export type ReleasePlan = {
  /** Base units sent to the platform. */
  fee: string
  /** Base units for each recipient, in split order. */
  shares: string[]
}

/** floor(amount × bps / 10,000). */
export function mulBps(amount: bigint, bps: number): bigint {
  return (amount * BigInt(bps)) / BigInt(TOTAL_BPS)
}

/**
 * Splits one milestone (base units): the fee first, then each recipient's
 * share of the rest. Shares are rounded down and what is left over goes to
 * the first recipient, so the parts always add up to the amount.
 */
export function releasePlan(
  amount: string,
  feeBps: number,
  split: Share[]
): ReleasePlan {
  const total = BigInt(amount)
  const fee = mulBps(total, feeBps)
  const rest = total - fee
  const shares = split.map((share) => mulBps(rest, share.bps))
  if (shares.length > 0) {
    const assigned = shares.reduce((sum, share) => sum + share, BigInt(0))
    shares[0] += rest - assigned
  }
  return { fee: fee.toString(), shares: shares.map((s) => s.toString()) }
}
