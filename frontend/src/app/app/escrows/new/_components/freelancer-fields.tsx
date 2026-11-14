"use client"

import Link from "next/link"
import { Notice } from "@/components/feedback-message"
import { FormSection } from "@/components/form-section"
import { SplitFields, type SplitRow } from "@/components/split-fields"
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import type { Wallet } from "@/lib/api/auth"
import type { SplitCheck } from "@/lib/escrow-rules"

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
  return (
    <FormSection
      title="Your payout"
      description="You are paid in USDC on NEAR."
    >
      {nearWallets.length === 0 ? (
        <Notice role="alert">
          To be paid, link a NEAR wallet to your account first.{" "}
          <Link
            href="/app/settings#wallets"
            className="font-medium underline underline-offset-4"
          >
            Link a wallet
          </Link>
        </Notice>
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

      <SplitFields
        split={split}
        onSplitChange={onSplitChange}
        check={check}
        showErrors={showErrors}
      />
    </FormSection>
  )
}
