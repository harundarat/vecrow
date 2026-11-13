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
