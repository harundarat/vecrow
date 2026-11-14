"use client"

import { useCallback } from "react"
import { sendTransaction } from "@/lib/api/errors"
import type { ApproverSignature, Escrow } from "@/lib/api/escrow"
import { approverActionRequest } from "@/lib/approver-action"
import { approverWallet } from "@/lib/escrow-viewer"
import {
  connectWalletAs,
  SCHEME_CHAIN,
  useWalletSigners,
} from "./use-wallet-signer"

export type ApproverStep = "connecting" | "signing" | "sending"

/**
 * Signs an action with the escrow's approver wallet and sends it. `message`
 * builds the text from a fresh nonce and deadline; `send` posts the result.
 */
export function useApproverAction(escrow: Escrow) {
  const signers = useWalletSigners()
  const approver = escrow.client?.approver
  const contractId = escrow.contract_id

  return useCallback(
    async (
      message: (nonce: string, deadlineMs: number) => string,
      send: (signature: ApproverSignature) => Promise<unknown>,
      onStep: (step: ApproverStep) => void
    ) => {
      if (!approver) throw new Error("This escrow has no approver yet.")
      const wallet = approverWallet(approver)
      const signer = signers[SCHEME_CHAIN[wallet.scheme]]
      onStep("connecting")
      await connectWalletAs(signer, wallet.address)

      const request = approverActionRequest(contractId, message)
      onStep("signing")
      const signed = await signer.sign(request)
      // The contract checks a NEAR signature against the key in the terms,
      // not against any key of the account.
      if (
        approver.scheme === "nep413" &&
        signed.public_key !== approver.public_key
      ) {
        throw new Error(
          "Your NEAR wallet signed with another key than the one in this escrow. Use the key you joined with."
        )
      }
      onStep("sending")
      await sendTransaction(() =>
        send({
          nonce: request.nonce,
          deadline_ms: request.deadline_ms,
          signature: signed.signature,
        })
      )
    },
    [approver, contractId, signers]
  )
}
