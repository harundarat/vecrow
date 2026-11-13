"use client"

import Link from "next/link"
import { ArrowLeft, CircleAlert, LoaderCircle } from "lucide-react"
import { VecrowLogoWordmark } from "@/assets/vecrow-logo-wordmark"
import { FeedbackMessage } from "@/components/feedback-message"
import { useAuth } from "@/context/auth"
import { useEscrowDetail } from "@/hooks/use-escrow-detail"
import type { useGuestAccess } from "@/hooks/use-guest-access"
import type { EscrowDetail } from "@/lib/api/escrow"
import { sameWallet } from "@/lib/guest-access"
import {
  EscrowHeader,
  EscrowWarnings,
  MilestonesCard,
  PartiesCard,
} from "./escrow-details"
import { JoinSection } from "./join-section"
import { JoinedSection } from "./joined-section"
import { LinkSection } from "./link-section"
import { OnChainDone, OnChainProgress } from "./on-chain-section"

const ON_CHAIN = new Set(["Created", "Funded", "Completed"])

export function EscrowPage({ id }: { id: string }) {
  const { status } = useAuth()
  const detail = useEscrowDetail(id)
  const signedIn = status === "authenticated"

  return (
    <main className="mx-auto flex min-h-svh w-full max-w-3xl flex-col gap-6 px-4 py-6">
      <Link
        href={signedIn ? "/app" : "/"}
        aria-label="Vecrow"
        className="self-start"
      >
        <VecrowLogoWordmark className="w-28" />
      </Link>
      {signedIn && (
        <Link
          href="/app"
          className="-mb-2 inline-flex items-center gap-1 self-start text-sm text-muted-foreground hover:text-foreground"
        >
          <ArrowLeft aria-hidden className="size-4" />
          Escrows
        </Link>
      )}

      {detail.escrow ? (
        <EscrowView
          escrow={detail.escrow}
          reopened={detail.reopened}
          guest={detail.guest}
        />
      ) : detail.error ? (
        <div className="flex flex-col items-center gap-3 rounded-2xl border border-dashed px-4 py-12 text-center">
          <CircleAlert aria-hidden className="size-6 text-destructive" />
          <p className="font-semibold">Could not open the escrow</p>
          <p className="max-w-sm text-sm text-muted-foreground">
            {detail.error}
          </p>
        </div>
      ) : (
        <div className="flex justify-center py-16" aria-busy="true">
          <LoaderCircle
            aria-label="Loading the escrow"
            className="size-6 animate-spin text-muted-foreground"
          />
        </div>
      )}
    </main>
  )
}

function EscrowView({
  escrow,
  reopened,
  guest,
}: {
  escrow: EscrowDetail
  reopened: boolean
  guest: ReturnType<typeof useGuestAccess>
}) {
  const { recovery } = useAuth()
  const creator = escrow.viewer.creator
  const status = escrow.status
  const access = guest.access
  const joined =
    !creator &&
    access !== null &&
    escrow.submission !== null &&
    sameWallet(access.wallet, escrow.submission.wallet)

  return (
    <>
      <EscrowHeader escrow={escrow} />
      {recovery && (
        <p className="rounded-2xl bg-primary/10 p-4 text-sm">
          You signed in with an email code, so you can only view this escrow.{" "}
          <Link
            href={`/sign-in?next=${encodeURIComponent(`/escrows/${escrow.id}`)}`}
            className="font-medium underline underline-offset-4"
          >
            Sign in with a wallet
          </Link>
        </p>
      )}
      {reopened && status === "Open" && (
        <FeedbackMessage
          feedback={{
            kind: "error",
            message:
              "The contract did not accept the terms, so the escrow is open again. The other party can join again.",
          }}
        />
      )}
      <EscrowWarnings warnings={escrow.warnings} />

      {joined && (
        <JoinedSection escrow={escrow} access={access} renew={guest.renew} />
      )}
      {status === "Approved" && <OnChainProgress />}
      {ON_CHAIN.has(status) && <OnChainDone escrow={escrow} />}
      {creator && status === "Open" && <LinkSection link={escrow.link} />}

      <MilestonesCard escrow={escrow} />
      <PartiesCard escrow={escrow} />

      {!creator && !recovery && status === "Open" && (
        <JoinSection
          escrow={escrow}
          rejoin={access !== null}
          onJoined={guest.save}
        />
      )}

      {creator && status !== "Open" && <LinkSection link={escrow.link} />}
    </>
  )
}
