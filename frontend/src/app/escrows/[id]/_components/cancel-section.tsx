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
import { useAuth } from "@/context/auth"
import {
  useApproverAction,
  type ApproverStep,
} from "@/hooks/use-approver-action"
import {
  cancelAsApprover,
  cancelEscrow,
  type EscrowDetail,
} from "@/lib/api/escrow"
import { cancelMessage } from "@/lib/approver-action"
import { describeWalletError } from "@/lib/wallet-errors"

const STEP_TEXT: Record<ApproverStep, string> = {
  connecting: "Connect your approver wallet…",
  signing: "Sign the cancellation in your wallet. It is free.",
  sending: "Cancelling…",
}

/**
 * Cancels an escrow before it is paid in: the creator while it is not
 * on-chain, the approver with a signature once it is Created.
 */
export function CancelSection({
  escrow,
  asApprover,
  onDone,
}: {
  escrow: EscrowDetail
  asApprover: boolean
  onDone: () => void
}) {
  const { request } = useAuth()
  const act = useApproverAction(escrow)
  const [open, setOpen] = useState(false)
  const [busy, setBusy] = useState(false)
  const [step, setStep] = useState<ApproverStep | null>(null)
  const [error, setError] = useState<string | null>(null)

  async function cancel() {
    if (busy) return
    setBusy(true)
    setError(null)
    try {
      if (asApprover) {
        await act(
          (nonce, deadline) =>
            cancelMessage(escrow.contract_id, escrow.id, nonce, deadline),
          (signature) => cancelAsApprover(escrow.id, signature),
          setStep
        )
      } else {
        await cancelEscrow(request, escrow.id)
      }
      setOpen(false)
      onDone()
    } catch (err) {
      setError(describeWalletError(err))
      setOpen(false)
      // Shows the escrow as it is now, in case the cancellation went through.
      onDone()
    } finally {
      setBusy(false)
      setStep(null)
    }
  }

  return (
    <FormSection
      title="Cancel escrow"
      description={
        asApprover
          ? "Nothing is paid in yet, so cancelling costs nothing. You sign it with the approver wallet."
          : "Nothing is on-chain yet, so cancelling is free. Nobody can join after this."
      }
    >
      <FeedbackMessage
        feedback={error ? { kind: "error", message: error } : null}
      />
      <Button
        variant="destructive"
        className="self-start"
        onClick={() => setOpen(true)}
      >
        Cancel escrow
      </Button>

      <AlertDialog
        open={open}
        onOpenChange={(next) => {
          if (!next && !busy) setOpen(false)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Cancel this escrow?</AlertDialogTitle>
            <AlertDialogDescription>
              {asApprover
                ? "Nobody can pay into it after this. If you already sent a payment, wait for it to arrive instead. This cannot be undone."
                : "Nobody can join it after this. This cannot be undone."}
            </AlertDialogDescription>
          </AlertDialogHeader>
          {step && (
            <p aria-live="polite" className="text-sm text-muted-foreground">
              {STEP_TEXT[step]}
            </p>
          )}
          <AlertDialogFooter>
            <AlertDialogCancel disabled={busy}>Keep escrow</AlertDialogCancel>
            <Button
              variant="destructive"
              disabled={busy}
              onClick={() => void cancel()}
            >
              {busy && <LoaderCircle aria-hidden className="animate-spin" />}
              Cancel escrow
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </FormSection>
  )
}
