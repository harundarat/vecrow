import { CopyButton } from "@/components/copy-button"
import { shortenAddress } from "@/lib/format"
import { cn } from "@/utils"

/** A wallet or account address, shortened, with the full value on hover and a copy button. */
export function Address({
  value,
  className,
}: {
  value: string
  className?: string
}) {
  return (
    <span
      className={cn(
        "inline-flex max-w-full min-w-0 items-center gap-1 align-middle",
        className
      )}
    >
      <span className="truncate font-mono" title={value}>
        {shortenAddress(value)}
      </span>
      <CopyButton value={value} label={`Copy ${value}`} iconOnly />
    </span>
  )
}
