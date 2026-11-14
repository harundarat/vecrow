import { LoaderCircle } from "lucide-react"
import { cn } from "@/utils"

/** A spinner that screen readers announce as `label`. */
export function LoadingState({
  label,
  className,
}: {
  label: string
  className?: string
}) {
  return (
    <div
      role="status"
      aria-busy="true"
      className={cn("flex justify-center py-12", className)}
    >
      <LoaderCircle
        aria-hidden
        className="size-6 animate-spin text-muted-foreground"
      />
      <span className="sr-only">{label}</span>
    </div>
  )
}
