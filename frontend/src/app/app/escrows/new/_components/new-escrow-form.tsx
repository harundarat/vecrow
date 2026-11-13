"use client"

import Link from "next/link"
import { useRef, useState } from "react"
import { LoaderCircle } from "lucide-react"
import { FeedbackMessage } from "@/components/feedback-message"
import {
  followAccount,
  splitIsValid,
  splitShares,
  type SplitRow,
} from "@/components/split-fields"
import { Button, buttonVariants } from "@/components/ui/button"
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldLabel,
  FieldTitle,
} from "@/components/ui/field"
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group"
import { useAuth } from "@/context/auth"
import { useWalletSigner } from "@/hooks/use-wallet-signer"
import { proveWallet, type Account, type Wallet } from "@/lib/api/auth"
import {
  createEscrow,
  type Approver,
  type CreatedEscrow,
  type CreateEscrowRequest,
  type CreatorRole,
} from "@/lib/api/escrow"
import {
  checkMilestones,
  checkSplit,
  nearAccountError,
} from "@/lib/escrow-rules"
import {
  invitationBody,
  invitationError,
  NO_INVITE,
  type InviteState,
} from "@/lib/invitation"
import { describeWalletError } from "@/lib/wallet-errors"
import { ClientFields } from "./client-fields"
import { EscrowSummary } from "./escrow-summary"
import { FormSection } from "@/components/form-section"
import { FreelancerFields } from "./freelancer-fields"
import { InvitationFields } from "./invitation-fields"
import { MilestonesFields, type MilestoneRow } from "./milestones-fields"
import { newRowKey } from "@/lib/row-key"

const ROLES: Array<{ value: CreatorRole; title: string; description: string }> =
  [
    {
      value: "freelancer",
      title: "Freelancer",
      description: "I do the work and get paid.",
    },
    {
      value: "client",
      title: "Client",
      description: "I pay for the work.",
    },
  ]

type Step = "signing" | "saving"

