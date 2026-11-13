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
  connectWalletAs,
  SCHEME_CHAIN,
  useWalletSigners,
} from "@/hooks/use-wallet-signer"
import {
  approveSubmission,
  rejectSubmission,
  type EscrowDetail,
  type Submission,
  type WalletRef,
} from "@/lib/api/escrow"
import { formatPercent } from "@/lib/escrow-rules"
import {
  formatDate,
  formatTime,
  shortenAddress,
  WALLET_SCHEME_LABEL,
} from "@/lib/format"
import { assertTermsRequest } from "@/lib/terms"
import { describeWalletError } from "@/lib/wallet-errors"
import { approverLabel, EscrowWarnings } from "./escrow-details"

type Step = "connecting" | "signing" | "sending" | "rejecting" | "done"

const STEP_TEXT: Partial<Record<Step, string>> = {
  connecting: "Connect your wallet…",
  signing: "Sign the terms in your wallet. It is free and sends nothing.",
  sending: "Approving…",
}

/** The creator checks what the other party sent, then approves or rejects. */
export function ReviewSection({
  escrow,
  submission,
  onDone,
}: {
  escrow: EscrowDetail
  submission: Submission
  onDone: () => void
}) {
  const { request } = useAuth()
  const signers = useWalletSigners()
  const [step, setStep] = useState<Step | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [confirmReject, setConfirmReject] = useState(false)

  const { terms } = submission
  const otherRole =
    escrow.creator_role === "freelancer" ? "client" : "freelancer"
  // The creator signs with the wallet their own details name.
  const creatorWallet: WalletRef =
    escrow.creator_role === "freelancer"
      ? { scheme: "nep413", address: terms.freelancer }
      : {
          scheme: terms.approver.scheme,
          address: terms.approver.account_id ?? terms.approver.address ?? "",
        }
  const deadline = new Date(terms.deadline_ms).toISOString()

  async function approve() {
    if (step) return
    setError(null)
    try {
      const signer = signers[SCHEME_CHAIN[creatorWallet.scheme]]
      setStep("connecting")
      await connectWalletAs(signer, creatorWallet.address)
      assertTermsRequest(submission, escrow.contract_id)
      setStep("signing")
      const signed = await signer.sign(submission)
      setStep("sending")
      await approveSubmission(
        request,
        escrow.id,
        escrow.creator_role === "freelancer"
          ? signed
          : { signature: signed.signature }
      )
      setStep("done")
      onDone()
    } catch (err) {
      setError(describeWalletError(err))
      setStep(null)
    }
  }

  async function reject() {
    if (step) return
    setError(null)
    setStep("rejecting")
    try {
      await rejectSubmission(request, escrow.id)
      setStep("done")
      setConfirmReject(false)
      onDone()
    } catch (err) {
      setError(describeWalletError(err))
      setConfirmReject(false)
      setStep(null)
    }
  }

  return (
    <FormSection
      title="Review and approve"
      description={`The ${otherRole} sent their details and signed. Check them, then approve or reject.`}
    >
      <EscrowWarnings warnings={escrow.warnings} />

      <dl className="grid gap-3 text-sm sm:grid-cols-2">
        <Detail label="Signed with">
          {WALLET_SCHEME_LABEL[submission.wallet.scheme]}{" "}
          <span className="font-mono break-all">
            {submission.wallet.address}
          </span>
        </Detail>
        {otherRole === "client" ? (
          <>
            <Detail label="Approver wallet">
              <span className="font-mono break-all">
                {approverLabel(terms.approver)}
              </span>
            </Detail>
            <Detail label="Refund account">
              <span className="font-mono break-all">
                {terms.refund_account}
              </span>
            </Detail>
          </>
        ) : (
          <>
            <Detail label="NEAR account">
              <span className="font-mono break-all">{terms.freelancer}</span>
            </Detail>
            <Detail label="Split">
              <ul>
                {terms.split.map((share) => (
                  <li key={share.account_id} className="break-all">
                    {formatPercent(share.bps)} to{" "}
                    <span className="font-mono">{share.account_id}</span>
                  </li>
                ))}
              </ul>
            </Detail>
          </>
        )}
        <Detail label="Approve before">
          {formatDate(deadline)}, {formatTime(deadline)}
        </Detail>
      </dl>

      <FeedbackMessage
        feedback={error ? { kind: "error", message: error } : null}
      />
      <p aria-live="polite" className="text-sm text-muted-foreground">
        {(step && STEP_TEXT[step]) ??
          `Your ${WALLET_SCHEME_LABEL[creatorWallet.scheme]} wallet ${shortenAddress(creatorWallet.address)} signs the same terms. It is free.`}
      </p>
      <div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
        <Button
          variant="outline"
          disabled={step !== null}
          onClick={() => setConfirmReject(true)}
        >
          Reject
        </Button>
        <Button disabled={step !== null} onClick={() => void approve()}>
          {step && step !== "rejecting" && (
            <LoaderCircle aria-hidden className="animate-spin" />
          )}
          Approve
        </Button>
      </div>

      <AlertDialog
        open={confirmReject}
        onOpenChange={(open) => {
          if (!open && step !== "rejecting") setConfirmReject(false)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Reject these details?</AlertDialogTitle>
            <AlertDialogDescription>
              The escrow opens again, and anyone with the link can join.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={step === "rejecting"}>
              Keep
            </AlertDialogCancel>
            <Button
              variant="destructive"
              disabled={step === "rejecting"}
              onClick={() => void reject()}
            >
              {step === "rejecting" && (
                <LoaderCircle aria-hidden className="animate-spin" />
              )}
              Reject
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </FormSection>
  )
}

function Detail({
  label,
  children,
}: React.PropsWithChildren<{ label: string }>) {
  return (
    <div className="flex min-w-0 flex-col gap-0.5">
      <dt className="text-muted-foreground">{label}</dt>
      <dd>{children}</dd>
    </div>
  )
}
