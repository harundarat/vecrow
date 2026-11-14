"use client"

import { useEffect, useMemo, useState } from "react"
import {
  CircleAlert,
  CircleCheck,
  ExternalLink,
  LoaderCircle,
} from "lucide-react"
import { CopyButton } from "@/components/copy-button"
import { LoadingState } from "@/components/loading-state"
import { FeedbackMessage } from "@/components/feedback-message"
import { FormSection } from "@/components/form-section"
import { Button } from "@/components/ui/button"
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { useFunding } from "@/hooks/use-funding"
import { describeApiError } from "@/lib/api/errors"
import type { Approver, EscrowDetail } from "@/lib/api/escrow"
import {
  listTokens,
  requestQuote,
  type Funding,
  type FundingAuth,
  type FundingQuote,
  type Token,
} from "@/lib/api/funding"
import { formatDate, formatTime, shortenAddress } from "@/lib/format"
import {
  canRequestNewQuote,
  chainName,
  chainTokens,
  defaultChain,
  defaultToken,
  depositExplorerUrl,
  fundingStatusText,
  refundAddressFor,
  tokenChains,
} from "@/lib/funding"
import { formatUsdc } from "@/lib/usdc"
import { ApproverWallet } from "./escrow-details"

type TokensState = { tokens: Token[] | null; error: string | null }

/** The approver pays in from any chain through 1Click. */
export function FundingSection({
  escrow,
  auth,
  onFunded,
}: {
  escrow: EscrowDetail
  auth: FundingAuth
  onFunded: () => void
}) {
  const { funding, loaded, error, show } = useFunding(escrow.id, auth)
  const [tokens, setTokens] = useState<TokensState>({
    tokens: null,
    error: null,
  })
  const [replacing, setReplacing] = useState(false)
  const approver = escrow.client?.approver
  const funded = escrow.status !== "Created"

  useEffect(() => {
    const controller = new AbortController()
    listTokens(controller.signal).then(
      (list) => setTokens({ tokens: list, error: null }),
      (err: unknown) => {
        if (controller.signal.aborted) return
        setTokens({ tokens: null, error: describeApiError(err) })
      }
    )
    return () => controller.abort()
  }, [])

  // A quote that nothing was sent to can be replaced once it expires.
  const [nowMs, setNowMs] = useState(() => Date.now())
  useEffect(() => {
    const timer = setInterval(() => setNowMs(Date.now()), 15000)
    return () => clearInterval(timer)
  }, [])

  const fundingStatus = funding?.status
  useEffect(() => {
    if (fundingStatus === "SUCCESS" && !funded) onFunded()
  }, [fundingStatus, funded, onFunded])

  if (!approver) return null
  if (funded) {
    if (!funding) return null
    return (
      <FormSection title="Payment">
        <DepositDetails funding={funding} tokens={tokens.tokens} paid />
      </FormSection>
    )
  }

  const showForm =
    loaded &&
    (!funding || replacing || canRequestNewQuote(funding, nowMs)) &&
    funding?.status !== "SUCCESS"

  return (
    <FormSection
      title="Pay in"
      description={`Pay ${formatUsdc(escrow.total)} USDC from any chain and token. 1Click swaps it and sends it to the escrow.`}
    >
      <p className="flex items-start gap-2 rounded-xl bg-muted p-3 text-sm">
        <CircleAlert aria-hidden className="mt-0.5 size-4 shrink-0" />
        <span className="min-w-0 break-words">
          Pay only if the approver <ApproverWallet approver={approver} /> is
          your wallet. It approves every payout.
        </span>
      </p>

      {!loaded && !error ? (
        <LoadingState label="Loading the payment" className="py-2" />
      ) : null}
      <FeedbackMessage
        feedback={error ? { kind: "error", message: error } : null}
      />

      {funding && !replacing && (
        <DepositDetails
          funding={funding}
          tokens={tokens.tokens}
          onReplace={
            sentNothing(funding) && !canRequestNewQuote(funding, nowMs)
              ? () => setReplacing(true)
              : undefined
          }
        />
      )}

      {showForm &&
        (tokens.tokens ? (
          <QuoteForm
            escrowId={escrow.id}
            approver={approver}
            tokens={tokens.tokens}
            auth={auth}
            submitLabel={funding ? "Get a new deposit address" : undefined}
            onCancel={replacing ? () => setReplacing(false) : undefined}
            onQuote={(quote) => {
              setReplacing(false)
              show(quote)
            }}
          />
        ) : (
          <FeedbackMessage
            feedback={
              tokens.error ? { kind: "error", message: tokens.error } : null
            }
          />
        ))}
    </FormSection>
  )
}

