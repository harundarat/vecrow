"use client"

import Link from "next/link"
import { CircleAlert, Plus, Trash2 } from "lucide-react"
import { Button } from "@/components/ui/button"
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
  FieldLegend,
  FieldSet,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
  InputGroupText,
} from "@/components/ui/input-group"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import type { Wallet } from "@/lib/api/auth"
import {
  formatPercent,
  MAX_SPLIT_RECIPIENTS,
  type SplitCheck,
} from "@/lib/escrow-rules"
import { TOTAL_BPS } from "@/lib/payout"
import { cn } from "@/utils"
import { FormSection } from "./form-section"
import { newRowKey } from "./row-key"

export type SplitRow = { key: number; account: string; percent: string }

export function FreelancerFields({
  nearWallets,
  account,
  onAccountChange,
  split,
  onSplitChange,
  check,
  showErrors,
}: {
  nearWallets: Wallet[]
  account: string
  onAccountChange: (account: string) => void
  split: SplitRow[]
  onSplitChange: (rows: SplitRow[]) => void
  check: SplitCheck
  showErrors: boolean
}) {
  function update(key: number, patch: Partial<SplitRow>) {
    onSplitChange(
      split.map((row) => (row.key === key ? { ...row, ...patch } : row))
    )
  }

  const totalOk = check.totalBps === TOTAL_BPS

  return (
    <FormSection
      title="Your payout"
      description="You are paid in USDC on NEAR."
    >
      {nearWallets.length === 0 ? (
        <p
          role="alert"
          className="flex items-start gap-2 rounded-xl bg-destructive/10 p-3 text-sm text-destructive"
        >
          <CircleAlert aria-hidden className="mt-0.5 size-4 shrink-0" />
          <span>
            To be paid, link a NEAR wallet to your account first.{" "}
            <Link
              href="/app/settings#wallets"
              className="font-medium underline underline-offset-4"
            >
              Link a wallet
            </Link>
          </span>
        </p>
      ) : (
        <Field>
          <FieldLabel htmlFor="freelancer-account">
            Your NEAR account
          </FieldLabel>
          <Select
            items={nearWallets.map((w) => ({
              value: w.address,
              label: w.address,
            }))}
            value={account}
            onValueChange={(value) => onAccountChange(String(value ?? ""))}
          >
            <SelectTrigger id="freelancer-account" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {nearWallets.map((wallet) => (
                <SelectItem key={wallet.id} value={wallet.address}>
                  <span className="truncate">{wallet.address}</span>
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <FieldDescription>
            You sign the escrow with this wallet. Only NEAR wallets linked to
            your account are listed.
          </FieldDescription>
        </Field>
      )}

      <FieldSet className="gap-4">
        <FieldLegend variant="label" className="mb-1">
          Split
        </FieldLegend>
        <FieldDescription>
          Who gets each payment, and how much. The platform fee is taken first.
        </FieldDescription>
        <ol className="flex flex-col gap-4">
          {split.map((row, i) => {
            const accountError = showErrors ? check.accountErrors[i] : null
            const shareError = showErrors ? check.shareErrors[i] : null
            const accountId = `split-account-${row.key}`
            const shareId = `split-share-${row.key}`
            return (
              <li key={row.key} className="flex flex-col gap-2">
                <span className="text-sm text-muted-foreground">
                  Recipient {i + 1}
                </span>
                <div className="flex flex-wrap gap-2">
                  <Input
                    id={accountId}
                    aria-label={`Recipient ${i + 1} NEAR account`}
                    placeholder="alice.near"
                    autoComplete="off"
                    autoCapitalize="none"
                    spellCheck={false}
                    value={row.account}
                    onChange={(event) =>
                      update(row.key, { account: event.target.value })
                    }
                    aria-invalid={accountError ? true : undefined}
                    aria-describedby={
                      accountError ? `${accountId}-error` : undefined
                    }
                    className="min-w-0 basis-full sm:flex-1 sm:basis-0"
                  />
                  <InputGroup className="w-28 flex-1 sm:flex-none">
                    <InputGroupInput
                      id={shareId}
                      aria-label={`Recipient ${i + 1} share`}
                      inputMode="decimal"
                      autoComplete="off"
                      placeholder="0"
                      value={row.percent}
                      onChange={(event) =>
                        update(row.key, { percent: event.target.value })
                      }
                      aria-invalid={shareError ? true : undefined}
                      aria-describedby={
                        shareError ? `${shareId}-error` : undefined
                      }
                    />
                    <InputGroupAddon align="inline-end">
                      <InputGroupText>%</InputGroupText>
                    </InputGroupAddon>
                  </InputGroup>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    aria-label={`Remove recipient ${i + 1}`}
                    disabled={split.length === 1}
                    onClick={() =>
                      onSplitChange(
                        split.filter((other) => other.key !== row.key)
                      )
                    }
                  >
                    <Trash2 aria-hidden />
                  </Button>
                </div>
                <FieldError id={`${accountId}-error`}>
                  {accountError}
                </FieldError>
                <FieldError id={`${shareId}-error`}>{shareError}</FieldError>
              </li>
            )
          })}
        </ol>
        <div className="flex flex-wrap items-center justify-between gap-3">
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={split.length >= MAX_SPLIT_RECIPIENTS}
            onClick={() =>
              onSplitChange([
                ...split,
                { key: newRowKey(), account: "", percent: "" },
              ])
            }
          >
            <Plus aria-hidden />
            Add recipient
          </Button>
          <p
            aria-live="polite"
            className={cn(
              "text-sm",
              totalOk ? "text-muted-foreground" : "text-destructive"
            )}
          >
            Total {formatPercent(check.totalBps)} of 100%
          </p>
        </div>
        {showErrors && check.listError && (
          <FieldError>{check.listError}</FieldError>
        )}
        <FieldDescription>
          Up to {MAX_SPLIT_RECIPIENTS} NEAR accounts, each listed once.
        </FieldDescription>
      </FieldSet>
    </FormSection>
  )
}
