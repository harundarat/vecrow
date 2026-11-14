"use client"

import { LoaderCircle } from "lucide-react"
import { FeedbackMessage } from "@/components/feedback-message"
import { FormSection } from "@/components/form-section"
import { Button } from "@/components/ui/button"
import type { EscrowDetail, SubmitResponse } from "@/lib/api/escrow"
import type { GuestAccess } from "@/lib/guest-access"
import { JoinClientForm } from "./join-client-form"
import { JoinFreelancerForm } from "./join-freelancer-form"

export type JoinStep = "connecting" | "key" | "signing" | "sending" | "done"

const STEP_TEXT: Record<JoinStep, string> = {
  connecting: "Connect your wallet…",
  key: "Sign the first message in your NEAR wallet. It shares your key.",
  signing: "Sign the terms in your wallet. It is free and sends nothing.",
  sending: "Sending your details…",
  done: "Sent.",
}

/** Joining an Open escrow as the role its creator did not take. */
export function JoinSection({
  escrow,
  rejoin,
  onJoined,
}: {
  escrow: EscrowDetail
  /** The visitor joined before, but that submission is gone. */
  rejoin: boolean
  onJoined: (access: GuestAccess) => void
}) {
  const role = escrow.creator_role === "freelancer" ? "client" : "freelancer"

  function joined(res: SubmitResponse, fallback: GuestAccess["wallet"]) {
    onJoined({
      token: res.guest_token,
      expires_at: res.expires_at,
      wallet: res.escrow.submission?.wallet ?? fallback,
    })
  }

  return (
    <FormSection
      title={`Join as the ${role}`}
      description={
        role === "client"
          ? "You pay into the escrow and approve each milestone. No account needed."
          : "You do the work and get paid in USDC on NEAR. No account needed."
      }
    >
      {rejoin && (
        <p className="rounded-xl bg-primary/10 p-3 text-sm">
          Your last details were not approved. You can send them again.
        </p>
      )}
      {role === "client" ? (
        <JoinClientForm escrow={escrow} onJoined={joined} />
      ) : (
        <JoinFreelancerForm escrow={escrow} onJoined={joined} />
      )}
    </FormSection>
  )
}

/** The error, the current step and the submit button of a join form. */
export function JoinFooter({
  step,
  error,
}: {
  step: JoinStep | null
  error: string | null
}) {
  return (
    <div className="flex flex-col gap-3">
      <FeedbackMessage
        feedback={error ? { kind: "error", message: error } : null}
      />
      <p
        aria-live="polite"
        className="text-sm text-muted-foreground empty:hidden"
      >
        {step && STEP_TEXT[step]}
      </p>
      <Button
        type="submit"
        disabled={step !== null}
        className="self-stretch sm:self-end"
      >
        {step && <LoaderCircle aria-hidden className="animate-spin" />}
        Sign and join
      </Button>
    </div>
  )
}
