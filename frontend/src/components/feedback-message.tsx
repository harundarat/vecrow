import { CircleAlert, CircleCheck } from "lucide-react"
import { cn } from "@/utils"

/** The outcome of the last action: an error or a confirmation. */
export type Feedback = { kind: "error" | "success"; message: string } | null

export function FeedbackMessage({ feedback }: { feedback: Feedback }) {
  return (
    <div aria-live="polite">
      {feedback && <Notice kind={feedback.kind}>{feedback.message}</Notice>}
    </div>
  )
}

/** A boxed error or confirmation, with an optional button beside it. */
export function Notice({
  kind = "error",
  role,
  action,
  className,
  children,
}: React.PropsWithChildren<{
  kind?: "error" | "success"
  role?: "alert"
  action?: React.ReactNode
  className?: string
}>) {
  const Icon = kind === "error" ? CircleAlert : CircleCheck
  return (
    <div
      role={role}
      className={cn(
        "flex flex-col gap-3 rounded-xl p-3 text-sm sm:flex-row sm:items-center sm:justify-between",
        kind === "error"
          ? "bg-destructive/10 text-destructive"
          : "bg-primary/10",
        className
      )}
    >
      <p className="flex min-w-0 items-start gap-2">
        <Icon
          aria-hidden
          className={cn(
            "mt-0.5 size-4 shrink-0",
            kind === "success" && "text-primary"
          )}
        />
        <span className="min-w-0 break-words">{children}</span>
      </p>
      {action && (
        <div className="shrink-0 self-start sm:self-auto">{action}</div>
      )}
    </div>
  )
}
