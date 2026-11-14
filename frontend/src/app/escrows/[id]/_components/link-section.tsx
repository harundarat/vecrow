import { CopyField } from "@/components/copy-button"
import { FormSection } from "@/components/form-section"

export function LinkSection({ link, open }: { link: string; open: boolean }) {
  return (
    <FormSection
      title="Link"
      description={
        open
          ? "Send this link to the other party so they can join."
          : "Both parties can follow this escrow at this link."
      }
    >
      <CopyField value={link} label="Escrow link" />
    </FormSection>
  )
}
