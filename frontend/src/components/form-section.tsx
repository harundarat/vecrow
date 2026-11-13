import { useId } from "react"

export function FormSection({
  title,
  description,
  children,
}: React.PropsWithChildren<{ title: string; description?: React.ReactNode }>) {
  const headingId = useId()
  return (
    <section
      aria-labelledby={headingId}
      className="flex flex-col gap-4 rounded-2xl bg-card p-4 sm:p-6"
    >
      <div>
        <h2 id={headingId} className="text-lg font-medium">
          {title}
        </h2>
        {description && (
          <p className="mt-1 text-sm text-muted-foreground">{description}</p>
        )}
      </div>
      {children}
    </section>
  )
}
