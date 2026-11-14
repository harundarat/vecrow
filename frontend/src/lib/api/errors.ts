import { ApiError } from "./client"

/** Turns an API or network error into a sentence for the user. */
export function describeApiError(err: unknown): string {
  if (err instanceof ApiError || (err instanceof Error && err.message)) {
    return toSentence(err.message)
  }
  return "Something went wrong. Please try again."
}

// Backend messages are lowercase clauses ("username is already taken").
function toSentence(message: string): string {
  const text = message.trim()
  if (!text) return "Something went wrong. Please try again."
  const capitalized = text[0].toUpperCase() + text.slice(1)
  return /[.!?]$/.test(capitalized) ? capitalized : `${capitalized}.`
}

/**
 * Sends a call that waits for a NEAR transaction. The server answers only
 * once it is final, so a dropped connection does not mean it failed.
 */
export async function sendTransaction<T>(send: () => Promise<T>): Promise<T> {
  try {
    return await send()
  } catch (err) {
    if (err instanceof ApiError && err.status === 0) {
      throw new Error(
        "The server did not answer in time. It may have gone through, so check the escrow before you try again."
      )
    }
    throw err
  }
}
