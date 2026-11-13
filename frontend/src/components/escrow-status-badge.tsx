import { Badge } from "@/components/ui/badge"
import type { EscrowStatus } from "@/lib/api/escrow"
import { cn } from "@/utils"

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
      {status}
    </Badge>
  )
}
