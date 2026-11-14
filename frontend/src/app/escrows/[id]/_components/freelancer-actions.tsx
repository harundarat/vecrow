"use client"

import { useState } from "react"
import { LoaderCircle } from "lucide-react"
import { FeedbackMessage } from "@/components/feedback-message"
import { FormSection } from "@/components/form-section"
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Button } from "@/components/ui/button"
import {
  useFreelancerAction,
  type FreelancerStep,
} from "@/hooks/use-freelancer-action"
import {
  refundMilestones,
  submitMilestone,
  type DelegateBody,
  type EscrowDetail,
  type Milestone,
} from "@/lib/api/escrow"
import {
  refundCall,
  submitCall,
  type FreelancerCall,
} from "@/lib/delegate-action"
import { formatDuration } from "@/lib/format"
import { formatUsdc, sumUsdc } from "@/lib/usdc"
import { describeWalletError } from "@/lib/wallet-errors"

export const FREELANCER_STEP_TEXT: Record<FreelancerStep, string> = {
  connecting: "Connect your NEAR wallet…",
  signing: "Sign in your wallet. Vecrow pays the gas.",
  sending: "Sending…",
}

/** The freelancer's buttons for a milestone that is not submitted yet. */
export function FreelancerMilestoneActions({
  escrow,
  milestone,
  onDone,
}: {
  escrow: EscrowDetail
  milestone: Milestone
  onDone: () => void
}) {
  const number = milestone.index + 1
  const resubmit = milestone.status === "RevisionRequested"
  const window = escrow.review_window_sec
  const review = window
    ? `The client has ${formatDuration(window * 1000)} to review it. If they do nothing, it is paid out automatically.`
    : "The client then reviews it."

  return (
    <div className="flex flex-col gap-2 sm:flex-row sm:justify-end">
      {milestone.status === "Pending" && (
        <ConfirmedAction
          escrow={escrow}
          call={refundCall(escrow.id, milestone.index)}
          send={(body) => refundMilestones(escrow.id, body)}
          label="Refund"
          variant="outline"
          title={`Refund milestone ${number}?`}
          description={refundText(escrow, milestone.amount)}
          confirmLabel="Refund"
          onDone={onDone}
        />
      )}
      <ConfirmedAction
        escrow={escrow}
        call={submitCall(escrow.id, milestone.index)}
        send={(body) => submitMilestone(escrow.id, milestone.index, body)}
        label={resubmit ? "Resubmit" : "Submit"}
        title={`${resubmit ? "Resubmit" : "Submit"} milestone ${number}?`}
        description={review}
        confirmLabel="Submit"
        onDone={onDone}
      />
    </div>
  )
}

/** Refunds every milestone the freelancer has not submitted, in one call. */
export function RefundAllSection({
  escrow,
  onDone,
}: {
  escrow: EscrowDetail
  onDone: () => void
}) {
  const pending = escrow.milestones.filter((m) => m.status === "Pending")
  if (pending.length < 2) return null
  const total = sumUsdc(pending.map((m) => m.amount))
  return (
    <FormSection
      title="Refund unsubmitted work"
      description="Return every milestone you have not submitted to the client."
    >
      <ConfirmedAction
        escrow={escrow}
        call={refundCall(escrow.id, null)}
        send={(body) => refundMilestones(escrow.id, body)}
        label={`Refund all ${pending.length}`}
        variant="outline"
        title={`Refund ${pending.length} milestones?`}
        description={refundText(escrow, total)}
        confirmLabel="Refund all"
        onDone={onDone}
        className="self-start"
      />
    </FormSection>
  )
}

function refundText(escrow: EscrowDetail, amount: string) {
  const to = escrow.client?.refund_account
  return (
    <>
      {formatUsdc(amount)} USDC goes back to the client
      {to && (
        <>
          {" "}
          at <span className="font-mono break-all">{to}</span>
        </>
      )}
      . This cannot be undone.
    </>
  )
}

/** A button that asks first, then signs `call` as the freelancer and sends it. */
function ConfirmedAction({
  escrow,
  call,
  send,
  label,
  variant = "primary",
  title,
  description,
  confirmLabel,
  onDone,
  className,
}: {
  escrow: EscrowDetail
  call: FreelancerCall
  send: (body: DelegateBody) => Promise<unknown>
  label: string
  variant?: "primary" | "outline"
  title: string
  description: React.ReactNode
  confirmLabel: string
  onDone: () => void
  className?: string
}) {
  const act = useFreelancerAction(escrow)
  const [open, setOpen] = useState(false)
  const [step, setStep] = useState<FreelancerStep | null>(null)
  const [error, setError] = useState<string | null>(null)
  const busy = step !== null

  async function run() {
    if (busy) return
    setError(null)
    try {
      await act(call, send, setStep)
    } catch (err) {
      setError(describeWalletError(err))
    } finally {
      setOpen(false)
      setStep(null)
      // Shows the milestone as it is now, also when the call failed after
      // it was sent.
      onDone()
    }
  }

  return (
    <div className={className}>
      <Button
        variant={variant}
        size="sm"
        className="w-full sm:w-auto"
        onClick={() => {
          setError(null)
          setOpen(true)
        }}
      >
        {label}
      </Button>
      <FeedbackMessage
        feedback={error ? { kind: "error", message: error } : null}
      />
      <AlertDialog
        open={open}
        onOpenChange={(next) => {
          if (!next && !busy) setOpen(false)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{title}</AlertDialogTitle>
            <AlertDialogDescription>{description}</AlertDialogDescription>
          </AlertDialogHeader>
          {step && (
            <p aria-live="polite" className="text-sm text-muted-foreground">
              {FREELANCER_STEP_TEXT[step]}
            </p>
          )}
          <AlertDialogFooter>
            <AlertDialogCancel disabled={busy}>Not yet</AlertDialogCancel>
            <Button disabled={busy} onClick={() => void run()}>
              {busy && <LoaderCircle aria-hidden className="animate-spin" />}
              {confirmLabel}
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
