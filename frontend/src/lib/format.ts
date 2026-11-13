/** Shortens long wallet addresses for display; short NEAR names stay whole. */
export function shortenAddress(address: string): string {
  if (address.length <= 18) return address
  return `${address.slice(0, 6)}…${address.slice(-4)}`
}
