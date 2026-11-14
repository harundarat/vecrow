import { useId } from "react"

/** A card with a heading. Give it an `id` to link to it from another page. */
export function FormSection({
  id,
  title,
  description,
  children,
}: React.PropsWithChildren<{
  id?: string
  title: string
  description?: React.ReactNode
}>) {
  const headingId = useId()
  return (
    <section
      id={id}
      aria-labelledby={headingId}
      className="flex scroll-mt-24 flex-col gap-4 rounded-2xl bg-card p-4 sm:p-6"
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
