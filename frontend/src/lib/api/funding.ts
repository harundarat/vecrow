import { apiFetch } from "./client"

/** A token 1Click can take in; `blockchain` is its chain code ("eth", "sol"). */
export type Token = {
  asset_id: string
  symbol: string
  blockchain: string
  decimals: number
  price: number
  contract_address?: string
}

/** SUCCESS once the escrow holds its funds; REFUNDED when 1Click sent them back. */
export type FundingStatus = "PENDING" | "SUCCESS" | "REFUNDED"

export type FundingQuote = {
  id: string
  origin_asset: string
  refund_to: string
  deposit_address: string
  deposit_memo?: string
  /** Base units of the origin token. */
  amount_in: string
  amount_in_formatted: string
  /** USDC base units: the escrow total. */
  amount_out: string
  deadline: string | null
  status: FundingStatus
  oneclick_status?: string
  created_at: string
}

export type Funding = {
  status: FundingStatus
  oneclick_status?: string
  escrow_status: string
  quote: FundingQuote
}

/** Only the approver wallet funds: proven by its guest token or a session. */
export type FundingAuth = { token: string | null; guestToken: string | null }

export async function listTokens(signal?: AbortSignal) {
  const res = await apiFetch<{ tokens: Token[] }>("/tokens", { signal })
  return res.tokens
}

export function requestQuote(
  id: string,
  body: { origin_asset: string; refund_to: string },
  auth: FundingAuth
) {
  return apiFetch<FundingQuote>(`${path(id)}/quote`, {
    method: "POST",
    body,
    ...auth,
  })
}

/** The latest quote and its status; 404 until a quote was requested. */
export function getFunding(
  id: string,
  auth: FundingAuth,
  signal?: AbortSignal
) {
  return apiFetch<Funding>(`${path(id)}/funding`, { ...auth, signal })
}

function path(id: string) {
  return `/escrows/${encodeURIComponent(id)}`
}
