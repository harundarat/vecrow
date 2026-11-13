"use client"

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  useSyncExternalStore,
} from "react"
import {
  getMe,
  issueNonce,
  logout,
  verifyProof,
  type Account,
  type VerifyResponse,
} from "@/lib/api/auth"
import { ApiError, apiFetch, type RequestOptions } from "@/lib/api/client"
import type { WalletSigner } from "@/hooks/use-wallet-signer"

const TOKEN_KEY = "vecrow.session_token"

// The token lives in localStorage so a session survives reloads and is
// shared by every tab; useSyncExternalStore keeps tabs and renders in sync.
const tokenListeners = new Set<() => void>()

function readToken(): string | null {
  try {
    return window.localStorage.getItem(TOKEN_KEY)
  } catch {
    return null
  }
}

function writeToken(token: string | null) {
  try {
    if (token) window.localStorage.setItem(TOKEN_KEY, token)
    else window.localStorage.removeItem(TOKEN_KEY)
  } catch {}
  tokenListeners.forEach((listener) => listener())
}

function subscribeToken(listener: () => void) {
  tokenListeners.add(listener)
  const onStorage = (event: StorageEvent) => {
    if (event.key === TOKEN_KEY || event.key === null) listener()
  }
  window.addEventListener("storage", onStorage)
  return () => {
    tokenListeners.delete(listener)
    window.removeEventListener("storage", onStorage)
  }
}

export type AuthStatus =
  "loading" | "authenticated" | "unauthenticated" | "error"

export type SignInSigner = Pick<WalletSigner, "scheme" | "sign"> & {
  address: string
}

type AuthContextValue = {
  status: AuthStatus
  account: Account | null
  token: string | null
  /** Set when status is "error": the session could not be checked. */
  error: string | null
  signIn: (signer: SignInSigner) => Promise<VerifyResponse>
  signOut: () => Promise<void>
  /** Loads the account again, e.g. after an "error" status. */
  reload: () => void
  /** Calls the API with the session token and signs out on a 401. */
  request: <T>(
    path: string,
    options?: Omit<RequestOptions, "token">
  ) => Promise<T>
}

const AuthContext = createContext<AuthContextValue | null>(null)

type Me =
  | { token: string; account: Account; error?: undefined }
  | { token: string; account?: undefined; error: string }

export function AuthProvider({ children }: React.PropsWithChildren) {
  // undefined while rendering on the server and hydrating: not known yet.
  const token = useSyncExternalStore<string | null | undefined>(
    subscribeToken,
    readToken,
    () => undefined
  )
  const [me, setMe] = useState<Me | null>(null)
  const [reloads, setReloads] = useState(0)

  useEffect(() => {
    if (!token) return
    const controller = new AbortController()
    getMe(token, controller.signal).then(
      (account) => setMe({ token, account }),
      (err: unknown) => {
        if (controller.signal.aborted) return
        if (err instanceof ApiError && err.status === 401) {
          writeToken(null)
          return
        }
        setMe({
          token,
          error:
            err instanceof Error ? err.message : "Could not load your account.",
        })
      }
    )
    return () => controller.abort()
  }, [token, reloads])

  const current = token && me?.token === token ? me : null

  let status: AuthStatus
  if (token === undefined) status = "loading"
  else if (token === null) status = "unauthenticated"
  else if (!current) status = "loading"
  else status = current.account ? "authenticated" : "error"

  const signIn = useCallback(async (signer: SignInSigner) => {
    const challenge = await issueNonce(signer.scheme, signer.address)
    const signature = await signer.sign({
      message: challenge.message,
      nonce: challenge.nonce,
      recipient: challenge.recipient,
    })
    const res = await verifyProof({ nonce: challenge.nonce, ...signature })
    setMe({ token: res.token, account: res.account })
    writeToken(res.token)
    return res
  }, [])

  const signOut = useCallback(async () => {
    const current = readToken()
    writeToken(null)
    setMe(null)
    if (!current) return
    try {
      await logout(current)
    } catch {
      // The local session is already gone; an expired or unreachable
      // server-side session needs no further action.
    }
  }, [])

  const request = useCallback(
    async <T,>(path: string, options?: Omit<RequestOptions, "token">) => {
      try {
        return await apiFetch<T>(path, { ...options, token: readToken() })
      } catch (err) {
        if (err instanceof ApiError && err.status === 401) writeToken(null)
        throw err
      }
    },
    []
  )

  const reload = useCallback(() => setReloads((n) => n + 1), [])

  const value = useMemo<AuthContextValue>(
    () => ({
      status,
      account: current?.account ?? null,
      token: token ?? null,
      error: current?.error ?? null,
      signIn,
      signOut,
      reload,
      request,
    }),
    [status, current, token, signIn, signOut, reload, request]
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth() {
  const context = useContext(AuthContext)
  if (!context) throw new Error("useAuth must be used within AuthProvider")
  return context
}
