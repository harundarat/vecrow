"use client"

import { Field, FieldError, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import type { WalletScheme } from "@/lib/api/auth"
import { WALLET_SCHEME_LABEL } from "@/lib/format"
import type { InviteKind, InviteState } from "@/lib/invitation"

const KIND_ITEMS: Array<{ value: InviteKind; label: string }> = [
  { value: "none", label: "Don't invite" },
  { value: "username", label: "Username" },
  { value: "email", label: "Email" },
  { value: "wallet", label: "Wallet address" },
]

const SCHEMES: WalletScheme[] = ["nep413", "eip191", "ed25519"]

const PLACEHOLDER: Record<WalletScheme, string> = {
  nep413: "alice.near",
  eip191: "0x…",
  ed25519: "Solana address",
}

/** Picks how to name the other party, then takes the value. */
export function InvitationInputs({
  invite,
  onChange,
  error,
}: {
  invite: InviteState
  onChange: (invite: InviteState) => void
  error: string | null
}) {
  const label =
    invite.kind === "username"
      ? "Their username"
      : invite.kind === "email"
        ? "Their email"
        : "Their wallet"

  return (
    <>
      <Field>
        <FieldLabel htmlFor="invite-kind">Invite by</FieldLabel>
        <Select
          items={KIND_ITEMS}
          value={invite.kind}
          onValueChange={(value) =>
            onChange({
              ...invite,
              kind: (value ?? "none") as InviteKind,
              value: "",
            })
          }
        >
          <SelectTrigger id="invite-kind" className="w-full sm:w-56">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {KIND_ITEMS.map((item) => (
              <SelectItem key={item.value} value={item.value}>
                {item.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </Field>

      {invite.kind !== "none" && (
        <Field data-invalid={error ? true : undefined}>
          <FieldLabel htmlFor="invite-value">{label}</FieldLabel>
          <div className="flex flex-col gap-2 sm:flex-row">
            {invite.kind === "wallet" && (
              <Select
                items={SCHEMES.map((s) => ({
                  value: s,
                  label: WALLET_SCHEME_LABEL[s],
                }))}
                value={invite.scheme}
                onValueChange={(value) =>
                  onChange({
                    ...invite,
                    scheme: (value ?? "nep413") as WalletScheme,
                  })
                }
              >
                <SelectTrigger
                  aria-label="Wallet network"
                  className="w-full sm:w-32"
                >
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {SCHEMES.map((scheme) => (
                    <SelectItem key={scheme} value={scheme}>
                      {WALLET_SCHEME_LABEL[scheme]}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
            <Input
              id="invite-value"
              type={invite.kind === "email" ? "email" : "text"}
              placeholder={
                invite.kind === "username"
                  ? "alice"
                  : invite.kind === "email"
                    ? "name@example.com"
                    : PLACEHOLDER[invite.scheme]
              }
              autoComplete="off"
              autoCapitalize="none"
              spellCheck={false}
              value={invite.value}
              onChange={(event) =>
                onChange({ ...invite, value: event.target.value })
              }
              aria-invalid={error ? true : undefined}
              aria-describedby={error ? "invite-value-error" : undefined}
              className="min-w-0 sm:flex-1"
            />
          </div>
          <FieldError id="invite-value-error">{error}</FieldError>
        </Field>
      )}
    </>
  )
}
