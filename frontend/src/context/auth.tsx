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
  logout,
  proveWallet,
  verifyEmailCode,
  verifyProof,
  type Account,
  type ChallengeSigner,
} from "@/lib/api/auth"
import {
  ApiError,
  apiFetch,
  type Requester,
  type RequestOptions,
} from "@/lib/api/client"

const SESSION_KEY = "vecrow.session"

type StoredSession = {
  token: string
  /** Opened with an email code: may only view the account and link or unlink wallets. */
  recovery: boolean
  expires_at: string
}

// The session lives in localStorage so it survives reloads and is shared by
// every tab; useSyncExternalStore keeps tabs and renders in sync. The raw
// string is the snapshot because it compares by value.
const sessionListeners = new Set<() => void>()

function readRawSession(): string | null {
  try {
    return window.localStorage.getItem(SESSION_KEY)
  } catch {
    return null
  }
}

function parseSession(raw: string | null): StoredSession | null {
  if (!raw) return null
  try {
    const value = JSON.parse(raw) as Partial<StoredSession> | null
    if (typeof value?.token !== "string" || !value.token) return null
    return {
      token: value.token,
      recovery: value.recovery === true,
      expires_at: typeof value.expires_at === "string" ? value.expires_at : "",
    }
  } catch {
    return null
  }
}

function readSession(): StoredSession | null {
  return parseSession(readRawSession())
}

function writeSession(session: StoredSession | null) {
  try {
    if (session) {
      window.localStorage.setItem(SESSION_KEY, JSON.stringify(session))
    } else {
      window.localStorage.removeItem(SESSION_KEY)
    }
  } catch {}
  sessionListeners.forEach((listener) => listener())
}

function subscribeSession(listener: () => void) {
  sessionListeners.add(listener)
  const onStorage = (event: StorageEvent) => {
    if (event.key === SESSION_KEY || event.key === null) listener()
  }
  window.addEventListener("storage", onStorage)
  return () => {
    sessionListeners.delete(listener)
    window.removeEventListener("storage", onStorage)
  }
}

/** Replaces the stored session and ends the one it replaces, if any. */
function startSession(next: StoredSession) {
  const previous = readSession()
  writeSession(next)
  if (previous && previous.token !== next.token) {
    logout(previous.token).catch(() => {})
  }
}

const WALLET_SESSION_REQUIRED =
  "You signed in with an email code, so you can only view your account and " +
  "link or unlink wallets. Sign in with a wallet to do this."

export type AuthStatus =
  "loading" | "authenticated" | "unauthenticated" | "error"

type AuthContextValue = {
  status: AuthStatus
  account: Account | null
  token: string | null
  /** True when the session was opened with an email code instead of a wallet. */
  recovery: boolean
  /** When the session ends, or null when unknown. */
  expiresAt: string | null
  /** Set when status is "error": the session could not be checked. */
  error: string | null
  signIn: (signer: ChallengeSigner) => Promise<Account>
  signInWithEmail: (email: string, code: string) => Promise<Account>
  signOut: () => Promise<void>
  /** Loads the account again, e.g. after an "error" status. */
  reload: () => void
  /** Stores an account the API returned after a change. */
  setAccount: (account: Account) => void
  /**
   * Calls the API with the session token. A 401 re-checks the session and
   * signs out if it has ended.
   */
  request: Requester
}

const AuthContext = createContext<AuthContextValue | null>(null)

type Me =
  | { token: string; account: Account; error?: undefined }
  | { token: string; account?: undefined; error: string }

export function AuthProvider({ children }: React.PropsWithChildren) {
  // undefined while rendering on the server and hydrating: not known yet.
  const rawSession = useSyncExternalStore<string | null | undefined>(
    subscribeSession,
    readRawSession,
    () => undefined
  )
  const session = useMemo(
    () => (rawSession === undefined ? undefined : parseSession(rawSession)),
    [rawSession]
  )
  const token = session === undefined ? undefined : (session?.token ?? null)
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
          writeSession(null)
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

  const signIn = useCallback(async (signer: ChallengeSigner) => {
    const res = await verifyProof(await proveWallet(signer))
    setMe({ token: res.token, account: res.account })
    startSession({
      token: res.token,
      recovery: false,
      expires_at: res.expires_at,
    })
    return res.account
  }, [])

  const signInWithEmail = useCallback(async (email: string, code: string) => {
    const res = await verifyEmailCode(email, code)
    setMe({ token: res.token, account: res.account })
    startSession({
      token: res.token,
      recovery: res.recovery,
      expires_at: res.expires_at,
    })
    return res.account
  }, [])

  const signOut = useCallback(async () => {
    const current = readSession()
    writeSession(null)
    setMe(null)
    if (!current) return
    try {
      await logout(current.token)
    } catch {
      // The local session is already gone; an expired or unreachable
      // server-side session needs no further action.
    }
  }, [])

  const reload = useCallback(() => setReloads((n) => n + 1), [])

  const setAccount = useCallback((account: Account) => {
    setMe((prev) => (prev ? { token: prev.token, account } : prev))
  }, [])

  const request = useCallback<Requester>(
    async <T,>(path: string, options?: Omit<RequestOptions, "token">) => {
      const session = readSession()
      try {
        return await apiFetch<T>(path, { ...options, token: session?.token })
      } catch (err) {
        if (err instanceof ApiError && session) {
          // A wrong one-time code is also a 401, so ask the server whether
          // the session itself has ended before dropping it.
          if (err.status === 401) setReloads((n) => n + 1)
          if (err.status === 403 && session.recovery) {
            throw new ApiError(err.status, err.code, WALLET_SESSION_REQUIRED)
          }
        }
        throw err
      }
    },
    []
  )

  const value = useMemo<AuthContextValue>(
    () => ({
      status,
      account: current?.account ?? null,
      token: token ?? null,
      recovery: session?.recovery ?? false,
      expiresAt: session?.expires_at || null,
      error: current?.error ?? null,
      signIn,
      signInWithEmail,
      signOut,
      reload,
      setAccount,
      request,
    }),
    [
      status,
      current,
      token,
      session,
      signIn,
      signInWithEmail,
      signOut,
      reload,
      setAccount,
      request,
    ]
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth() {
  const context = useContext(AuthContext)
  if (!context) throw new Error("useAuth must be used within AuthProvider")
  return context
}
