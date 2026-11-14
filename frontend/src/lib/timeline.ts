import type { EscrowDetail } from "./api/escrow"

export type TimelineEntry = {
  kind: "submitted" | "revision" | "released" | "refunded"
  /** ISO time, when known. */
  at: string | null
  txHash: string
  txUrl?: string
  /** How many revisions were asked so far, on a revision entry. */
  revision?: number
  /** Released by the end of the review window, not by the client. */
  auto?: boolean
  /** The client's reason, on a revision entry. */
  reason?: string
}

type EventData = {
  milestone?: unknown
  milestones?: unknown
  submitted_at_ms?: unknown
  revision_count?: unknown
  by?: unknown
}

/** What happened to one milestone, from the transactions the backend sent. */
export function milestoneTimeline(
  escrow: Pick<EscrowDetail, "transactions" | "revision_notes">,
  index: number
): TimelineEntry[] {
  const entries: TimelineEntry[] = []
  for (const tx of escrow.transactions) {
    if (!tx.succeeded) continue
    for (const { event, data } of tx.events) {
      const d = (
        typeof data === "object" && data !== null ? data : {}
      ) as EventData
      const base = { at: tx.created_at ?? null, txHash: tx.hash, txUrl: tx.url }
      if (event === "milestone_submitted" && d.milestone === index) {
        const at =
          typeof d.submitted_at_ms === "number"
            ? new Date(d.submitted_at_ms).toISOString()
            : base.at
        entries.push({ ...base, kind: "submitted", at })
      } else if (event === "revision_requested" && d.milestone === index) {
        const note = escrow.revision_notes.find((n) => n.tx_hash === tx.hash)
        entries.push({
          ...base,
          kind: "revision",
          revision:
            typeof d.revision_count === "number" ? d.revision_count : undefined,
          reason: note?.reason,
        })
      } else if (event === "milestone_released" && d.milestone === index) {
        entries.push({
          ...base,
          kind: "released",
          auto: d.by === "auto_release",
        })
      } else if (
        event === "milestones_refunded" &&
        Array.isArray(d.milestones) &&
        d.milestones.includes(index)
      ) {
        entries.push({ ...base, kind: "refunded" })
      }
    }
  }
  // Notes of revisions sent by someone else have no stored transaction.
  for (const note of escrow.revision_notes) {
    if (note.milestone !== index) continue
    if (entries.some((e) => e.txHash === note.tx_hash)) continue
    entries.push({
      kind: "revision",
      at: note.created_at,
      txHash: note.tx_hash,
      reason: note.reason,
    })
  }
  return entries
}
