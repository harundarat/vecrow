export const DEFAULT_SIGNED_IN_PATH = "/app"

/**
 * Returns `next` if it is a path on this site, so a crafted `?next=` cannot
 * send the user to another origin or run a `javascript:` URL.
 */
export function safeNextPath(next: string | null | undefined): string {
  if (!next || !next.startsWith("/") || next.startsWith("//")) {
    return DEFAULT_SIGNED_IN_PATH
  }
  if (next.includes("\\")) return DEFAULT_SIGNED_IN_PATH
  return next
}
