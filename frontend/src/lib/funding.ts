import type { WalletScheme } from "./api/auth"
import type { Approver } from "./api/escrow"
import type { Funding, Token } from "./api/funding"

const CHAIN_NAMES: Record<string, string> = {
  near: "NEAR",
  eth: "Ethereum",
  sol: "Solana",
  base: "Base",
  arb: "Arbitrum",
  op: "Optimism",
  pol: "Polygon",
  bsc: "BNB Chain",
  avax: "Avalanche",
  gnosis: "Gnosis",
  bera: "Berachain",
  scroll: "Scroll",
  monad: "Monad",
  btc: "Bitcoin",
  zec: "Zcash",
  doge: "Dogecoin",
  ltc: "Litecoin",
  xrp: "XRP",
  tron: "Tron",
  ton: "TON",
  sui: "Sui",
  aptos: "Aptos",
  stellar: "Stellar",
  starknet: "Starknet",
}

// Chains where an EVM address can receive a refund.
const EVM_CHAINS = new Set([
  "eth",
  "base",
  "arb",
  "op",
  "pol",
  "bsc",
  "avax",
  "gnosis",
  "bera",
  "scroll",
  "monad",
])

const SCHEME_CHAIN: Record<WalletScheme, string> = {
  nep413: "near",
  eip191: "eth",
  ed25519: "sol",
}

export function chainName(code: string): string {
  return CHAIN_NAMES[code] ?? code.toUpperCase()
}

/** The chains in the token list, by name. */
export function tokenChains(tokens: Token[]): string[] {
  return [...new Set(tokens.map((t) => t.blockchain))].sort((a, b) =>
    chainName(a).localeCompare(chainName(b))
  )
}

export function chainTokens(tokens: Token[], chain: string): Token[] {
  return tokens
    .filter((t) => t.blockchain === chain)
    .sort((a, b) => a.symbol.localeCompare(b.symbol))
}

/** The approver's own chain when 1Click supports it, else the first one. */
export function defaultChain(chains: string[], scheme: WalletScheme): string {
  const own = SCHEME_CHAIN[scheme]
  return chains.includes(own) ? own : (chains[0] ?? "")
}

/** USDC when the chain has it: no swap price to worry about. */
export function defaultToken(tokens: Token[]): Token | undefined {
  return tokens.find((t) => t.symbol === "USDC") ?? tokens[0]
}

/** The approver wallet, when it can take a refund on `chain`. */
export function refundAddressFor(
  approver: Approver,
  chain: string
): string | undefined {
  const fits =
    (approver.scheme === "nep413" && chain === "near") ||
    (approver.scheme === "ed25519" && chain === "sol") ||
    (approver.scheme === "eip191" && EVM_CHAINS.has(chain))
  return fits ? (approver.account_id ?? approver.address) : undefined
}

/** Where a 1Click deposit can be followed, by its deposit address. */
export function depositExplorerUrl(depositAddress: string): string {
  return `https://explorer.near-intents.org/transactions/${encodeURIComponent(depositAddress)}`
}

const ONECLICK_STATUS_TEXT: Record<string, string> = {
  PENDING_DEPOSIT: "Waiting for your payment.",
  KNOWN_DEPOSIT_TX: "Payment seen. Waiting for it to confirm.",
  PROCESSING: "Payment received. Swapping it to USDC…",
  SUCCESS: "Swap done. Waiting for the escrow to receive it.",
  INCOMPLETE_DEPOSIT:
    "Less than the quoted amount arrived. It goes back to your refund address.",
  FAILED: "The swap failed. Get a new deposit address and try again.",
}

export function fundingStatusText(funding: Funding): string {
  if (funding.status === "SUCCESS") return "Paid. The escrow is funded."
  if (funding.status === "REFUNDED") {
    return `1Click sent the payment back to ${funding.quote.refund_to}.`
  }
  const raw = funding.oneclick_status ?? funding.quote.oneclick_status
  return (raw && ONECLICK_STATUS_TEXT[raw]) ?? "Waiting for your payment."
}

/**
 * A pending quote can be replaced once nothing was sent to it in time, or
 * once 1Click gave up on it.
 */
export function canRequestNewQuote(funding: Funding, nowMs: number): boolean {
  if (funding.status === "REFUNDED") return true
  if (funding.status !== "PENDING") return false
  const raw = funding.oneclick_status ?? funding.quote.oneclick_status
  if (raw === "FAILED") return true
  const deadline = funding.quote.deadline
  const expired = deadline !== null && Date.parse(deadline) < nowMs
  return expired && (raw === undefined || raw === "PENDING_DEPOSIT")
}