export function NewEscrowForm({
  account,
  onCreated,
}: {
  account: Account
  onCreated: (escrow: CreatedEscrow) => void
}) {
  const { request } = useAuth()
  const nearSigner = useWalletSigner("near")
  const formRef = useRef<HTMLFormElement>(null)

  const nearWallets = account.wallets.filter((w) => w.scheme === "nep413")
  const defaultNear = nearWallets[0]?.address ?? ""
  // Wallets other than NEAR need no extra step to be used as the approver.
  const defaultApprover =
    account.wallets.find((w) => w.scheme !== "nep413") ?? account.wallets[0]

  const [role, setRole] = useState<CreatorRole>("freelancer")
  const [milestones, setMilestones] = useState<MilestoneRow[]>(() => [
    { key: newRowKey(), amount: "" },
  ])
  const [freelancer, setFreelancer] = useState(defaultNear)
  const [split, setSplit] = useState<SplitRow[]>(() => [
    { key: newRowKey(), account: defaultNear, percent: "100" },
  ])
  const [approverId, setApproverId] = useState(defaultApprover?.id ?? "")
  const [refundAccount, setRefundAccount] = useState(defaultNear)
  const [invite, setInvite] = useState<InviteState>(NO_INVITE)
  const [showErrors, setShowErrors] = useState(false)
  const [step, setStep] = useState<Step | null>(null)
  const [error, setError] = useState<string | null>(null)

  const approver = account.wallets.find((w) => w.id === approverId)
  const milestoneCheck = checkMilestones(milestones.map((m) => m.amount))
  const splitCheck = checkSplit(
    split.map((row) => ({ account: row.account, percent: row.percent }))
  )
  const refundError = role === "client" ? nearAccountError(refundAccount) : null
  const inviteError = invitationError(account, invite)

  const splitValid = splitIsValid(splitCheck)
  const roleValid =
    role === "freelancer"
      ? Boolean(freelancer) && splitValid
      : Boolean(approver) && !refundError
  const valid =
    !milestoneCheck.listError &&
    milestoneCheck.errors.every((e) => e === null) &&
    roleValid &&
    !inviteError

  function changeFreelancer(next: string) {
    setSplit((rows) => followAccount(rows, freelancer, next))
    setFreelancer(next)
  }

  async function nearApprover(wallet: Wallet): Promise<Approver> {
    // The key is read from a signature so it is the one the wallet will sign
    // the terms with later.
    if (nearSigner.address && nearSigner.address !== wallet.address) {
      await nearSigner.disconnect()
    }
    const connected = await nearSigner.connect()
    if (connected !== wallet.address) {
      throw new Error(
        `Your NEAR wallet is on ${connected}. Switch to ${wallet.address} and try again.`
      )
    }
    const proof = await proveWallet({
      scheme: "nep413",
      address: wallet.address,
      sign: nearSigner.sign,
    })
    if (!proof.public_key) {
      throw new Error("Your NEAR wallet did not share its public key.")
    }
    return {
      scheme: "nep413",
      account_id: wallet.address,
      public_key: proof.public_key,
    }
  }

  async function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (step) return
    setShowErrors(true)
    setError(null)
    if (!valid) {
      // Wait for the errors to render, then move to the first one.
      setTimeout(() => {
        formRef.current
          ?.querySelector<HTMLElement>('[aria-invalid="true"], [role="alert"]')
          ?.focus()
        formRef.current
          ?.querySelector('[aria-invalid="true"], [role="alert"]')
          ?.scrollIntoView({ block: "center", behavior: "smooth" })
      })
      return
    }

    const amounts = milestoneCheck.amounts as string[]
    const invitation = invitationBody(invite)
    try {
      let body: CreateEscrowRequest
      if (role === "freelancer") {
        body = {
          role,
          milestones: amounts,
          freelancer,
          split: splitShares(split, splitCheck),
          invitation,
        }
      } else {
        const wallet = approver as Wallet
        let approverBody: Approver
        if (wallet.scheme === "nep413") {
          setStep("signing")
          approverBody = await nearApprover(wallet)
        } else {
          approverBody = { scheme: wallet.scheme, address: wallet.address }
        }
        body = {
          role,
          milestones: amounts,
          approver: approverBody,
          refund_account: refundAccount,
          invitation,
        }
      }
      setStep("saving")
      onCreated(await createEscrow(request, body))
    } catch (err) {
      setError(describeWalletError(err))
    } finally {
      setStep(null)
    }
  }

  const otherRole = role === "freelancer" ? "client" : "freelancer"

  return (
    <form
      ref={formRef}
      onSubmit={handleSubmit}
      noValidate
      className="flex flex-col gap-6"
    >
      <FormSection title="Your role">
        <RadioGroup
          value={role}
          onValueChange={(value) => setRole(value as CreatorRole)}
          aria-label="Your role"
          className="grid gap-3 sm:grid-cols-2"
        >
          {ROLES.map((option) => (
            <FieldLabel key={option.value} htmlFor={`role-${option.value}`}>
              <Field orientation="horizontal">
                <FieldContent>
                  <FieldTitle>{option.title}</FieldTitle>
                  <FieldDescription>{option.description}</FieldDescription>
                </FieldContent>
                <RadioGroupItem
                  value={option.value}
                  id={`role-${option.value}`}
                />
              </Field>
            </FieldLabel>
          ))}
        </RadioGroup>
      </FormSection>

      <MilestonesFields
        rows={milestones}
        onChange={setMilestones}
        check={milestoneCheck}
        showErrors={showErrors}
      />

      {role === "freelancer" ? (
        <FreelancerFields
          nearWallets={nearWallets}
          account={freelancer}
          onAccountChange={changeFreelancer}
          split={split}
          onSplitChange={setSplit}
          check={splitCheck}
          showErrors={showErrors}
        />
      ) : (
        <ClientFields
          wallets={account.wallets}
          approverId={approverId}
          onApproverChange={setApproverId}
          refundAccount={refundAccount}
          onRefundAccountChange={setRefundAccount}
          refundError={showErrors ? refundError : null}
        />
      )}

      <InvitationFields
        invite={invite}
        onChange={setInvite}
        otherRole={otherRole}
        error={showErrors ? inviteError : null}
      />

      <EscrowSummary
        role={role}
        amounts={milestoneCheck.amounts}
        split={
          role === "freelancer" && splitValid
            ? splitShares(split, splitCheck)
            : null
        }
      />

      <div className="flex flex-col gap-3">
        {showErrors && !valid && (
          <p aria-live="polite" className="text-sm text-destructive">
            Fix the fields marked in red, then try again.
          </p>
        )}
        <FeedbackMessage
          feedback={error ? { kind: "error", message: error } : null}
        />
        <p
          aria-live="polite"
          className="text-sm text-muted-foreground empty:hidden"
        >
          {step === "signing" &&
            "Sign the message in your NEAR wallet. It is free and sends nothing."}
        </p>
        <div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
          <Link href="/app" className={buttonVariants({ variant: "outline" })}>
            Cancel
          </Link>
          <Button type="submit" disabled={step !== null}>
            {step && <LoaderCircle aria-hidden className="animate-spin" />}
            Create escrow
          </Button>
        </div>
      </div>
    </form>
  )
}
