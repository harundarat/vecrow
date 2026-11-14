// USDC amounts stay strings end to end: the API sends base units (1 USDC =
// 1,000,000) as decimal strings, and any math uses BigInt, never floats.

export const USDC_DECIMALS = 6

const AMOUNT_INPUT = /^(\d+)(?:\.(\d*))?$/

export type ParsedUsdc =
  { ok: true; baseUnits: string } | { ok: false; error: string }

/**
 * Reads an amount typed in USDC ("1500" or "1500.25") and returns it in base
 * units, the form the API accepts: digits only, no leading zeros.
 */
export function parseUsdc(input: string): ParsedUsdc {
  const text = input.trim()
  if (!text) return { ok: false, error: "Enter an amount." }
  const match = AMOUNT_INPUT.exec(text)
  if (!match) {
    return { ok: false, error: "Use digits and a dot, like 1500 or 1500.50." }
  }
  const [, whole, fraction = ""] = match
  if (fraction.length > USDC_DECIMALS) {
    return { ok: false, error: "Use at most 6 decimal places." }
  }
  const digits = (whole + fraction.padEnd(USDC_DECIMALS, "0")).replace(
    /^0+/,
    ""
  )
  if (!digits) return { ok: false, error: "Enter an amount above 0." }
  return { ok: true, baseUnits: digits }
}

/** Base units as USDC for display: "696500000" → "696.50". */
export function formatUsdc(baseUnits: string): string {
  const digits = baseUnits.replace(/^0+/, "").padStart(USDC_DECIMALS + 1, "0")
  const whole = digits.slice(0, -USDC_DECIMALS)
  const fraction = digits
    .slice(-USDC_DECIMALS)
    .replace(/0+$/, "")
    .padEnd(2, "0")
  return `${whole.replace(/\B(?=(\d{3})+(?!\d))/g, ",")}.${fraction}`
}

/** Sum of amounts in base units. */
export function sumUsdc(amounts: string[]): string {
  return amounts
    .reduce((total, amount) => total + BigInt(amount), BigInt(0))
    .toString()
}
