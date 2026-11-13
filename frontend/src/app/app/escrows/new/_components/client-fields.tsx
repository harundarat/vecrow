"use client"

import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import type { Wallet } from "@/lib/api/auth"
import { shortenAddress, WALLET_SCHEME_LABEL } from "@/lib/format"
import { FormSection } from "./form-section"

export function walletLabel(wallet: Wallet) {
  return `${WALLET_SCHEME_LABEL[wallet.scheme] ?? wallet.scheme} · ${shortenAddress(wallet.address)}`
}

export function ClientFields({
  wallets,
  approverId,
  onApproverChange,
  refundAccount,
  onRefundAccountChange,
  refundError,
}: {
  wallets: Wallet[]
  approverId: string
  onApproverChange: (id: string) => void
  refundAccount: string
  onRefundAccountChange: (account: string) => void
  refundError: string | null
}) {
  const approver = wallets.find((wallet) => wallet.id === approverId)

  return (
    <FormSection
      title="Your wallets"
      description="You pay into the escrow and approve each milestone."
    >
      <Field>
        <FieldLabel htmlFor="approver-wallet">Approver wallet</FieldLabel>
        <Select
          items={wallets.map((w) => ({ value: w.id, label: walletLabel(w) }))}
          value={approverId}
          onValueChange={(value) => onApproverChange(String(value ?? ""))}
        >
          <SelectTrigger id="approver-wallet" className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {wallets.map((wallet) => (
              <SelectItem key={wallet.id} value={wallet.id}>
                {walletLabel(wallet)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <FieldDescription>
          You approve milestones by signing with this wallet. It cannot be
          changed later.
          {approver?.scheme === "nep413" &&
            " When you create the escrow, your NEAR wallet asks you to sign a short message. It is free."}
        </FieldDescription>
      </Field>

      <Field data-invalid={refundError ? true : undefined}>
        <FieldLabel htmlFor="refund-account">Refund account</FieldLabel>
        <Input
          id="refund-account"
          placeholder="alice.near"
          autoComplete="off"
          autoCapitalize="none"
          spellCheck={false}
          value={refundAccount}
          onChange={(event) => onRefundAccountChange(event.target.value)}
          aria-invalid={refundError ? true : undefined}
          aria-describedby="refund-account-hint refund-account-error"
        />
        <FieldDescription id="refund-account-hint">
          A NEAR account. If the work stops, money for unfinished milestones
          comes back here.
        </FieldDescription>
        <FieldError id="refund-account-error">{refundError}</FieldError>
      </Field>
    </FormSection>
  )
}
