import { decodeSignedDelegate } from "near-api-js"
import { looseBase64ToBytes, bytesToBase64 } from "./wallet-encoding"

/** The contract methods the freelancer calls through the relayer. */
export type FreelancerMethod =
  "submit_milestone" | "refund_unsubmitted" | "cancel"

export type FreelancerCall = {
  method: FreelancerMethod
  args: { escrow_id: string; milestone?: number }
}

const TGAS = BigInt(10) ** BigInt(12)

/** The relayer refuses a delegate action with more gas than this. */
export const RELAYER_MAX_GAS = BigInt(100) * TGAS

// A refund pays out one transfer with its callback, which the contract
// reserves 30 TGas for; the others only change state.
const GAS: Record<FreelancerMethod, bigint> = {
  submit_milestone: BigInt(30) * TGAS,
  refund_unsubmitted: BigInt(50) * TGAS,
  cancel: BigInt(30) * TGAS,
}

export function submitCall(
  escrowId: string,
  milestone: number
): FreelancerCall {
  checkMilestone(milestone)
  return {
    method: "submit_milestone",
    args: { escrow_id: escrowId, milestone },
  }
}

/** Refunds one Pending milestone, or every Pending one when `milestone` is null. */
export function refundCall(
  escrowId: string,
  milestone: number | null
): FreelancerCall {
  if (milestone === null) {
    return { method: "refund_unsubmitted", args: { escrow_id: escrowId } }
  }
  checkMilestone(milestone)
  return {
    method: "refund_unsubmitted",
    args: { escrow_id: escrowId, milestone },
  }
}

export function cancelCall(escrowId: string): FreelancerCall {
  return { method: "cancel", args: { escrow_id: escrowId } }
}

/** What near-connect's `signDelegateActions` takes for one call. */
export type DelegateActionRequest = {
  receiverId: string
  actions: Array<{
    type: "FunctionCall"
    params: {
      methodName: string
      args: Record<string, unknown>
      gas: string
      deposit: string
    }
  }>
}

/** Exactly one function call to the escrow contract, with no deposit. */
export function delegateActionRequest(
  contractId: string,
  call: FreelancerCall
): DelegateActionRequest {
  return {
    receiverId: contractId,
    actions: [
      {
        type: "FunctionCall",
        params: {
          methodName: call.method,
          args: { ...call.args },
          gas: GAS[call.method].toString(),
          deposit: "0",
        },
      },
    ],
  }
}

/**
 * Checks that what the wallet signed is the call we asked for, from
 * `senderId`, and returns it as standard base64. Wallets build the delegate
 * action themselves, so a mismatch is caught here with a clear message
 * instead of a refusal from the relayer.
 */
export function checkSignedDelegate(
  signed: string,
  expected: { senderId: string; contractId: string; call: FreelancerCall }
): string {
  let bytes: Uint8Array
  let delegate: ReturnType<typeof decodeSignedDelegate>
  try {
    bytes = looseBase64ToBytes(signed)
    delegate = decodeSignedDelegate(bytes)
  } catch {
    throw new Error("Your wallet returned a signed action Vecrow cannot read.")
  }
  const action = delegate.delegateAction
  if (action.senderId !== expected.senderId) {
    throw new Error(
      `Your wallet signed as ${action.senderId}, not ${expected.senderId}.`
    )
  }
  const call = action.actions.length === 1 && action.actions[0].functionCall
  if (
    action.receiverId !== expected.contractId ||
    !call ||
    call.methodName !== expected.call.method ||
    call.deposit !== BigInt(0) ||
    call.gas <= BigInt(0) ||
    call.gas > RELAYER_MAX_GAS ||
    !sameArgs(call.args, expected.call.args)
  ) {
    throw new Error("Your wallet signed a different action. Try again.")
  }
  return bytesToBase64(bytes)
}

// The decoder returns the argument bytes as a plain array.
function sameArgs(
  raw: ArrayLike<number>,
  want: FreelancerCall["args"]
): boolean {
  try {
    const text = new TextDecoder().decode(Uint8Array.from(raw))
    const got = JSON.parse(text) as Record<string, unknown>
    const keys = Object.keys(got)
    return (
      keys.length === Object.keys(want).length &&
      keys.every((key) => got[key] === want[key as keyof typeof want])
    )
  } catch {
    return false
  }
}

function checkMilestone(milestone: number) {
  if (!Number.isSafeInteger(milestone) || milestone < 0) {
    throw new Error("A milestone index is a non-negative integer")
  }
}
