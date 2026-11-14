import type { CreatorRole } from "@/lib/api/escrow"
import { formatPercent } from "@/lib/escrow-rules"
import { PLATFORM_FEE_BPS, releasePlan, type Share } from "@/lib/payout"
import { formatUsdc, sumUsdc } from "@/lib/usdc"
import { cn } from "@/utils"
import { FormSection } from "@/components/form-section"

/**
 * The totals, the platform fee and what each recipient gets, worked out per
 * milestone the way the contract pays them out.
 */
export function EscrowSummary({
  role,
  amounts,
  split,
}: {
  role: CreatorRole
  /** Base units per milestone, or null where the amount is not valid yet. */
  amounts: Array<string | null>
  /** The split when it is valid; null for a client or an invalid split. */
  split: Share[] | null
}) {
  const ready = amounts.length > 0 && amounts.every((a) => a !== null)
  const valid = ready ? (amounts as string[]) : []
  const plans = valid.map((amount) =>
    releasePlan(amount, PLATFORM_FEE_BPS, split ?? [])
  )
  const total = sumUsdc(valid)
  const fee = sumUsdc(plans.map((plan) => plan.fee))
  const net = (BigInt(total) - BigInt(fee)).toString()
  const feeLabel = `Platform fee (${formatPercent(PLATFORM_FEE_BPS)})`

  return (
    <FormSection
      title="Summary"
      description="Check the numbers before you create the escrow."
    >
      {!ready ? (
        <p className="text-sm text-muted-foreground">
          Enter the milestone amounts to see the totals.
        </p>
      ) : (
        <>
          <dl className="flex flex-col gap-2 text-sm">
            <Row label="The client pays" value={total} strong />
            <Row label={feeLabel} value={fee} />
            {split ? (
              split.map((share, j) => (
                <Row
                  key={share.account_id}
                  label={`${share.account_id} (${formatPercent(share.bps)})`}
                  value={sumUsdc(plans.map((plan) => plan.shares[j]))}
                />
              ))
            ) : (
              <Row label="The freelancer gets" value={net} />
            )}
          </dl>

          <div className="flex flex-col gap-2">
            <h3 className="text-sm font-medium">Per milestone</h3>
            <ol className="flex flex-col gap-2">
              {valid.map((amount, i) => (
                <li key={i} className="rounded-xl border p-3">
                  <dl className="flex flex-col gap-1 text-sm">
                    <Row label={`Milestone ${i + 1}`} value={amount} strong />
                    <Row label={feeLabel} value={plans[i].fee} muted />
                    {split ? (
                      split.map((share, j) => (
                        <Row
                          key={share.account_id}
                          label={share.account_id}
                          value={plans[i].shares[j]}
                          muted
                        />
                      ))
                    ) : (
                      <Row
                        label="The freelancer gets"
                        value={(
                          BigInt(amount) - BigInt(plans[i].fee)
                        ).toString()}
                        muted
                      />
                    )}
                  </dl>
                </li>
              ))}
            </ol>
          </div>

          <p className="text-xs text-muted-foreground">
            The fee is taken from each milestone when it is paid out.
            {split
              ? " Amounts are rounded down; any leftover goes to the first recipient."
              : role === "client"
                ? " The freelancer chooses how their part is split."
                : " Fix the split to see each share."}
          </p>
        </>
      )}
    </FormSection>
  )
}

function Row({
  label,
  value,
  strong,
  muted,
}: {
  label: string
  /** Base units. */
  value: string
  strong?: boolean
  muted?: boolean
}) {
  return (
    <div
      className={cn(
        "flex items-baseline justify-between gap-3",
        muted && "text-muted-foreground",
        strong && "font-medium"
      )}
    >
      <dt className="min-w-0 truncate" title={label}>
        {label}
      </dt>
      <dd className="shrink-0 tabular-nums">{formatUsdc(value)} USDC</dd>
    </div>
  )
}
