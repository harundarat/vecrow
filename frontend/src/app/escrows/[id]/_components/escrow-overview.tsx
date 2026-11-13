"use client"

import Link from "next/link"
import { useEffect, useState } from "react"
import { ArrowLeft, CircleAlert, LoaderCircle } from "lucide-react"
import { VecrowLogoWordmark } from "@/assets/vecrow-logo-wordmark"
import { CopyButton } from "@/components/copy-button"
import {
  ESCROW_STATUS_HINT,
  EscrowStatusBadge,
} from "@/components/escrow-status-badge"
import { Input } from "@/components/ui/input"
import { useAuth } from "@/context/auth"
import { ApiError } from "@/lib/api/client"
import { describeApiError } from "@/lib/api/errors"
import { getEscrow, type EscrowDetail } from "@/lib/api/escrow"
import { formatPercent } from "@/lib/escrow-rules"
import { ESCROW_ROLE_LABEL, formatDate } from "@/lib/format"
import { formatUsdc } from "@/lib/usdc"
import { cn } from "@/utils"

type Result = { key: string; escrow?: EscrowDetail; error?: string }

/**
 * A short read-only view of an escrow until the full escrow page exists:
 * enough to check what was created and to copy its link.
 */
export function EscrowOverview({ id }: { id: string }) {
  const { status, token } = useAuth()
  const [result, setResult] = useState<Result | null>(null)
  const key = `${id}|${token ?? ""}`

  useEffect(() => {
    // Wait for the session so the creator sees what only they may see.
    if (status === "loading") return
    const controller = new AbortController()
    getEscrow(id, token, controller.signal).then(
      (escrow) => setResult({ key, escrow }),
      (err: unknown) => {
        if (controller.signal.aborted) return
        setResult({
          key,
          error:
            err instanceof ApiError && err.status === 404
              ? "This escrow does not exist. Check the link."
              : describeApiError(err),
        })
      }
    )
    return () => controller.abort()
  }, [id, token, status, key])

  const current = result?.key === key ? result : null
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

      {!current ? (
        <div className="flex justify-center py-16" aria-busy="true">
          <LoaderCircle
            aria-label="Loading the escrow"
            className="size-6 animate-spin text-muted-foreground"
          />
        </div>
      ) : current.error || !current.escrow ? (
        <div className="flex flex-col items-center gap-3 rounded-2xl border border-dashed px-4 py-12 text-center">
          <CircleAlert aria-hidden className="size-6 text-destructive" />
          <p className="font-semibold">Could not open the escrow</p>
          <p className="max-w-sm text-sm text-muted-foreground">
            {current.error}
          </p>
        </div>
      ) : (
        <Overview escrow={current.escrow} />
      )}
    </main>
  )
}

function Overview({ escrow }: { escrow: EscrowDetail }) {
  const creatorRole = ESCROW_ROLE_LABEL[escrow.creator_role]

  return (
    <>
      <div className="flex flex-col gap-2">
        <div className="flex flex-wrap items-center gap-2">
          <h1 className="text-2xl font-medium">
            {escrow.total ? formatUsdc(escrow.total) : "—"} USDC
          </h1>
          <EscrowStatusBadge status={escrow.status} />
        </div>
        <p className="text-sm text-muted-foreground">
          {ESCROW_STATUS_HINT[escrow.status]}. Created by the{" "}
          {creatorRole.toLowerCase()} on {formatDate(escrow.created_at)}.
        </p>
      </div>

      <section className="flex flex-col gap-3 rounded-2xl bg-card p-4 sm:p-6">
        <h2 className="text-lg font-medium">Link</h2>
        <p className="text-sm text-muted-foreground">
          {escrow.viewer.creator
            ? "Send this link to the other party so they can join."
            : "Anyone with this link can see this escrow."}
        </p>
        <div className="flex flex-col gap-2 sm:flex-row">
          <Input
            readOnly
            aria-label="Escrow link"
            value={escrow.link}
            onFocus={(event) => event.currentTarget.select()}
            className="min-w-0 font-mono text-sm sm:flex-1"
          />
          <CopyButton value={escrow.link} label="Copy link" />
        </div>
      </section>

      <section className="flex flex-col gap-3 rounded-2xl bg-card p-4 sm:p-6">
        <h2 className="text-lg font-medium">Milestones</h2>
        <ol className="flex flex-col divide-y rounded-xl border">
          {escrow.milestones.map((milestone) => (
            <li
              key={milestone.index}
              className="flex items-center justify-between gap-3 p-3 text-sm"
            >
              <span>Milestone {milestone.index + 1}</span>
              <span className="flex items-center gap-3">
                <span className="text-muted-foreground">
                  {milestone.status}
                </span>
                <span className="tabular-nums">
                  {formatUsdc(milestone.amount)} USDC
                </span>
              </span>
            </li>
          ))}
        </ol>
        <p className="text-xs text-muted-foreground">
          Platform fee: {formatPercent(escrow.fee_bps)} of each milestone.
        </p>
      </section>

      <section className="flex flex-col gap-3 rounded-2xl bg-card p-4 sm:p-6">
        <h2 className="text-lg font-medium">Parties</h2>
        <dl className="grid gap-3 text-sm sm:grid-cols-2">
          <div className="min-w-0">
            <dt className="text-muted-foreground">Freelancer</dt>
            <dd
              className={cn("truncate", escrow.freelancer && "font-mono")}
              title={escrow.freelancer?.account}
            >
              {escrow.freelancer?.account ?? "Not joined yet"}
            </dd>
            {escrow.freelancer?.split.map((share) => (
              <dd
                key={share.account_id}
                className="truncate text-xs text-muted-foreground"
                title={share.account_id}
              >
                {formatPercent(share.bps)} to {share.account_id}
              </dd>
            ))}
          </div>
          <div className="min-w-0">
            <dt className="text-muted-foreground">Client approver</dt>
            <dd
              className={cn("truncate", escrow.client && "font-mono")}
              title={approverLabel(escrow) ?? undefined}
            >
              {approverLabel(escrow) ?? "Not joined yet"}
            </dd>
            {escrow.client && (
              <dd
                className="truncate text-xs text-muted-foreground"
                title={escrow.client.refund_account}
              >
                Refunds to {escrow.client.refund_account}
              </dd>
            )}
          </div>
        </dl>
      </section>

      <p className="text-sm text-muted-foreground">
        Joining, approving and paying are not available on this page yet.
      </p>
    </>
  )
}

function approverLabel(escrow: EscrowDetail): string | null {
  const approver = escrow.client?.approver
  if (!approver) return null
  return approver.account_id ?? approver.address ?? null
}
