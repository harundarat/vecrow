import type { SignRequest } from "./api/auth"
import { bytesToHex } from "./wallet-encoding"

// The contract rebuilds these messages from the call arguments and checks
// the approver's signature over them, so they must match its text byte for
// byte. Unlike the terms, no endpoint returns them.

export type MilestoneAction = "approve_milestone" | "request_revision"

export const ACTION_NONCE_BYTES = 32

/** How long a signed action stays valid; long enough to sign and send. */
export const ACTION_TTL_MS = 15 * 60 * 1000

const NONCE = /^[0-9a-f]{64}$/
const ESCROW_ID = /^[a-z0-9_-]{1,64}$/

/** 32 random bytes as 64 lowercase hex characters, used once per approver. */
export function actionNonce(
  bytes: Uint8Array = crypto.getRandomValues(new Uint8Array(ACTION_NONCE_BYTES))
): string {
  if (bytes.length !== ACTION_NONCE_BYTES) {
    throw new Error(`An action nonce is ${ACTION_NONCE_BYTES} bytes`)
  }
  return bytesToHex(bytes).slice(2)
}

export function actionDeadline(nowMs: number = Date.now()): number {
  return nowMs + ACTION_TTL_MS
}

export function milestoneActionMessage(
  action: MilestoneAction,
  contractId: string,
  escrowId: string,
  milestone: number,
  nonce: string,
  deadlineMs: number
): string {
  if (!Number.isSafeInteger(milestone) || milestone < 0) {
    throw new Error("Milestone must be a 0-based index")
  }
  return [
    `Vecrow ${action}`,
    `Contract: ${contractId}`,
    `Escrow: ${checkEscrowId(escrowId)}`,
    `Milestone: ${milestone}`,
    `Nonce: ${checkNonce(nonce)}`,
    `Deadline: ${checkDeadline(deadlineMs)}`,
  ].join("\n")
}

export function cancelMessage(
  contractId: string,
  escrowId: string,
  nonce: string,
  deadlineMs: number
): string {
  return [
    "Vecrow cancel",
    `Contract: ${contractId}`,
    `Escrow: ${checkEscrowId(escrowId)}`,
    `Nonce: ${checkNonce(nonce)}`,
    `Deadline: ${checkDeadline(deadlineMs)}`,
  ].join("\n")
}

/** What the approver signs, and the fields sent next to the signature. */
export type ApproverActionRequest = SignRequest & { deadline_ms: number }

/**
 * A fresh action for the approver's wallet. NEAR wallets sign the nonce
 * bytes and the contract as recipient; other wallets sign the text only.
 */
export function approverActionRequest(
  contractId: string,
  message: (nonce: string, deadlineMs: number) => string,
  nowMs: number = Date.now()
): ApproverActionRequest {
  const nonce = actionNonce()
  const deadline = actionDeadline(nowMs)
  return {
    message: message(nonce, deadline),
    nonce,
    recipient: contractId,
    deadline_ms: deadline,
  }
}

function checkEscrowId(id: string): string {
  if (!ESCROW_ID.test(id)) throw new Error("Invalid escrow ID")
  return id
}

function checkNonce(nonce: string): string {
  if (!NONCE.test(nonce)) {
    throw new Error("Nonce must be 64 lowercase hex characters")
  }
  return nonce
}

function checkDeadline(deadlineMs: number): string {
  if (!Number.isSafeInteger(deadlineMs) || deadlineMs <= 0) {
    throw new Error("Deadline must be milliseconds since the epoch")
  }
  return String(deadlineMs)
}
