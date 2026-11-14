"use client"

import { Input } from "@/components/ui/input"
import { cn } from "@/utils"

export const CODE_LENGTH = 6

type OneTimeCodeInputProps = Omit<
  React.ComponentProps<typeof Input>,
  "value" | "onChange" | "type"
> & {
  value: string
  onValueChange: (value: string) => void
}

/** A 6-digit code field that keeps digits only, so pasted "123 456" works. */
export function OneTimeCodeInput({
  value,
  onValueChange,
  className,
  ...props
}: OneTimeCodeInputProps) {
  return (
    <Input
      type="text"
      inputMode="numeric"
      autoComplete="one-time-code"
      spellCheck={false}
      placeholder="000000"
      value={value}
      onChange={(event) =>
        onValueChange(
          event.target.value.replace(/\D/g, "").slice(0, CODE_LENGTH)
        )
      }
      className={cn("h-11 font-mono text-lg tracking-[0.4em]", className)}
      {...props}
    />
  )
}
