"use client"

import { useState } from "react"
import { LoaderCircle } from "lucide-react"
import { FeedbackMessage } from "@/components/feedback-message"
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
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
} from "@/components/ui/field"
import { Textarea } from "@/components/ui/textarea"
import {
  useApproverAction,
  type ApproverStep,
} from "@/hooks/use-approver-action"
import {
  approveMilestone,
  requestRevision,
  type EscrowDetail,
  type Milestone,
} from "@/lib/api/escrow"
import { milestoneActionMessage } from "@/lib/approver-action"
import { MAX_REVISIONS, revisionReasonError } from "@/lib/escrow-rules"
import { mulBps } from "@/lib/payout"
import { formatUsdc } from "@/lib/usdc"
import { describeWalletError } from "@/lib/wallet-errors"

type Busy = { kind: "approve" | "revision"; step: ApproverStep }

const STEP_TEXT: Record<ApproverStep, string> = {
  connecting: "Connect your approver wallet…",
  signing: "Sign in your wallet. It is free.",
  sending: "Sending…",
}

/** The approver's buttons for a submitted milestone. */
export function MilestoneActions({
  escrow,
  milestone,
  onDone,
}: {
  escrow: EscrowDetail
  milestone: Milestone
  onDone: () => void
}) {
  const act = useApproverAction(escrow)
  const [busy, setBusy] = useState<Busy | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [confirming, setConfirming] = useState(false)
  const [reason, setReason] = useState<string | null>(null)
  const [showErrors, setShowErrors] = useState(false)

  const index = milestone.index
  const number = index + 1
  const left = Math.max(0, MAX_REVISIONS - milestone.revision_count)
  const fee = mulBps(BigInt(milestone.amount), escrow.fee_bps).toString()
  const reasonError = reason === null ? null : revisionReasonError(reason)

  async function run(kind: Busy["kind"], text: string | null) {
    if (busy) return
    setError(null)
    try {
      await act(
        (nonce, deadline) =>
          milestoneActionMessage(
            kind === "approve" ? "approve_milestone" : "request_revision",
            escrow.contract_id,
            escrow.id,
            index,
            nonce,
            deadline
          ),
        (signature) =>
          kind === "approve"
            ? approveMilestone(escrow.id, index, signature)
            : requestRevision(escrow.id, index, {
                ...signature,
                reason: text ?? "",
              }),
        (step) => setBusy({ kind, step })
      )
      setConfirming(false)
      setReason(null)
      onDone()
    } catch (err) {
      setError(describeWalletError(err))
      setConfirming(false)
      // Shows the milestone as it is now, in case the action went through.
      onDone()
    } finally {
      setBusy(null)
    }
  }

  function submitRevision(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setShowErrors(true)
    if (reason === null || reasonError) return
    void run("revision", reason.trim())
  }

  const status = busy ? STEP_TEXT[busy.step] : null
  const reasonId = `revision-reason-${index}`

  return (
    <div className="flex flex-col gap-3">
      {reason !== null ? (
        <form
          onSubmit={submitRevision}
          noValidate
          className="flex flex-col gap-3 rounded-xl border p-3"
        >
          <Field data-invalid={showErrors && reasonError ? true : undefined}>
            <FieldLabel htmlFor={reasonId}>What needs to change?</FieldLabel>
            <Textarea
              id={reasonId}
              value={reason}
              onChange={(event) => setReason(event.target.value)}
              disabled={busy !== null}
              aria-invalid={showErrors && reasonError ? true : undefined}
              aria-describedby={`${reasonId}-hint ${reasonId}-error`}
            />
            <FieldDescription id={`${reasonId}-hint`}>
              The freelancer sees this. {revisionsLeftText(left)}
            </FieldDescription>
            <FieldError id={`${reasonId}-error`}>
              {showErrors ? reasonError : null}
            </FieldError>
          </Field>
          {busy?.kind === "revision" && (
            <p aria-live="polite" className="text-sm text-muted-foreground">
              {status}
            </p>
          )}
          <div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
            <Button
              type="button"
              variant="outline"
              disabled={busy !== null}
              onClick={() => {
                setReason(null)
                setShowErrors(false)
              }}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={busy !== null}>
              {busy?.kind === "revision" && (
                <LoaderCircle aria-hidden className="animate-spin" />
              )}
              Send request
            </Button>
          </div>
        </form>
      ) : (
        <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
          <p className="text-xs text-muted-foreground">
            {revisionsLeftText(left)}
          </p>
          <div className="flex flex-col-reverse gap-2 sm:flex-row">
            {left > 0 && (
              <Button
                variant="outline"
                size="sm"
                disabled={busy !== null}
                onClick={() => {
                  setError(null)
                  setShowErrors(false)
                  setReason("")
                }}
              >
                Request revision
              </Button>
            )}
            <Button
              size="sm"
              disabled={busy !== null}
              onClick={() => {
                setError(null)
                setConfirming(true)
              }}
            >
              Approve
            </Button>
          </div>
        </div>
      )}

      <FeedbackMessage
        feedback={error ? { kind: "error", message: error } : null}
      />

      <AlertDialog
        open={confirming}
        onOpenChange={(open) => {
          if (!open && !busy) setConfirming(false)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Approve milestone {number}?</AlertDialogTitle>
            <AlertDialogDescription>
              {formatUsdc(milestone.amount)} USDC is paid out now:{" "}
              {formatUsdc(fee)} USDC platform fee, the rest to the freelancer.
              This cannot be undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          {busy?.kind === "approve" && (
            <p aria-live="polite" className="text-sm text-muted-foreground">
              {status}
            </p>
          )}
          <AlertDialogFooter>
            <AlertDialogCancel disabled={busy !== null}>
              Not yet
            </AlertDialogCancel>
            <Button
              disabled={busy !== null}
              onClick={() => void run("approve", null)}
            >
              {busy?.kind === "approve" && (
                <LoaderCircle aria-hidden className="animate-spin" />
              )}
              Approve and pay
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

function revisionsLeftText(left: number): string {
  if (left === 0) {
    return "No revisions left. Approve it, or it is released by itself."
  }
  return `${left} of ${MAX_REVISIONS} revisions left.`
}
