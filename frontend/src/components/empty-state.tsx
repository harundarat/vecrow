import { cn } from "@/utils"

/** A centered card for an empty list, a page that cannot load, or a locked feature. */
export function EmptyState({
  icon,
  title,
  description,
  action,
  tone = "default",
}: {
  icon: React.ReactNode
  title: string
  description: React.ReactNode
  action?: React.ReactNode
  tone?: "default" | "error"
}) {
  return (
    <div
      role={tone === "error" ? "alert" : undefined}
      className="flex flex-col items-center gap-3 rounded-2xl border border-dashed bg-card/40 px-4 py-12 text-center"
    >
      <span
        className={cn(
          "flex size-12 items-center justify-center rounded-full",
          tone === "error"
            ? "bg-destructive/10 text-destructive"
            : "bg-primary/15 text-primary"
        )}
      >
        {icon}
      </span>
      <div>
        <p className="font-semibold">{title}</p>
        <p className="mt-1 max-w-sm text-sm break-words text-muted-foreground">
          {description}
        </p>
      </div>
      {action}
    </div>
  )
}
