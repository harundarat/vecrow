"use client"

import { useState } from "react"
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { useWalletSigners, type WalletChain } from "@/hooks/use-wallet-signer"
import { proveWallet } from "@/lib/api/auth"
import {
  buildTerms,
  submitCounterparty,
  type Approver,
  type EscrowDetail,
  type SubmitResponse,
  type WalletRef,
} from "@/lib/api/escrow"
import { nearAccountError } from "@/lib/escrow-rules"
import { assertTermsRequest } from "@/lib/terms"
import { describeWalletError } from "@/lib/wallet-errors"
import { JoinFooter, type JoinStep } from "./join-section"
import { ConnectedWallet, WalletChoice } from "./wallet-choice"

const CHAINS: WalletChain[] = ["near", "evm", "solana"]

export function JoinClientForm({
  escrow,
  onJoined,
}: {
  escrow: EscrowDetail
  onJoined: (res: SubmitResponse, wallet: WalletRef) => void
}) {
  const signers = useWalletSigners()
  const [chain, setChain] = useState<WalletChain | null>(null)
  const signer = chain ? signers[chain] : null
  const address = signer?.address ?? ""
  const [refundAccount, setRefundAccount] = useState("")
  const [showErrors, setShowErrors] = useState(false)
  const [step, setStep] = useState<JoinStep | null>(null)
  const [error, setError] = useState<string | null>(null)

  const refundError = nearAccountError(refundAccount)

  async function connect(next: WalletChain) {
    setError(null)
    setStep("connecting")
    try {
      const connected = await signers[next].connect()
      setChain(next)
      // A NEAR approver is usually also where refunds should go.
      if (next === "near" && !refundAccount) setRefundAccount(connected)
    } catch (err) {
      setError(describeWalletError(err))
    } finally {
      setStep(null)
    }
  }

  async function disconnect() {
    if (!signer) return
    setError(null)
    try {
      await signer.disconnect()
      setChain(null)
    } catch (err) {
      setError(describeWalletError(err))
    }
  }

  async function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (step) return
    setShowErrors(true)
    setError(null)
    if (!signer || !address) {
      setError("Connect the wallet you approve with first.")
      return
    }
    if (refundError) return

    try {
      let approver: Approver
      if (signer.scheme === "nep413") {
        // The terms name the key that signs, so it is read from a signature
        // before the terms can be built.
        setStep("key")
        const proof = await proveWallet({
          scheme: "nep413",
          address,
          sign: signer.sign,
        })
        if (!proof.public_key) {
          throw new Error("Your NEAR wallet did not share its public key.")
        }
        approver = {
          scheme: "nep413",
          account_id: address,
          public_key: proof.public_key,
        }
      } else {
        approver = { scheme: signer.scheme, address }
      }
      const details = { approver, refund_account: refundAccount.trim() }

      setStep("signing")
      const terms = await buildTerms(escrow.id, details)
      assertTermsRequest(terms, escrow.contract_id)
      const signed = await signer.sign(terms)
      setStep("sending")
      const res = await submitCounterparty(escrow.id, {
        ...details,
        deadline_ms: terms.terms.deadline_ms,
        signature: signed.signature,
      })
      setStep("done")
      onJoined(res, { scheme: signer.scheme, address })
    } catch (err) {
      setError(describeWalletError(err))
      setStep(null)
    }
  }

  const shownRefundError = showErrors ? refundError : null

  return (
    <form onSubmit={handleSubmit} noValidate className="flex flex-col gap-6">
      <Field>
        <FieldLabel>Approver wallet</FieldLabel>
        {chain && address ? (
          <ConnectedWallet
            chain={chain}
            address={address}
            busy={step !== null}
            onChange={() => void disconnect()}
          />
        ) : (
          <WalletChoice
            chains={CHAINS}
            busy={step !== null}
            onConnect={(next) => void connect(next)}
          />
        )}
        <FieldDescription>
          You approve each milestone by signing with this wallet. It cannot be
          changed later.
        </FieldDescription>
      </Field>

      <Field data-invalid={shownRefundError ? true : undefined}>
        <FieldLabel htmlFor="join-refund-account">Refund account</FieldLabel>
        <Input
          id="join-refund-account"
          placeholder="alice.near"
          autoComplete="off"
          autoCapitalize="none"
          spellCheck={false}
          value={refundAccount}
          onChange={(event) => setRefundAccount(event.target.value)}
          aria-invalid={shownRefundError ? true : undefined}
          aria-describedby="join-refund-hint join-refund-error"
        />
        <FieldDescription id="join-refund-hint">
          A NEAR account. If the work stops, money for unfinished milestones
          comes back here.
        </FieldDescription>
        <FieldError id="join-refund-error">{shownRefundError}</FieldError>
      </Field>

      <JoinFooter step={step} error={error} />
    </form>
  )
}
