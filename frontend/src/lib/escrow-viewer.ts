import type { Account } from "./api/auth"
import type { Approver, Escrow, EscrowRole, WalletRef } from "./api/escrow"
import type { FundingAuth } from "./api/funding"
import { sameWallet, type GuestAccess } from "./guest-access"

export function approverWallet(approver: Approver): WalletRef {
  return {
    scheme: approver.scheme,
    address: approver.account_id ?? approver.address ?? "",
  }
}

function ownsWallet(account: Account | null, wallet: WalletRef): boolean {
  return (
    account?.wallets.some((w) =>
      sameWallet({ scheme: w.scheme, address: w.address }, wallet)
    ) ?? false
  )
}

export type ApproverView = {
  /** The visitor holds the approver wallet: by its guest access or account. */
  approver: boolean
  /** Credentials the API accepts for funding, or null when they expired. */
  fundingAuth: FundingAuth | null
}

/** Whether the visitor is the client's approver wallet. */
export function approverView(
  escrow: Escrow,
  guest: GuestAccess | null,
  account: Account | null,
  sessionToken: string | null
): ApproverView {
  const approver = escrow.client?.approver
  if (!approver) return { approver: false, fundingAuth: null }
  const wallet = approverWallet(approver)
  const bySession = sessionToken !== null && ownsWallet(account, wallet)
  const byGuest = guest !== null && sameWallet(guest.wallet, wallet)
  // A session lasts longer than a guest token, so it is preferred.
  const fundingAuth = bySession
    ? { token: sessionToken, guestToken: null }
    : byGuest && guest.token
      ? { token: null, guestToken: guest.token }
      : null
  return { approver: bySession || byGuest, fundingAuth }
}

/** Whether the visitor is the freelancer, by guest access or account. */
export function isFreelancer(
  escrow: Escrow & { viewer: { creator: boolean } },
  guest: GuestAccess | null,
  account: Account | null
): boolean {
  if (escrow.viewer.creator && escrow.creator_role === "freelancer") {
    return true
  }
  const freelancer = escrow.freelancer?.account
  if (!freelancer) return false
  const wallet: WalletRef = { scheme: "nep413", address: freelancer }
  return (
    (guest !== null && sameWallet(guest.wallet, wallet)) ||
    ownsWallet(account, wallet)
  )
}

/** The viewer's roles in the escrow, as the dashboard lists them. */
export function viewerRoles(
  escrow: Escrow & { viewer: { creator: boolean } },
  guest: GuestAccess | null,
  account: Account | null,
  approver: boolean
): EscrowRole[] {
  const roles: EscrowRole[] = []
  if (isFreelancer(escrow, guest, account)) roles.push("freelancer")
  if (approver || (escrow.viewer.creator && escrow.creator_role === "client")) {
    roles.push("client")
  }
  const member = (escrow.freelancer?.split ?? []).some((share) => {
    const wallet: WalletRef = { scheme: "nep413", address: share.account_id }
    return (
      (guest !== null && sameWallet(guest.wallet, wallet)) ||
      ownsWallet(account, wallet)
    )
  })
  if (member && !roles.includes("freelancer")) roles.push("member")
  return roles
}
