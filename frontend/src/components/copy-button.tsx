"use client"

import { useEffect, useState } from "react"
import { Check, Copy } from "lucide-react"
import { Button } from "@/components/ui/button"

type CopyState = "idle" | "copied" | "failed"

/** Copies `value` to the clipboard and says so for a moment. */
export function CopyButton({
  value,
  label = "Copy",
}: {
  value: string
  label?: string
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

  return (
    <Button
      type="button"
      variant="outline"
      size="sm"
      onClick={() => void handleCopy()}
      aria-live="polite"
    >
      {state === "copied" ? <Check aria-hidden /> : <Copy aria-hidden />}
      {state === "copied"
        ? "Copied"
        : state === "failed"
          ? "Copy failed"
          : label}
    </Button>
  )
}
