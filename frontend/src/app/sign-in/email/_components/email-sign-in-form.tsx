"use client"

import { useRouter } from "next/navigation"
import { useEffect, useRef, useState } from "react"
import { CircleAlert, LoaderCircle, MailCheck } from "lucide-react"
import { CODE_LENGTH, OneTimeCodeInput } from "@/components/one-time-code-input"
import { Button } from "@/components/ui/button"
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { useAuth } from "@/context/auth"
import { requestEmailCode } from "@/lib/api/auth"
import { describeApiError } from "@/lib/api/errors"
import { formatTime } from "@/lib/format"
import { DEFAULT_SIGNED_IN_PATH } from "@/lib/redirect"

const RECOVERY_LANDING_PATH = "/app/settings#wallets"

type CodeRequest = {
  email: string
  /** The server's answer, shown as is: it never says whether the email has an account. */
  message: string
  expiresInSeconds: number
  requestedAt: string
}

export function EmailSignInForm() {
  const router = useRouter()
  const { status, recovery, signInWithEmail } = useAuth()
  const [email, setEmail] = useState("")
  const [sent, setSent] = useState<CodeRequest | null>(null)
  const [code, setCode] = useState("")
  const [pending, setPending] = useState<"send" | "verify" | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [signedIn, setSignedIn] = useState(false)
  const codeInput = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (status === "authenticated" && !recovery && !signedIn) {
      router.replace(DEFAULT_SIGNED_IN_PATH)
    }
  }, [status, recovery, signedIn, router])

  async function sendCode(address: string) {
    setPending("send")
    setError(null)
    try {
      const res = await requestEmailCode(address)
      setSent({
        email: address,
        message: res.message,
        expiresInSeconds: res.expires_in_seconds,
        requestedAt: new Date().toISOString(),
      })
      setCode("")
      requestAnimationFrame(() => codeInput.current?.focus())
    } catch (err) {
      setError(describeApiError(err))
    } finally {
      setPending(null)
    }
  }

  async function handleVerify(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!sent || code.length !== CODE_LENGTH) return
    setPending("verify")
    setError(null)
    try {
      await signInWithEmail(sent.email, code)
      setSignedIn(true)
      router.replace(RECOVERY_LANDING_PATH)
    } catch (err) {
      setError(describeApiError(err))
      setPending(null)
    }
  }

  const busy = pending !== null || signedIn

  return (
    <div className="flex flex-col gap-4 rounded-2xl bg-card p-4 sm:p-6">
      {sent ? (
        <form onSubmit={handleVerify} className="flex flex-col gap-4">
          <div
            role="status"
            className="flex items-start gap-2 rounded-xl bg-primary/10 p-3 text-sm"
          >
            <MailCheck
              aria-hidden
              className="mt-0.5 size-4 shrink-0 text-primary"
            />
            <span>
              {sent.message}{" "}
              <span className="text-muted-foreground">
                Requested at {formatTime(sent.requestedAt)} for {sent.email}.
              </span>
            </span>
          </div>
          <Field>
            <FieldLabel htmlFor="recovery-code">
              {CODE_LENGTH}-digit code
            </FieldLabel>
            <OneTimeCodeInput
              ref={codeInput}
              id="recovery-code"
              value={code}
              onValueChange={setCode}
              disabled={busy}
            />
            <FieldDescription>
              The code is valid for {Math.round(sent.expiresInSeconds / 60)}{" "}
              minutes. Check your spam folder if it does not arrive.
            </FieldDescription>
          </Field>
          <Button type="submit" disabled={busy || code.length !== CODE_LENGTH}>
            {(pending === "verify" || signedIn) && (
              <LoaderCircle aria-hidden className="animate-spin" />
            )}
            {signedIn ? "Signed in. Opening settings…" : "Sign in"}
          </Button>
          <div className="flex flex-wrap gap-2">
            <Button
              type="button"
              variant="ghost"
              size="sm"
              disabled={busy}
              onClick={() => void sendCode(sent.email)}
            >
              {pending === "send" && (
                <LoaderCircle aria-hidden className="animate-spin" />
              )}
              Send a new code
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              disabled={busy}
              onClick={() => {
                setSent(null)
                setCode("")
                setError(null)
              }}
            >
              Use another email
            </Button>
          </div>
        </form>
      ) : (
        <form
          onSubmit={(event) => {
            event.preventDefault()
            void sendCode(email.trim())
          }}
          className="flex flex-col gap-4"
        >
          <Field>
            <FieldLabel htmlFor="recovery-email">Email</FieldLabel>
            <Input
              id="recovery-email"
              type="email"
              autoComplete="email"
              required
              value={email}
              onChange={(event) => setEmail(event.target.value)}
              disabled={busy}
              placeholder="name@example.com"
            />
          </Field>
          <Button type="submit" disabled={busy || !email.trim()}>
            {pending === "send" && (
              <LoaderCircle aria-hidden className="animate-spin" />
            )}
            Send code
          </Button>
        </form>
      )}

      <div aria-live="polite">
        {error && (
          <p className="flex items-start gap-2 rounded-xl bg-destructive/10 p-3 text-sm text-destructive">
            <CircleAlert aria-hidden className="mt-0.5 size-4 shrink-0" />
            {error}
          </p>
        )}
      </div>
    </div>
  )
}
