import { ApiError } from "./api/client"
import { describeApiError } from "./api/errors"

export class WalletCancelledError extends Error {
  constructor() {
    super("The wallet window was closed before a wallet connected.")
    this.name = "WalletCancelledError"
  }
}

const REJECTED = /reject|denied|declin|cancel/i

/** Turns errors from wallets and the API into a sentence for the user. */
export function describeWalletError(err: unknown): string {
  if (err instanceof WalletCancelledError) return err.message
  if (err instanceof ApiError) return describeApiError(err)
  if (isUserRejection(err)) return "The request was rejected in your wallet."
  return describeApiError(err)
}

function isUserRejection(err: unknown): boolean {
  if (typeof err !== "object" || err === null) return false
  const { code, message } = err as { code?: unknown; message?: unknown }
  // 4001 is EIP-1193's user rejection; ethers wraps it as ACTION_REJECTED.
  if (code === 4001 || code === "ACTION_REJECTED") return true
  return typeof message === "string" && REJECTED.test(message)
}
