import {
  ESCROW_STATUS_HINT,
  EscrowStatusBadge,
  InvitedBadge,
  MilestoneStatusBadge,
} from "@/components/escrow-status-badge"
import { Address } from "@/components/address"
import { Notice } from "@/components/feedback-message"
import { FormSection } from "@/components/form-section"
import type { Approver, EscrowDetail, Milestone } from "@/lib/api/escrow"
import { formatPercent } from "@/lib/escrow-rules"
import {
  ESCROW_ROLE_LABEL,
  formatDate,
  WALLET_SCHEME_LABEL,
} from "@/lib/format"
import { mulBps, releasePlan } from "@/lib/payout"
import { formatUsdc, sumUsdc } from "@/lib/usdc"
import { MilestoneTimeline } from "./milestone-timeline"

/** "NEAR · alice.near" or "EVM · 0x12…", with a copy button. */
export function ApproverWallet({ approver }: { approver: Approver }) {
  return (
    <span className="inline-flex max-w-full min-w-0 items-center gap-1 align-middle">
      <span className="shrink-0">
        {WALLET_SCHEME_LABEL[approver.scheme] ?? approver.scheme} ·
      </span>
      <Address value={approver.account_id ?? approver.address ?? ""} />
    </span>
  )
}

export function EscrowHeader({ escrow }: { escrow: EscrowDetail }) {
  const creatorRole = ESCROW_ROLE_LABEL[escrow.creator_role].toLowerCase()
  return (
    <div className="flex flex-col gap-2">
      <div className="flex flex-wrap items-center gap-2">
        <h1 className="text-2xl font-medium">
          {escrow.total ? `${formatUsdc(escrow.total)} USDC` : "No amount yet"}
        </h1>
        <EscrowStatusBadge status={escrow.status} />
        {escrow.viewer.invited && <InvitedBadge />}
      </div>
      <p className="text-sm text-muted-foreground">
        {ESCROW_STATUS_HINT[escrow.status]}. Created by the {creatorRole} on{" "}
        {formatDate(escrow.created_at)}.
      </p>
    </div>
  )
}

/** Warnings the backend gives the creator and the invited account. */
export function EscrowWarnings({
  warnings,
}: {
  warnings: EscrowDetail["warnings"]
}) {
  if (warnings.length === 0) return null
  return (
    <div className="flex flex-col gap-2">
      {warnings.map((warning) => (
        <Notice key={warning.code} role="alert">
          {warning.message}
        </Notice>
      ))}
    </div>
  )
}

export function MilestonesCard({
  escrow,
  actions,
}: {
  escrow: EscrowDetail
  /** Extra controls under a milestone, e.g. the approver's buttons. */
  actions?: (milestone: Milestone) => React.ReactNode
}) {
  const fee = sumUsdc(
    escrow.milestones.map((m) =>
      mulBps(BigInt(m.amount), escrow.fee_bps).toString()
    )
  )
  return (
    <FormSection title="Milestones">
      <ol className="flex flex-col divide-y rounded-xl border">
        {escrow.milestones.map((milestone) => (
          <li key={milestone.index} className="flex flex-col gap-3 p-3 text-sm">
            <div className="flex items-start justify-between gap-3">
              <div className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1">
                <span className="font-medium whitespace-nowrap">
                  Milestone {milestone.index + 1}
                </span>
                <MilestoneStatusBadge status={milestone.status} />
              </div>
              <span className="shrink-0 whitespace-nowrap tabular-nums">
                {formatUsdc(milestone.amount)} USDC
              </span>
            </div>
            <PayoutBreakdown escrow={escrow} amount={milestone.amount} />
            <MilestoneTimeline escrow={escrow} milestone={milestone} />
            {actions?.(milestone)}
          </li>
        ))}
      </ol>
      <p className="text-xs text-muted-foreground">
        Platform fee: {formatPercent(escrow.fee_bps)} of each payment,{" "}
        {formatUsdc(fee)} USDC in total.
      </p>
    </FormSection>
  )
}

/** Who gets what when the milestone is released, exactly as the contract pays. */
function PayoutBreakdown({
  escrow,
  amount,
}: {
  escrow: EscrowDetail
  amount: string
}) {
  const split = escrow.freelancer?.split ?? []
  const plan = releasePlan(amount, escrow.fee_bps, split)
  const rows =
    split.length > 0
      ? split.map((share, i) => ({
          label: share.account_id,
          amount: plan.shares[i],
          mono: true,
        }))
      : [
          {
            label: "Freelancer",
            amount: (BigInt(amount) - BigInt(plan.fee)).toString(),
            mono: false,
          },
        ]
  return (
    <ul
      aria-label="Payout on release"
      className="flex flex-col gap-1 text-xs text-muted-foreground"
    >
      {rows.map((row) => (
        <li key={row.label} className="flex justify-between gap-3">
          <span className="min-w-0 truncate">
            {row.mono ? <Address value={row.label} /> : row.label}
          </span>
          <span className="shrink-0 tabular-nums">
            {formatUsdc(row.amount)} USDC
          </span>
        </li>
      ))}
      <li className="flex justify-between gap-3">
        <span>Platform fee</span>
        <span className="shrink-0 tabular-nums">
          {formatUsdc(plan.fee)} USDC
        </span>
      </li>
    </ul>
  )
}

export function PartiesCard({ escrow }: { escrow: EscrowDetail }) {
  const approver = escrow.client?.approver
  return (
    <FormSection title="Parties">
      <dl className="grid gap-4 text-sm sm:grid-cols-2">
        <div className="flex min-w-0 flex-col gap-0.5">
          <dt className="text-muted-foreground">Freelancer</dt>
          <dd className="min-w-0">
            {escrow.freelancer ? (
              <Address value={escrow.freelancer.account} />
            ) : (
              "Not joined yet"
            )}
          </dd>
          {escrow.freelancer?.split.map((share) => (
            <dd
              key={share.account_id}
              className="flex min-w-0 items-center gap-1 text-xs text-muted-foreground"
            >
              <span className="shrink-0">{formatPercent(share.bps)} to</span>
              <Address value={share.account_id} />
            </dd>
          ))}
        </div>
        <div className="flex min-w-0 flex-col gap-0.5">
          <dt className="text-muted-foreground">Client approver</dt>
          <dd className="min-w-0">
            {approver ? (
              <ApproverWallet approver={approver} />
            ) : (
              "Not joined yet"
            )}
          </dd>
          {escrow.client && (
            <dd className="flex min-w-0 items-center gap-1 text-xs text-muted-foreground">
              <span className="shrink-0">Refunds to</span>
              <Address value={escrow.client.refund_account} />
            </dd>
          )}
        </div>
      </dl>
    </FormSection>
  )
}
