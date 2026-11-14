"use client"

import { useEffect, useState } from "react"
import { Check, Copy } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"

type CopyState = "idle" | "copied" | "failed"

/**
 * Copies `value` to the clipboard and says so for a moment. With `iconOnly`,
 * `label` is the button's accessible name.
 */
export function CopyButton({
  value,
  label = "Copy",
  iconOnly = false,
}: {
  value: string
  label?: string
  iconOnly?: boolean
}) {
  const [state, setState] = useState<CopyState>("idle")

  useEffect(() => {
    if (state === "idle") return
    const timer = setTimeout(() => setState("idle"), 2000)
    return () => clearTimeout(timer)
  }, [state])

  async function handleCopy() {
    try {
      await navigator.clipboard.writeText(value)
      setState("copied")
    } catch {
      setState("failed")
    }
  }

  const text =
    state === "copied" ? "Copied" : state === "failed" ? "Copy failed" : label
  const icon = state === "copied" ? <Check aria-hidden /> : <Copy aria-hidden />

  if (iconOnly) {
    return (
      <Button
        type="button"
        variant="ghost"
        size="icon-xs"
        onClick={() => void handleCopy()}
        aria-label={label}
        title={text}
        className="text-muted-foreground hover:text-foreground"
      >
        {icon}
        <span aria-live="polite" className="sr-only">
          {state === "idle" ? "" : text}
        </span>
      </Button>
    )
  }

  return (
    <Button
      type="button"
      variant="outline"
      size="sm"
      onClick={() => void handleCopy()}
      aria-live="polite"
    >
      {icon}
      {text}
    </Button>
  )
}

/** A read-only field with a copy button, for links people pass on. */
export function CopyField({
  value,
  label,
  copyLabel = "Copy link",
}: {
  value: string
  label: string
  copyLabel?: string
}) {
  return (
    <div className="flex flex-col gap-2 sm:flex-row">
      <Input
        readOnly
        aria-label={label}
        value={value}
        onFocus={(event) => event.currentTarget.select()}
        className="min-w-0 font-mono text-sm sm:flex-1"
      />
      <CopyButton value={value} label={copyLabel} />
    </div>
  )
}
