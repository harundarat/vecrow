"use client"

import { useCallback, useEffect, useRef, useState } from "react"
import { useAuth } from "@/context/auth"
import { ApiError } from "@/lib/api/client"
import { describeApiError } from "@/lib/api/errors"
import {
  getEscrow,
  type EscrowDetail,
  type EscrowStatus,
} from "@/lib/api/escrow"
import { useGuestAccess } from "./use-guest-access"

// Approved turns into Created within a minute or two, so it is polled
// often; a pending submission waits for a person and is polled slowly.
const POLL_MS: Partial<Record<EscrowStatus, number>> = {
  Approved: 4000,
  PendingApproval: 15000,
}

type State = { escrow: EscrowDetail | null; error: string | null }

/**
 * Loads an escrow for the link page with the session and the guest token,
 * when there are any, and keeps it fresh while it waits for someone.
 */
export function useEscrowDetail(id: string) {
  const { status, token, recovery, reload: recheckSession } = useAuth()
  const guest = useGuestAccess(id)
  // The API refuses a recovery session here, so read as a visitor.
  const sessionToken = recovery ? null : token
  const guestToken = guest.access?.token || null
  const { expire } = guest

  const [state, setState] = useState<State>({ escrow: null, error: null })
  const [loads, setLoads] = useState(0)
  const [reopened, setReopened] = useState(false)
  const lastStatus = useRef<EscrowStatus | null>(null)

  useEffect(() => {
    if (status === "loading") return
    const controller = new AbortController()
    getEscrow(id, { token: sessionToken, guestToken }, controller.signal).then(
      (escrow) => {
        if (lastStatus.current === "Approved" && escrow.status === "Open") {
          setReopened(true)
        }
        lastStatus.current = escrow.status
        setState({ escrow, error: null })
      },
      (err: unknown) => {
        if (controller.signal.aborted) return
        if (err instanceof ApiError && err.status === 401) {
          // Dropping the token loads the escrow again without it.
          if (guestToken) return expire()
          if (sessionToken) recheckSession()
        }
        setState((prev) =>
          prev.escrow
            ? prev
            : {
                escrow: null,
                error:
                  err instanceof ApiError && err.status === 404
                    ? "This escrow does not exist. Check the link."
                    : describeApiError(err),
              }
        )
      }
    )
    return () => controller.abort()
  }, [id, status, sessionToken, guestToken, loads, expire, recheckSession])

  const current = state.escrow?.status
  useEffect(() => {
    const ms = current ? POLL_MS[current] : undefined
    if (!ms) return
    const timer = setTimeout(() => setLoads((n) => n + 1), ms)
    return () => clearTimeout(timer)
  }, [current, state])

  const reload = useCallback(() => setLoads((n) => n + 1), [])

  return { ...state, reload, reopened, guest }
}
