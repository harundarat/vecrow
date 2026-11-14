"use client"

import { useState } from "react"
import { LoaderCircle } from "lucide-react"
import { FeedbackMessage, type Feedback } from "@/components/feedback-message"
import { FormSection } from "@/components/form-section"
import { InvitationInputs } from "@/components/invitation-inputs"
import { Button } from "@/components/ui/button"
import { useAuth } from "@/context/auth"
import {
  updateInvitation,
  type EscrowDetail,
  type InvitationInput,
} from "@/lib/api/escrow"
import {
  describeInvitation,
  invitationBody,
  invitationError,
  inviteStateFrom,
  type InviteState,
} from "@/lib/invitation"
import { describeWalletError } from "@/lib/wallet-errors"

/** The creator changes or removes the invitation until the escrow is on-chain. */
export function InvitationSection({
  escrow,
  onDone,
}: {
  escrow: EscrowDetail
  onDone: () => void
}) {
  const { request, account } = useAuth()
  const [invite, setInvite] = useState<InviteState | null>(null)
  const [showErrors, setShowErrors] = useState(false)
  const [busy, setBusy] = useState(false)
  const [feedback, setFeedback] = useState<Feedback>(null)

  const otherRole =
    escrow.creator_role === "freelancer" ? "client" : "freelancer"
  const error = invite ? invitationError(account, invite) : null

  function edit() {
    setFeedback(null)
    setShowErrors(false)
    setInvite(inviteStateFrom(escrow.invitation))
  }

  async function save(next: InvitationInput | null, message: string) {
    if (busy) return
    setBusy(true)
    setFeedback(null)
    try {
      await updateInvitation(request, escrow.id, next)
      setInvite(null)
      setFeedback({ kind: "success", message })
      onDone()
    } catch (err) {
      setFeedback({ kind: "error", message: describeWalletError(err) })
    } finally {
      setBusy(false)
    }
  }

  function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!invite) return
    setShowErrors(true)
    if (error) return
    const body = invitationBody(invite) ?? null
    void save(body, body ? "Invitation saved." : "Invitation removed.")
  }

  return (
    <FormSection
      title="Invitation"
      description={`The invited ${otherRole} sees this escrow in their account. Anyone with the link can still join.`}
    >
      {invite ? (
        <form
          onSubmit={handleSubmit}
          noValidate
          className="flex flex-col gap-4"
        >
          <InvitationInputs
            invite={invite}
            onChange={setInvite}
            error={showErrors ? error : null}
          />
          <div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
            <Button
              type="button"
              variant="outline"
              disabled={busy}
              onClick={() => setInvite(null)}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={busy}>
              {busy && <LoaderCircle aria-hidden className="animate-spin" />}
              Save
            </Button>
          </div>
        </form>
      ) : (
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <p className="min-w-0 text-sm break-all">
            {escrow.invitation ? (
              describeInvitation(escrow.invitation)
            ) : (
              <span className="text-muted-foreground">No one is invited.</span>
            )}
          </p>
          <div className="flex shrink-0 gap-2">
            {escrow.invitation && (
              <Button
                variant="ghost"
                size="sm"
                disabled={busy}
                onClick={() => void save(null, "Invitation removed.")}
              >
                {busy && <LoaderCircle aria-hidden className="animate-spin" />}
                Remove
              </Button>
            )}
            <Button variant="outline" size="sm" disabled={busy} onClick={edit}>
              {escrow.invitation ? "Change" : "Invite"}
            </Button>
          </div>
        </div>
      )}
      <FeedbackMessage feedback={feedback} />
    </FormSection>
  )
}
