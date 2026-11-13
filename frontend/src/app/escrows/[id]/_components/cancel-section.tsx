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
import { cancelEscrow } from "@/lib/api/escrow"
import { describeWalletError } from "@/lib/wallet-errors"

/** The creator cancels an escrow that is not on-chain yet. */
export function CancelSection({
  escrowId,
  onDone,
}: {
  escrowId: string
  onDone: () => void
}) {
  const { request } = useAuth()
  const [open, setOpen] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function cancel() {
    if (busy) return
    setBusy(true)
    setError(null)
    try {
      await cancelEscrow(request, escrowId)
      setOpen(false)
      onDone()
    } catch (err) {
      setError(describeWalletError(err))
      setOpen(false)
    } finally {
      setBusy(false)
    }
  }

  return (
    <FormSection
      title="Cancel escrow"
      description="Nothing is on-chain yet, so cancelling is free. Nobody can join after this."
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
              Nobody can join it after this. This cannot be undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
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
