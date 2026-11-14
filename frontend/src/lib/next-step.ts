import type { Escrow, EscrowRole, Milestone } from "./api/escrow"
import { formatUsdc } from "./usdc"

export type NextStep = {
  text: string
  /** The viewer is the one who acts next. */
  yours: boolean
}

/** What happens next in an escrow, from the viewer's side. */
export function nextStep(
  escrow: Pick<Escrow, "status" | "total" | "milestones" | "pending_payouts">,
  roles: EscrowRole[],
  creator: boolean
): NextStep {
  const freelancer = roles.includes("freelancer")
  const client = roles.includes("client")
  const yours = (text: string) => ({ text, yours: true })
  const wait = (text: string) => ({ text, yours: false })

  switch (escrow.status) {
    case "Open":
      return creator
        ? yours("Send the link to the other party")
        : yours("Join with your details")
    case "PendingApproval":
      return creator
        ? yours("Review the other party's details")
        : wait("Waiting for the creator to approve")
    case "Approved":
      return wait("Going on-chain, about a minute")
    case "Created":
      return client
        ? yours(`Pay in ${formatUsdc(escrow.total)} USDC`)
        : wait("Waiting for the client to pay in")
    case "Funded":
      return fundedStep(escrow.milestones, freelancer, client)
    case "Completed":
      return escrow.pending_payouts.length > 0
        ? wait("A payout failed. Vecrow retries it automatically")
        : wait("Done. Every milestone is paid or refunded")
    case "Cancelled":
      return wait("Cancelled. Nothing left to do")
  }
}

function fundedStep(
  milestones: Milestone[],
  freelancer: boolean,
  client: boolean
): NextStep {
  const first = (status: Milestone["status"]) =>
    milestones.find((m) => m.status === status)
  const label = (m: Milestone) => `milestone ${m.index + 1}`
  const revision = first("RevisionRequested")
  const submitted = first("Submitted")
  const pending = first("Pending")

  if (freelancer) {
    if (revision)
      return { text: `Revise and resubmit ${label(revision)}`, yours: true }
    if (pending) return { text: `Submit ${label(pending)}`, yours: true }
  }
  if (client && submitted) {
    return { text: `Review ${label(submitted)}`, yours: true }
  }
  if (submitted) {
    return {
      text: `Waiting for the client to review ${label(submitted)}`,
      yours: false,
    }
  }
  if (revision) {
    return {
      text: `Waiting for the freelancer to revise ${label(revision)}`,
      yours: false,
    }
  }
  if (pending) {
    return {
      text: `Waiting for the freelancer to submit ${label(pending)}`,
      yours: false,
    }
  }
  return { text: "Waiting for the contract to close the escrow", yours: false }
}
