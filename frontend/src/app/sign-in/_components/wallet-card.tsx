import Image from "next/image"
import { LoaderCircle } from "lucide-react"
import { cn } from "@/utils"

export function WalletCard({
  iconUrls,
  label,
  detail,
  busy = false,
  disabled = false,
  disableLabel,
  onClick,
}: {
  iconUrls: string[]
  label: string
  /** A second line under the label, e.g. the connected address. */
  detail?: string
  busy?: boolean
  disabled?: boolean
  disableLabel?: string
  onClick?: () => void
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      aria-busy={busy}
      className={cn(
        "relative flex min-w-0 cursor-pointer flex-col items-start gap-3 rounded-2xl bg-card p-4 text-left transition-all select-none hover:ring-2 hover:ring-primary/50 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none active:scale-98 disabled:pointer-events-none sm:p-5",
        busy && "ring-2 ring-primary"
      )}
    >
      <div className="flex w-full items-center justify-between gap-2">
        <div className="flex items-center">
          {iconUrls.map((url) => (
            <Image
              width={64}
              height={64}
              key={url}
              src={url}
              alt=""
              className="size-8 rounded-full border-2 border-card not-first:-ml-2"
            />
          ))}
        </div>
        {busy && (
          <LoaderCircle
            aria-hidden
            className="size-5 animate-spin text-primary"
          />
        )}
      </div>
      <span className="flex w-full min-w-0 flex-col gap-0.5">
        <span className="font-semibold">{label}</span>
        {detail && (
          <span className="truncate text-xs font-medium text-muted-foreground">
            {detail}
          </span>
        )}
      </span>
      {disabled && disableLabel && (
        <span className="absolute inset-0 flex items-center justify-center rounded-2xl bg-background/70 font-semibold">
          {disableLabel}
        </span>
      )}
    </button>
  )
}