function sentNothing(funding: Funding): boolean {
  const raw = funding.oneclick_status ?? funding.quote.oneclick_status
  return (
    funding.status === "PENDING" &&
    (raw === undefined || raw === "PENDING_DEPOSIT")
  )
}

function tokenOf(tokens: Token[] | null, assetId: string) {
  return tokens?.find((t) => t.asset_id === assetId)
}

function DepositDetails({
  funding,
  tokens,
  paid = false,
  onReplace,
}: {
  funding: Funding
  tokens: Token[] | null
  paid?: boolean
  onReplace?: () => void
}) {
  const { quote, status } = funding
  const token = tokenOf(tokens, quote.origin_asset)
  const symbol = token?.symbol ?? quote.origin_asset
  const chain = token ? chainName(token.blockchain) : null
  const deadline = quote.deadline

  return (
    <div className="flex flex-col gap-4">
      {paid ? (
        <p className="text-sm">
          Paid with {quote.amount_in_formatted} {symbol}
          {chain && ` on ${chain}`}.
        </p>
      ) : (
        <dl className="flex flex-col gap-3 text-sm">
          <DepositRow
            label={chain ? `Send exactly, on ${chain}` : "Send exactly"}
            copy={quote.amount_in_formatted}
            copyLabel="Copy amount"
          >
            <span className="font-medium tabular-nums">
              {quote.amount_in_formatted} {symbol}
            </span>
          </DepositRow>
          <DepositRow
            label="To this address"
            copy={quote.deposit_address}
            copyLabel="Copy address"
          >
            <span className="font-mono break-all">{quote.deposit_address}</span>
          </DepositRow>
          {quote.deposit_memo && (
            <DepositRow
              label="With this memo, or the payment is lost"
              copy={quote.deposit_memo}
              copyLabel="Copy memo"
            >
              <span className="font-mono break-all">{quote.deposit_memo}</span>
            </DepositRow>
          )}
          {deadline && (
            <div className="flex flex-col gap-0.5">
              <dt className="text-muted-foreground">Send before</dt>
              <dd>
                {formatDate(deadline)}, {formatTime(deadline)}
              </dd>
            </div>
          )}
        </dl>
      )}

      {!paid && (
        <p
          aria-live="polite"
          className="flex items-center gap-2 text-sm font-medium"
        >
          {status === "PENDING" ? (
            <LoaderCircle
              aria-hidden
              className="size-4 shrink-0 animate-spin text-muted-foreground"
            />
          ) : status === "SUCCESS" ? (
            <CircleCheck aria-hidden className="size-4 shrink-0 text-primary" />
          ) : (
            <CircleAlert
              aria-hidden
              className="size-4 shrink-0 text-destructive"
            />
          )}
          <span className="min-w-0 break-words">
            {fundingStatusText(funding)}
          </span>
        </p>
      )}

      <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
        <a
          href={depositExplorerUrl(quote.deposit_address)}
          target="_blank"
          rel="noreferrer"
          className="inline-flex items-center gap-1 self-start text-sm font-medium underline underline-offset-4"
        >
          {paid ? "View the payment" : "Track the payment"}
          <ExternalLink aria-hidden className="size-3.5" />
        </a>
        {onReplace && (
          <Button variant="ghost" size="sm" onClick={onReplace}>
            Pay with another token
          </Button>
        )}
      </div>
    </div>
  )
}

function DepositRow({
  label,
  copy,
  copyLabel,
  children,
}: React.PropsWithChildren<{
  label: string
  copy: string
  copyLabel: string
}>) {
  return (
    <div className="flex flex-col gap-1">
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
        <span className="min-w-0">{children}</span>
        <span className="shrink-0 self-start sm:self-auto">
          <CopyButton value={copy} label={copyLabel} />
        </span>
      </dd>
    </div>
  )
}

function tokenItems(tokens: Token[]) {
  const counts = new Map<string, number>()
  tokens.forEach((t) => counts.set(t.symbol, (counts.get(t.symbol) ?? 0) + 1))
  return tokens.map((t) => ({
    value: t.asset_id,
    label:
      (counts.get(t.symbol) ?? 0) > 1 && t.contract_address
        ? `${t.symbol} (${shortenAddress(t.contract_address)})`
        : t.symbol,
  }))
}

