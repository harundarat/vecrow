import { CircleCheck, ExternalLink, LoaderCircle } from "lucide-react"
import { FormSection } from "@/components/form-section"
import type { EscrowDetail } from "@/lib/api/escrow"

/** Both parties signed; the backend is creating the escrow on NEAR. */
export function OnChainProgress() {
  return (
    <FormSection
      title="Going on-chain"
      description="This takes about a minute. The page updates by itself."
    >
      <ol className="flex flex-col gap-3 text-sm" aria-live="polite">
        <li className="flex items-center gap-2">
          <CircleCheck aria-hidden className="size-4 shrink-0 text-primary" />
          Both parties signed
        </li>
        <li className="flex items-center gap-2">
          <LoaderCircle
            aria-hidden
            className="size-4 shrink-0 animate-spin text-muted-foreground"
          />
          Creating the escrow on NEAR…
        </li>
      </ol>
    </FormSection>
  )
}

/** The escrow is on NEAR: links the transaction that created it. */
export function OnChainDone({ escrow }: { escrow: EscrowDetail }) {
  const created = escrow.transactions.find(
    (tx) => tx.method === "create_escrow" && tx.succeeded
  )
  return (
    <FormSection
      title="On-chain"
      description={
        escrow.status === "Created"
          ? "The escrow is on NEAR and waits for the client to pay in."
          : "The escrow is on NEAR."
      }
    >
      {created?.url && (
        <a
          href={created.url}
          target="_blank"
          rel="noreferrer"
          className="inline-flex items-center gap-1 self-start text-sm font-medium underline underline-offset-4"
        >
          View the transaction
          <ExternalLink aria-hidden className="size-3.5" />
        </a>
      )}
    </FormSection>
  )
}
