"use client"

import { useState } from "react"
import {
  followAccount,
  SplitFields,
  splitIsValid,
  splitShares,
  type SplitRow,
} from "@/components/split-fields"
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field"
import { useWalletSigner } from "@/hooks/use-wallet-signer"
import {
  buildTerms,
  submitCounterparty,
  type EscrowDetail,
  type SubmitResponse,
  type WalletRef,
} from "@/lib/api/escrow"
import { checkSplit } from "@/lib/escrow-rules"
import { newRowKey } from "@/lib/row-key"
import { assertTermsRequest } from "@/lib/terms"
import { describeWalletError } from "@/lib/wallet-errors"
import { JoinFooter, type JoinStep } from "./join-section"
import { ConnectedWallet, WalletChoice } from "./wallet-choice"

export function JoinFreelancerForm({
  escrow,
  onJoined,
}: {
  escrow: EscrowDetail
  onJoined: (res: SubmitResponse, wallet: WalletRef) => void
}) {
  const near = useWalletSigner("near")
  const account = near.address ?? ""
  const [split, setSplit] = useState<SplitRow[]>(() => [
    { key: newRowKey(), account, percent: "100" },
  ])
  const [followed, setFollowed] = useState(account)
  const [showErrors, setShowErrors] = useState(false)
  const [step, setStep] = useState<JoinStep | null>(null)
  const [error, setError] = useState<string | null>(null)

  if (account !== followed) {
    setFollowed(account)
    setSplit((rows) => followAccount(rows, followed, account))
  }

  const check = checkSplit(
    split.map((row) => ({ account: row.account, percent: row.percent }))
  )

  async function connect() {
    setError(null)
    setStep("connecting")
    try {
      await near.connect()
    } catch (err) {
      setError(describeWalletError(err))
    } finally {
      setStep(null)
    }
  }

  async function disconnect() {
    setError(null)
    try {
      await near.disconnect()
    } catch (err) {
      setError(describeWalletError(err))
    }
  }

  async function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (step) return
    setShowErrors(true)
    setError(null)
    if (!account) {
      setError("Connect your NEAR wallet first.")
      return
    }
    if (!splitIsValid(check)) return

    const details = { freelancer: account, split: splitShares(split, check) }
    try {
      setStep("signing")
      const terms = await buildTerms(escrow.id, details)
      assertTermsRequest(terms, escrow.contract_id)
      const signed = await near.sign(terms)
      setStep("sending")
      const res = await submitCounterparty(escrow.id, {
        ...details,
        deadline_ms: terms.terms.deadline_ms,
        signature: signed.signature,
        public_key: signed.public_key,
      })
      setStep("done")
      onJoined(res, { scheme: "nep413", address: account })
    } catch (err) {
      setError(describeWalletError(err))
      setStep(null)
    }
  }

  return (
    <form onSubmit={handleSubmit} noValidate className="flex flex-col gap-6">
      <Field>
        <FieldLabel>Your NEAR account</FieldLabel>
        {account ? (
          <ConnectedWallet
            chain="near"
            address={account}
            busy={step !== null}
            onChange={() => void disconnect()}
          />
        ) : (
          <WalletChoice
            chains={["near"]}
            busy={step !== null}
            onConnect={() => void connect()}
          />
        )}
        <FieldDescription>
          You sign the terms with this wallet. Payments go to the split below.
        </FieldDescription>
      </Field>

      <SplitFields
        split={split}
        onSplitChange={setSplit}
        check={check}
        showErrors={showErrors}
      />

      <JoinFooter step={step} error={error} />
    </form>
  )
}