function QuoteForm({
  escrowId,
  approver,
  tokens,
  auth,
  submitLabel = "Get deposit address",
  onCancel,
  onQuote,
}: {
  escrowId: string
  approver: Approver
  tokens: Token[]
  auth: FundingAuth
  submitLabel?: string
  onCancel?: () => void
  onQuote: (quote: FundingQuote) => void
}) {
  const chains = useMemo(() => tokenChains(tokens), [tokens])
  const [chain, setChain] = useState(() =>
    defaultChain(chains, approver.scheme)
  )
  const onChain = useMemo(() => chainTokens(tokens, chain), [tokens, chain])
  const [assetId, setAssetId] = useState(
    () => defaultToken(chainTokens(tokens, chain))?.asset_id ?? ""
  )
  const [refundTo, setRefundTo] = useState(
    () => refundAddressFor(approver, chain) ?? ""
  )
  const [refundEdited, setRefundEdited] = useState(false)
  const [showErrors, setShowErrors] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const chainItems = chains.map((c) => ({ value: c, label: chainName(c) }))
  const items = tokenItems(onChain)
  const refundError = refundTo.trim()
    ? null
    : `Enter an address on ${chainName(chain)}.`

  function changeChain(next: string) {
    setChain(next)
    setAssetId(defaultToken(chainTokens(tokens, next))?.asset_id ?? "")
    if (!refundEdited) setRefundTo(refundAddressFor(approver, next) ?? "")
  }

  async function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (busy) return
    setShowErrors(true)
    setError(null)
    if (refundError || !assetId) return
    setBusy(true)
    try {
      onQuote(
        await requestQuote(
          escrowId,
          { origin_asset: assetId, refund_to: refundTo.trim() },
          auth
        )
      )
    } catch (err) {
      setError(describeApiError(err))
    } finally {
      setBusy(false)
    }
  }

  const shownRefundError = showErrors ? refundError : null

  return (
    <form onSubmit={handleSubmit} noValidate className="flex flex-col gap-4">
      <div className="grid gap-4 sm:grid-cols-2">
        <Field>
          <FieldLabel>Network</FieldLabel>
          <Select
            items={chainItems}
            value={chain}
            onValueChange={(value) => value && changeChain(value)}
          >
            <SelectTrigger aria-label="Network" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {chainItems.map((item) => (
                <SelectItem key={item.value} value={item.value}>
                  {item.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
        <Field>
          <FieldLabel>Token</FieldLabel>
          <Select
            items={items}
            value={assetId}
            onValueChange={(value) => value && setAssetId(value)}
          >
            <SelectTrigger aria-label="Token" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {items.map((item) => (
                <SelectItem key={item.value} value={item.value}>
                  {item.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
      </div>

      <Field data-invalid={shownRefundError ? true : undefined}>
        <FieldLabel htmlFor="funding-refund-to">Refund address</FieldLabel>
        <Input
          id="funding-refund-to"
          autoComplete="off"
          autoCapitalize="none"
          spellCheck={false}
          value={refundTo}
          onChange={(event) => {
            setRefundTo(event.target.value)
            setRefundEdited(true)
          }}
          aria-invalid={shownRefundError ? true : undefined}
          aria-describedby="funding-refund-hint funding-refund-error"
        />
        <FieldDescription id="funding-refund-hint">
          Your address on {chainName(chain)}. If the swap fails, your payment
          comes back here.
        </FieldDescription>
        <FieldError id="funding-refund-error">{shownRefundError}</FieldError>
      </Field>

      <FeedbackMessage
        feedback={error ? { kind: "error", message: error } : null}
      />
      <div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
        {onCancel && (
          <Button
            type="button"
            variant="outline"
            disabled={busy}
            onClick={onCancel}
          >
            Cancel
          </Button>
        )}
        <Button type="submit" disabled={busy}>
          {busy && <LoaderCircle aria-hidden className="animate-spin" />}
          {submitLabel}
        </Button>
      </div>
    </form>
  )
}

/** For visitors who are not the approver: no deposit address. */
export function FundingLocked({
  escrow,
  freelancer,
  expired,
}: {
  escrow: EscrowDetail
  freelancer: boolean
  /** The approver whose guest access needs renewing. */
  expired: boolean
}) {
  const approver = escrow.client?.approver
  if (freelancer) {
    return (
      <FormSection
        title="Waiting for payment"
        description={`The client pays in ${formatUsdc(escrow.total)} USDC. Only their approver wallet sees where to pay.`}
      />
    )
  }
  if (expired) {
    return (
      <FormSection
        title="Pay in"
        description="Renew your guest access above to see where to pay."
      />
    )
  }
  return (
    <FormSection title="Pay in">
      <p
        role="alert"
        className="flex items-start gap-2 rounded-xl bg-destructive/10 p-3 text-sm break-words text-destructive"
      >
        <CircleAlert aria-hidden className="mt-0.5 size-4 shrink-0" />
        <span className="min-w-0">
          Only the client&apos;s approver wallet sees where to pay. If you are
          the client, open this link where you joined, or sign in with that
          wallet.
          {approver && (
            <>
              {" "}
              Do not pay if <ApproverWallet approver={approver} /> is not your
              wallet: it approves every payout.
            </>
          )}
        </span>
      </p>
    </FormSection>
  )
}
