import type { WalletScheme } from "./auth"
import { apiFetch, type Requester } from "./client"

/** The first three exist only in the backend, before the escrow is on-chain. */
export const ESCROW_STATUSES = [
  "Open",
  "PendingApproval",
  "Approved",
  "Created",
  "Funded",
  "Completed",
  "Cancelled",
] as const

export type EscrowStatus = (typeof ESCROW_STATUSES)[number]

export type MilestoneStatus =
  "Pending" | "Submitted" | "RevisionRequested" | "Released" | "Refunded"

/** The role the creator takes. */
export type CreatorRole = "freelancer" | "client"

/** A role in "my escrows": members are split recipients. */
export type EscrowRole = CreatorRole | "member"

export type SplitShare = { account_id: string; bps: number }

/** account_id and public_key for nep413; address for eip191 and ed25519. */
export type Approver = {
  scheme: WalletScheme
  account_id?: string
  public_key?: string
  address?: string
}

export type WalletRef = { scheme: WalletScheme; address: string }

export type Milestone = {
  index: number
  /** Base units. */
  amount: string
  status: MilestoneStatus
  submitted_at_ms: number | null
  auto_release_at_ms: number | null
  revision_count: number
}

export type EscrowTerms = {
  escrow_id: string
  freelancer: string
  milestones: string[]
  split: SplitShare[]
  approver: Approver
  refund_account: string
  fee_bps: number
  deadline_ms: number
}

export type Submission = {
  id: string
  status: string
  terms: EscrowTerms
  message: string
  nonce: string
  recipient: string
  wallet: WalletRef
  created_at: string
}

export type Escrow = {
  id: string
  link: string
  contract_id: string
  status: EscrowStatus
  creator_role: CreatorRole
  fee_bps: number
  /** Base units. */
  total: string
  milestones: Milestone[]
  freelancer: { account: string; split: SplitShare[] } | null
  client: { approver: Approver; refund_account: string } | null
  submission: Submission | null
  review_window_sec: number | null
  balance?: string
  pending_payouts: Array<{ account_id: string; amount: string }>
  created_at_ms: number | null
  funded_at_ms: number | null
  created_at: string
  updated_at: string
}

export type MyEscrow = Escrow & {
  roles: EscrowRole[]
  /** The account was invited; the invited role is among `roles`. */
  invited: boolean
}

export type Invitation = {
  kind: "username" | "email" | "wallet"
  username?: string
  email?: string
  wallet?: WalletRef
}

/** Names the other party by exactly one of username, email or wallet. */
export type InvitationInput =
  { username: string } | { email: string } | { wallet: WalletRef }

export type CreateEscrowRequest = {
  role: CreatorRole
  /** Base units, one per milestone. */
  milestones: string[]
  invitation?: InvitationInput
} & (
  | { role: "freelancer"; freelancer: string; split: SplitShare[] }
  | { role: "client"; approver: Approver; refund_account: string }
)

export type CreatedEscrow = Escrow & { invitation: Invitation | null }

export type Transaction = {
  hash: string
  url?: string
  method: string
  signer_id: string
  sender_id: string
  receiver_id: string
  succeeded: boolean
  error?: string
  events: Array<{ event: string; data: unknown }>
  created_at?: string
}

export type EscrowDetail = Escrow & {
  transactions: Transaction[]
  revision_notes: Array<{
    milestone: number
    reason: string
    tx_hash: string
    created_at: string
  }>
  viewer: { creator: boolean; invited: boolean }
  /** Shown to the creator only. */
  invitation: Invitation | null
  warnings: Array<{ code: string; message: string }>
}

export async function listMyEscrows(
  request: Requester,
  filter: { role?: EscrowRole; status?: EscrowStatus },
  signal?: AbortSignal
) {
  const params = new URLSearchParams()
  if (filter.role) params.set("role", filter.role)
  if (filter.status) params.set("status", filter.status)
  const query = params.size ? `?${params}` : ""
  const res = await request<{ escrows: MyEscrow[] }>(`/me/escrows${query}`, {
    signal,
  })
  return res.escrows
}

export function createEscrow(request: Requester, body: CreateEscrowRequest) {
  return request<CreatedEscrow>("/escrows", { method: "POST", body })
}

/** Anyone with the link can read an escrow; a session adds what the viewer may see. */
export function getEscrow(
  id: string,
  token: string | null,
  signal?: AbortSignal
) {
  return apiFetch<EscrowDetail>(`/escrows/${encodeURIComponent(id)}`, {
    token,
    signal,
  })
}
