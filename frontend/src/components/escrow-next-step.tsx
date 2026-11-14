import { ArrowRight, Clock } from "lucide-react"
import type { Escrow, EscrowRole } from "@/lib/api/escrow"
import { nextStep } from "@/lib/next-step"
import { cn } from "@/utils"

/** "Next: Submit milestone 2" when the viewer acts, otherwise who we wait for. */
export function EscrowNextStep({
  escrow,
  roles,
  creator,
  className,
}: {
  escrow: Pick<Escrow, "status" | "total" | "milestones" | "pending_payouts">
  roles: EscrowRole[]
  creator: boolean
  className?: string
}) {
  const step = nextStep(escrow, roles, creator)
  const Icon = step.yours ? ArrowRight : Clock
  return (
    <p className={cn("flex items-start gap-2 text-sm", className)}>
      <Icon
        aria-hidden
        className={cn(
          "mt-0.5 size-4 shrink-0",
          step.yours ? "text-primary" : "text-muted-foreground"
        )}
      />
      <span className={cn("min-w-0", !step.yours && "text-muted-foreground")}>
        {step.yours && <span className="font-medium">Next: </span>}
        {step.text}
      </span>
    </p>
  )
}
