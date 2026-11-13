"use client"

import { useCallback, useMemo, useSyncExternalStore } from "react"
import { proveWallet, type ChallengeSigner } from "@/lib/api/auth"
import { renewGuestToken } from "@/lib/api/escrow"
import {
  guestAccessKey,
  parseGuestAccess,
  type GuestAccess,
} from "@/lib/guest-access"

const listeners = new Set<() => void>()

function read(key: string): string | null {
  try {
    return window.localStorage.getItem(key)
  } catch {
    return null
  }
}

function write(key: string, access: GuestAccess | null) {
  try {
    if (access) window.localStorage.setItem(key, JSON.stringify(access))
    else window.localStorage.removeItem(key)
  } catch {}
  listeners.forEach((listener) => listener())
}

function subscribe(listener: () => void) {
  listeners.add(listener)
  window.addEventListener("storage", listener)
  return () => {
    listeners.delete(listener)
    window.removeEventListener("storage", listener)
  }
}

/**
 * The guest token of one escrow, kept in localStorage so a visitor who
 * joined without an account still has it after a reload.
 */
export function useGuestAccess(escrowId: string) {
  const key = guestAccessKey(escrowId)
  const raw = useSyncExternalStore(
    subscribe,
    () => read(key),
    () => null
  )
  const access = useMemo(() => parseGuestAccess(raw), [raw])

  const save = useCallback(
    (next: GuestAccess | null) => write(key, next),
    [key]
  )

  /** Keeps the wallet but drops a token the server no longer accepts. */
  const expire = useCallback(() => {
    const current = parseGuestAccess(read(key))
    if (current?.token) write(key, { ...current, token: "", expires_at: "" })
  }, [key])

  /** Signs a login message with the same wallet for a new token. */
  const renew = useCallback(
    async (signer: ChallengeSigner) => {
      const current = parseGuestAccess(read(key))
      if (!current) throw new Error("You have not joined this escrow.")
      const res = await renewGuestToken(escrowId, await proveWallet(signer))
      write(key, {
        token: res.guest_token,
        expires_at: res.expires_at,
        wallet: current.wallet,
      })
    },
    [escrowId, key]
  )

  return { access, save, expire, renew }
}
