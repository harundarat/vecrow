import type { Account, WalletScheme } from "./api/auth"
import type { Invitation, InvitationInput } from "./api/escrow"
import { emailError, usernameError, walletAddressError } from "./escrow-rules"
import { WALLET_SCHEME_LABEL } from "./format"

export type InviteKind = "none" | "username" | "email" | "wallet"

/** The invitation as typed in a form. */
export type InviteState = {
  kind: InviteKind
  value: string
  scheme: WalletScheme
}

export const NO_INVITE: InviteState = {
  kind: "none",
  value: "",
  scheme: "nep413",
}

/** Prefills the form with an existing invitation. */
export function inviteStateFrom(invitation: Invitation | null): InviteState {
  if (!invitation) return NO_INVITE
  switch (invitation.kind) {
    case "username":
      return {
        ...NO_INVITE,
        kind: "username",
        value: invitation.username ?? "",
      }
    case "email":
      return { ...NO_INVITE, kind: "email", value: invitation.email ?? "" }
    case "wallet":
      return {
        kind: "wallet",
        value: invitation.wallet?.address ?? "",
        scheme: invitation.wallet?.scheme ?? "nep413",
      }
  }
}

/** The request body for the invitation, or undefined for no invitation. */
export function invitationBody(
  invite: InviteState
): InvitationInput | undefined {
  const value = invite.value.trim()
  switch (invite.kind) {
    case "username":
      return { username: value.toLowerCase() }
    case "email":
      return { email: value }
    case "wallet":
      return { wallet: { scheme: invite.scheme, address: value } }
    default:
      return undefined
  }
}

/** An error for the typed invitation, or null when it can be sent. */
export function invitationError(
  account: Account | null,
  invite: InviteState
): string | null {
  const value = invite.value.trim()
  const own = "That is your own account. Invite the other party."
  switch (invite.kind) {
    case "username":
      if (usernameError(value)) return usernameError(value)
      return value.toLowerCase() === account?.username ? own : null
    case "email":
      if (emailError(value)) return emailError(value)
      return account?.email &&
        value.toLowerCase() === account.email.toLowerCase()
        ? own
        : null
    case "wallet": {
      const addressError = walletAddressError(invite.scheme, value)
      if (addressError) return addressError
      const mine = account?.wallets.some(
        (w) =>
          w.scheme === invite.scheme &&
          (w.scheme === "eip191"
            ? w.address.toLowerCase() === value.toLowerCase()
            : w.address === value)
      )
      return mine ? own : null
    }
    default:
      return null
  }
}

/** "@alice", "alice@example.com" or "NEAR · alice.near". */
export function describeInvitation(invitation: Invitation): string {
  switch (invitation.kind) {
    case "username":
      return `@${invitation.username ?? ""}`
    case "email":
      return invitation.email ?? ""
    case "wallet":
      return invitation.wallet
        ? `${WALLET_SCHEME_LABEL[invitation.wallet.scheme]} · ${invitation.wallet.address}`
        : ""
  }
}
