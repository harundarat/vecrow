import { CircleAlert, CircleCheck } from "lucide-react"

type SettingsSectionProps = React.PropsWithChildren<{
  id: string
  title: string
  description: React.ReactNode
}>

export function SettingsSection({
  id,
  title,
  description,
  children,
}: SettingsSectionProps) {
  return (
    <section
      id={id}
      aria-labelledby={`${id}-heading`}
      className="flex scroll-mt-24 flex-col gap-4 rounded-2xl bg-card p-4 sm:p-6"
    >
      <div>
        <h2 id={`${id}-heading`} className="text-lg font-medium">
          {title}
        </h2>
        <p className="mt-1 text-sm text-muted-foreground">{description}</p>
      </div>
      {children}
    </section>
  )
}

/** The outcome of the section's last action: an error or a confirmation. */
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
