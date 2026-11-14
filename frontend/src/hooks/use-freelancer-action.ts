"use client"

import { useCallback } from "react"
import { useNearWallet } from "@/context/near"
import { sendTransaction } from "@/lib/api/errors"
import type { DelegateBody, Escrow } from "@/lib/api/escrow"
import {
  checkSignedDelegate,
  delegateActionRequest,
  type FreelancerCall,
} from "@/lib/delegate-action"
import { connectWalletAs, useWalletSigner } from "./use-wallet-signer"

export type FreelancerStep = "connecting" | "signing" | "sending"

/**
 * Signs a contract call as the escrow's freelancer, as a delegate action
 * the relayer pays the gas for, and sends it with `send`.
 */
export function useFreelancerAction(escrow: Escrow) {
  const signer = useWalletSigner("near")
  const { signDelegateAction } = useNearWallet()
  const freelancer = escrow.freelancer?.account
  const contractId = escrow.contract_id

  return useCallback(
    async (
      call: FreelancerCall,
      send: (body: DelegateBody) => Promise<unknown>,
      onStep: (step: FreelancerStep) => void
    ) => {
      if (!freelancer) throw new Error("This escrow has no freelancer yet.")
      onStep("connecting")
      await connectWalletAs(signer, freelancer)
      onStep("signing")
      const signed = await signDelegateAction(
        delegateActionRequest(contractId, call)
      )
      const delegate = checkSignedDelegate(signed, {
        senderId: freelancer,
        contractId,
        call,
      })
      onStep("sending")
      await sendTransaction(() => send({ delegate_action: delegate }))
    },
    [freelancer, contractId, signer, signDelegateAction]
  )
}
