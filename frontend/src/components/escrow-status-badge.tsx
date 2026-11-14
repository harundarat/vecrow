import { Mail } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import type { EscrowStatus, MilestoneStatus } from "@/lib/api/escrow"
import { cn } from "@/utils"

export const ESCROW_STATUS_LABEL: Record<EscrowStatus, string> = {
  Open: "Open",
  PendingApproval: "Pending approval",
  Approved: "Approved",
  Created: "Created",
  Funded: "Funded",
  Completed: "Completed",
  Cancelled: "Cancelled",
}

/** What each status means for the people in the escrow. */
export const ESCROW_STATUS_HINT: Record<EscrowStatus, string> = {
  Open: "Waiting for the other party to join",
  PendingApproval: "Waiting for the creator to approve",
  Approved: "Being put on-chain",
  Created: "On-chain, waiting for payment",
  Funded: "Paid in, work in progress",
  Completed: "All milestones are closed",
  Cancelled: "Cancelled before payment",
}

const VARIANT: Record<EscrowStatus, "default" | "secondary" | "outline"> = {
  Open: "outline",
  PendingApproval: "secondary",
  Approved: "secondary",
  Created: "secondary",
  Funded: "default",
  Completed: "default",
  Cancelled: "outline",
}

export function EscrowStatusBadge({
  status,
  className,
}: {
  status: EscrowStatus
  className?: string
}) {
  return (
    <Badge
      variant={VARIANT[status] ?? "outline"}
      title={ESCROW_STATUS_HINT[status]}
      className={cn(
        status === "Cancelled" && "text-muted-foreground",
        className
      )}
    >
      {ESCROW_STATUS_LABEL[status] ?? status}
    </Badge>
  )
}

const MILESTONE_STATUS_LABEL: Record<MilestoneStatus, string> = {
  Pending: "Pending",
  Submitted: "Submitted",
  RevisionRequested: "Revision requested",
  Released: "Released",
  Refunded: "Refunded",
}

const MILESTONE_VARIANT: Record<
  MilestoneStatus,
  "default" | "secondary" | "outline"
> = {
  Pending: "outline",
  Submitted: "secondary",
  RevisionRequested: "secondary",
  Released: "default",
  Refunded: "outline",
}

export function MilestoneStatusBadge({ status }: { status: MilestoneStatus }) {
  return (
    <Badge
      variant={MILESTONE_VARIANT[status] ?? "outline"}
      className={cn(status === "Refunded" && "text-muted-foreground")}
    >
      {MILESTONE_STATUS_LABEL[status] ?? status}
    </Badge>
  )
}

export function InvitedBadge() {
  return (
    <Badge variant="outline" title="Someone invited you to this escrow">
      <Mail aria-hidden />
      Invited
    </Badge>
  )
}
