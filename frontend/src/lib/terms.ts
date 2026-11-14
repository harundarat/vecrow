import type { SignRequest } from "./api/auth"
import { termsNonce } from "./wallet-encoding"

/**
 * Terms are signed exactly as the backend sends them. A NEAR wallet also
 * signs the nonce and recipient, and the contract only accepts sha256 of the
 * text and its own account there, so a mismatch is caught before the wallet
 * opens.
 */
export function assertTermsRequest(request: SignRequest, contractId: string) {
  if (
    request.recipient !== contractId ||
    request.nonce.toLowerCase() !== termsNonce(request.message)
  ) {
    throw new Error(
      "The terms from the server do not match this escrow. Reload the page and try again."
    )
  }
}
