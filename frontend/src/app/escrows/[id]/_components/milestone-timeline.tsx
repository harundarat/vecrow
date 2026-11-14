"use client"

import { ExternalLink } from "lucide-react"
import { useNow } from "@/hooks/use-now"
import type { EscrowDetail, Milestone } from "@/lib/api/escrow"
import { MAX_REVISIONS } from "@/lib/escrow-rules"
import { formatDate, formatDuration, formatTime } from "@/lib/format"
import { milestoneTimeline, type TimelineEntry } from "@/lib/timeline"

/** What happened to a milestone so far, and when it pays out by itself. */
export function MilestoneTimeline({
  escrow,
  milestone,
}: {
  escrow: EscrowDetail
  milestone: Milestone
}) {
  const entries = milestoneTimeline(escrow, milestone.index)
  const autoRelease =
    milestone.status === "Submitted" ? milestone.auto_release_at_ms : null
  if (entries.length === 0 && !autoRelease && milestone.revision_count === 0) {
    return null
  }
  return (
    <div className="flex flex-col gap-2 text-xs">
      {entries.length > 0 && (
        <ol aria-label="History" className="flex flex-col gap-2">
          {entries.map((entry, i) => (
            <li
              key={`${entry.txHash}-${entry.kind}`}
              className="flex flex-col gap-1"
            >
              <div className="flex flex-wrap items-center gap-x-2 gap-y-0.5">
                <span className="font-medium">
                  {entryLabel(entry, entries.slice(0, i))}
                </span>
                {entry.at && (
                  <span className="text-muted-foreground">
                    {formatDate(entry.at)}, {formatTime(entry.at)}
                  </span>
                )}
                {entry.txUrl && (
                  <a
                    href={entry.txUrl}
                    target="_blank"
                    rel="noreferrer"
                    className="inline-flex items-center gap-0.5 underline underline-offset-4"
                  >
                    Transaction
                    <ExternalLink aria-hidden className="size-3" />
                  </a>
                )}
              </div>
              {entry.reason && (
                <p className="rounded-lg bg-muted p-2 break-words whitespace-pre-line">
                  {entry.reason}
                </p>
              )}
            </li>
          ))}
        </ol>
      )}
      {milestone.revision_count > 0 && (
        <p className="text-muted-foreground">
          Revisions used: {milestone.revision_count} of {MAX_REVISIONS}
        </p>
      )}
      {autoRelease && <AutoReleaseCountdown at={autoRelease} />}
    </div>
  )
}

function entryLabel(entry: TimelineEntry, before: TimelineEntry[]): string {
  switch (entry.kind) {
    case "submitted":
      return before.some((e) => e.kind === "revision")
        ? "Resubmitted"
        : "Submitted"
    case "revision":
      return entry.revision
        ? `Revision ${entry.revision} of ${MAX_REVISIONS} requested`
        : "Revision requested"
    case "released":
      return entry.auto ? "Paid out automatically" : "Approved and paid out"
    case "refunded":
      return "Refunded to the client"
  }
}

function AutoReleaseCountdown({ at }: { at: number }) {
  const now = useNow(30_000)
  const left = at - now
  const iso = new Date(at).toISOString()
  return (
    <p aria-live="polite" className="text-muted-foreground">
      {left > 0
        ? `Paid out automatically in ${formatDuration(left)}, on ${formatDate(iso)} at ${formatTime(iso)}.`
        : "The review time is over. It is paid out automatically soon."}
    </p>
  )
}
