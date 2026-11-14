"use client"

import { useState } from "react"
import { Download, LoaderCircle } from "lucide-react"
import { FeedbackMessage } from "@/components/feedback-message"
import { FormSection } from "@/components/form-section"
import { Button } from "@/components/ui/button"
import { describeApiError } from "@/lib/api/errors"
import { downloadPayoutsCsv, type EscrowDetail } from "@/lib/api/escrow"
import { formatUsdc } from "@/lib/usdc"

export type ExportAuth = { token: string | null; guestToken: string | null }

/** Payouts that are still owed, and the CSV of every payout for the parties. */
export function PayoutsSection({
  escrow,
  auth,
}: {
  escrow: EscrowDetail
  /** How the viewer proves they are a party, or null for other visitors. */
  auth: ExportAuth | null
}) {
  const pending = escrow.pending_payouts
  if (!auth && pending.length === 0) return null
  return (
    <FormSection
      title="Payouts"
      description={
        auth
          ? "Download every payout as CSV. The platform fee is its own row."
          : undefined
      }
    >
      {pending.length > 0 && (
        <div className="flex flex-col gap-2 rounded-xl border p-3 text-sm">
          <p className="font-medium">Not paid yet</p>
          <ul className="flex flex-col gap-1">
            {pending.map((payout) => (
              <li
                key={payout.account_id}
                className="flex justify-between gap-3"
              >
                <span
                  className="min-w-0 truncate font-mono"
                  title={payout.account_id}
                >
                  {payout.account_id}
                </span>
                <span className="shrink-0 tabular-nums">
                  {formatUsdc(payout.amount)} USDC
                </span>
              </li>
            ))}
          </ul>
          <p className="text-xs text-muted-foreground">
            These accounts could not receive USDC yet. Vecrow retries the
            payment automatically.
          </p>
        </div>
      )}
      {auth && <DownloadCsv escrow={escrow} auth={auth} />}
    </FormSection>
  )
}

function DownloadCsv({
  escrow,
  auth,
}: {
  escrow: EscrowDetail
  auth: ExportAuth
}) {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function download() {
    if (busy) return
    setBusy(true)
    setError(null)
    try {
      // A plain link cannot send the session or guest token.
      const { blob, filename } = await downloadPayoutsCsv(escrow.id, auth)
      const url = URL.createObjectURL(blob)
      const link = document.createElement("a")
      link.href = url
      link.download = filename ?? `vecrow-escrow-${escrow.id}.csv`
      document.body.append(link)
      link.click()
      link.remove()
      setTimeout(() => URL.revokeObjectURL(url), 1000)
    } catch (err) {
      setError(describeApiError(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <>
      <FeedbackMessage
        feedback={error ? { kind: "error", message: error } : null}
      />
      <Button
        variant="outline"
        className="self-start"
        disabled={busy}
        onClick={() => void download()}
      >
        {busy ? (
          <LoaderCircle aria-hidden className="animate-spin" />
        ) : (
          <Download aria-hidden />
        )}
        Download CSV
      </Button>
    </>
  )
}
