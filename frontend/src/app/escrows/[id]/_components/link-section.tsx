import { CopyButton } from "@/components/copy-button"
import { FormSection } from "@/components/form-section"
import { Input } from "@/components/ui/input"

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
      <div className="flex flex-col gap-2 sm:flex-row">
        <Input
          readOnly
          aria-label="Escrow link"
          value={link}
          onFocus={(event) => event.currentTarget.select()}
          className="min-w-0 font-mono text-sm sm:flex-1"
        />
        <CopyButton value={link} label="Copy link" />
      </div>
    </FormSection>
  )
}
