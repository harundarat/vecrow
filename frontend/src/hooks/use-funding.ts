"use client"

import { useCallback, useEffect, useState } from "react"
import { ApiError } from "@/lib/api/client"
import { describeApiError } from "@/lib/api/errors"
import {
  getFunding,
  type Funding,
  type FundingAuth,
  type FundingQuote,
} from "@/lib/api/funding"

// A swap takes seconds to minutes; the backend asks 1Click at most this
// often anyway.
const POLL_MS = 5000

type State = { funding: Funding | null; loaded: boolean; error: string | null }

/** The escrow's latest quote and its status, refreshed while it is pending. */
export function useFunding(escrowId: string, auth: FundingAuth | null) {
  const token = auth?.token ?? null
  const guestToken = auth?.guestToken ?? null
  const [state, setState] = useState<State>({
    funding: null,
    loaded: false,
    error: null,
  })
  const [loads, setLoads] = useState(0)

  useEffect(() => {
    if (!token && !guestToken) return
    const controller = new AbortController()
    getFunding(escrowId, { token, guestToken }, controller.signal).then(
      (funding) => setState({ funding, loaded: true, error: null }),
      (err: unknown) => {
        if (controller.signal.aborted) return
        if (err instanceof ApiError && err.status === 404) {
          setState({ funding: null, loaded: true, error: null })
          return
        }
        setState((prev) => ({
          ...prev,
          loaded: true,
          error: describeApiError(err),
        }))
      }
    )
    return () => controller.abort()
  }, [escrowId, token, guestToken, loads])

  useEffect(() => {
    if (state.funding?.status !== "PENDING") return
    const timer = setTimeout(() => setLoads((n) => n + 1), POLL_MS)
    return () => clearTimeout(timer)
  }, [state])

  /** Shows a quote just requested, before the next poll. */
  const show = useCallback((quote: FundingQuote) => {
    setState({
      funding: {
        status: quote.status,
        oneclick_status: quote.oneclick_status,
        escrow_status: "Created",
        quote,
      },
      loaded: true,
      error: null,
    })
  }, [])

  return { ...state, show }
}
