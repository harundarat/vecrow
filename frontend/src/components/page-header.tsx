import Link from "next/link"
import { ArrowLeft } from "lucide-react"
import { cn } from "@/utils"

export function BackLink({
  href,
  className,
  children,
}: React.PropsWithChildren<{ href: string; className?: string }>) {
  return (
    <Link
      href={href}
      className={cn(
        "inline-flex items-center gap-1 self-start rounded-md text-sm text-muted-foreground transition-colors outline-none hover:text-foreground focus-visible:ring-3 focus-visible:ring-ring/50",
        className
      )}
    >
      <ArrowLeft aria-hidden className="size-4" />
      {children}
    </Link>
  )
}

/** The page title, with an optional link back and an action on the right. */
export function PageHeader({
  title,
  description,
  back,
  action,
}: {
  title: string
  description?: React.ReactNode
  back?: { href: string; label: string }
  action?: React.ReactNode
}) {
  return (
    <div className="flex flex-wrap items-end justify-between gap-4">
      <div className="flex min-w-0 flex-col gap-2">
        {back && <BackLink href={back.href}>{back.label}</BackLink>}
        <div>
          <h1 className="text-2xl font-medium">{title}</h1>
          {description && (
            <p className="mt-1 text-sm text-muted-foreground">{description}</p>
          )}
        </div>
      </div>
      {action}
    </div>
  )
}
