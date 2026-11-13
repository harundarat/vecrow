import { CircleAlert, CircleCheck } from "lucide-react"

/** The outcome of the last action: an error or a confirmation. */
export type Feedback = { kind: "error" | "success"; message: string } | null

export function FeedbackMessage({ feedback }: { feedback: Feedback }) {
  return (
    <div aria-live="polite">
      {feedback?.kind === "error" && (
        <p className="flex items-start gap-2 rounded-xl bg-destructive/10 p-3 text-sm text-destructive">
          <CircleAlert aria-hidden className="mt-0.5 size-4 shrink-0" />
          {feedback.message}
        </p>
      )}
      {feedback?.kind === "success" && (
        <p className="flex items-start gap-2 rounded-xl bg-primary/10 p-3 text-sm">
          <CircleCheck
            aria-hidden
            className="mt-0.5 size-4 shrink-0 text-primary"
          />
          {feedback.message}
        </p>
      )}
    </div>
  )
}
