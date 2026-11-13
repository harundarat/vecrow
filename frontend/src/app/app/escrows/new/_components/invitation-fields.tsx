"use client"

import { FormSection } from "@/components/form-section"
import { InvitationInputs } from "@/components/invitation-inputs"
import type { InviteState } from "@/lib/invitation"

export function InvitationFields({
  invite,
  onChange,
  otherRole,
  error,
}: {
  invite: InviteState
  onChange: (invite: InviteState) => void
  otherRole: string
  error: string | null
}) {
  return (
    <FormSection
      title={`Invite the ${otherRole} (optional)`}
      description="The escrow then shows up in their account. They still join with the link and sign with their own wallet."
    >
      <InvitationInputs invite={invite} onChange={onChange} error={error} />
    </FormSection>
  )
}
