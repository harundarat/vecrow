"use client"

import Link from "next/link"
import { useState } from "react"
import { CircleCheck, LoaderCircle } from "lucide-react"
import { Address } from "@/components/address"
import { FeedbackMessage, type Feedback } from "@/components/feedback-message"
import { FormSection } from "@/components/form-section"
import { Button } from "@/components/ui/button"
import { useAuth } from "@/context/auth"
import {
  connectWalletAs,
  SCHEME_CHAIN,
  useWalletSigners,
} from "@/hooks/use-wallet-signer"
import { linkWallet } from "@/lib/api/account"
import { proveWallet, type ChallengeSigner } from "@/lib/api/auth"
import type { EscrowDetail, EscrowStatus } from "@/lib/api/escrow"
import { WALLET_SCHEME_LABEL } from "@/lib/format"
import { sameWallet, type GuestAccess } from "@/lib/guest-access"
import { describeWalletError } from "@/lib/wallet-errors"

const STATUS_TEXT: Partial<Record<EscrowStatus, string>> = {
  PendingApproval: "Your details are sent. Waiting for the creator to approve.",
  Approved: "The creator approved. The escrow is going on-chain.",
  Created: "The escrow is on-chain.",
  Funded: "The escrow is on-chain and paid in.",
  Completed: "All milestones are closed.",
  Cancelled: "This escrow was cancelled.",
}

type Busy = "save" | "renew" | null

/** For the visitor whose wallet submitted to this escrow. */
export function JoinedSection({
  escrow,
  access,
  renew,
}: {
  escrow: EscrowDetail
  access: GuestAccess
  renew: (signer: ChallengeSigner) => Promise<void>
}) {
  const { status, account, recovery, request, signIn, setAccount } = useAuth()
  const signers = useWalletSigners()
  const [busy, setBusy] = useState<Busy>(null)
  const [feedback, setFeedback] = useState<Feedback>(null)

  const { wallet } = access
  const signedIn = status === "authenticated"
  const saved =
    signedIn &&
    account?.wallets.some((w) =>
      sameWallet({ scheme: w.scheme, address: w.address }, wallet)
    )

  async function withWallet(kind: Exclude<Busy, null>) {
    const signer = signers[SCHEME_CHAIN[wallet.scheme]]
    const address = await connectWalletAs(signer, wallet.address)
    const challenge: ChallengeSigner = {
      scheme: wallet.scheme,
      address,
      sign: signer.sign,
    }
    if (kind === "renew") return renew(challenge)
    if (signedIn) {
      setAccount(await linkWallet(request, await proveWallet(challenge)))
    } else {
      await signIn(challenge)
    }
  }

  async function run(kind: Exclude<Busy, null>) {
    if (busy) return
    setBusy(kind)
    setFeedback(null)
    try {
      await withWallet(kind)
      setFeedback(
        kind === "renew"
          ? { kind: "success", message: "Guest access renewed." }
          : null
      )
    } catch (err) {
      setFeedback({ kind: "error", message: describeWalletError(err) })
    } finally {
      setBusy(null)
    }
  }

  return (
    <FormSection
      title="You joined"
      description={STATUS_TEXT[escrow.status] ?? ""}
    >
      <p className="text-sm">
        <span className="text-muted-foreground">Signed with </span>
        {WALLET_SCHEME_LABEL[wallet.scheme]} <Address value={wallet.address} />
      </p>

      {saved ? (
        <p className="flex items-center gap-2 text-sm">
          <CircleCheck aria-hidden className="size-4 shrink-0 text-primary" />
          <span>
            Saved to your account.{" "}
            <Link
              href="/app"
              className="font-medium underline underline-offset-4"
            >
              Open my escrows
            </Link>
          </span>
        </p>
      ) : (
        <div className="flex flex-col gap-3 rounded-xl border p-3 sm:flex-row sm:items-center sm:justify-between">
          <p className="text-sm text-muted-foreground">
            {signedIn
              ? "Link this wallet to your account to find this escrow there."
              : "Sign in with the same wallet to find this escrow in your account."}
          </p>
          <Button
            variant="outline"
            disabled={busy !== null}
            onClick={() => void run("save")}
            className="shrink-0"
          >
            {busy === "save" && (
              <LoaderCircle aria-hidden className="animate-spin" />
            )}
            Save to account
          </Button>
        </div>
      )}

      {!access.token && !recovery && (
        <div className="flex flex-col gap-3 rounded-xl border p-3 sm:flex-row sm:items-center sm:justify-between">
          <p className="text-sm text-muted-foreground">
            Your guest access expired. Sign with the same wallet to renew it.
          </p>
          <Button
            variant="outline"
            disabled={busy !== null}
            onClick={() => void run("renew")}
            className="shrink-0"
          >
            {busy === "renew" && (
              <LoaderCircle aria-hidden className="animate-spin" />
            )}
            Renew access
          </Button>
        </div>
      )}

      <FeedbackMessage feedback={feedback} />
    </FormSection>
  )
}
