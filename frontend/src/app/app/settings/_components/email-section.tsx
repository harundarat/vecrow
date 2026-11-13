"use client"

import { useState } from "react"
import { LoaderCircle } from "lucide-react"
import { CODE_LENGTH, OneTimeCodeInput } from "@/components/one-time-code-input"
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { useAuth } from "@/context/auth"
import {
  updateAccount,
  verifyAccountEmail,
  type AccountUpdate,
} from "@/lib/api/account"
import type { Account } from "@/lib/api/auth"
import { describeApiError } from "@/lib/api/errors"
import {
  FeedbackMessage,
  SettingsSection,
  type Feedback,
} from "./settings-section"

type Action = "send" | "verify" | "resend" | "cancel" | "remove"

export function EmailSection({ account }: { account: Account }) {
  const { recovery, request, setAccount } = useAuth()
  const [editing, setEditing] = useState(false)
  const [newEmail, setNewEmail] = useState("")
  const [code, setCode] = useState("")
  const [pending, setPending] = useState<Action | null>(null)
  const [confirmRemove, setConfirmRemove] = useState(false)
  const [feedback, setFeedback] = useState<Feedback>(null)

  async function run(
    action: Action,
    call: () => Promise<Account>,
    success: (updated: Account) => string
  ) {
    setPending(action)
    setFeedback(null)
    try {
      const updated = await call()
      setAccount(updated)
      setFeedback({ kind: "success", message: success(updated) })
      return true
    } catch (err) {
      setFeedback({ kind: "error", message: describeApiError(err) })
      return false
    } finally {
      setPending(null)
    }
  }

  const update = (body: AccountUpdate) => () => updateAccount(request, body)

  async function handleSend(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const ok = await run("send", update({ email: newEmail }), (updated) =>
      updated.pending_email
        ? `We emailed a ${CODE_LENGTH}-digit code to ${updated.pending_email}. Enter it below to verify the address.`
        : "That is already your email."
    )
    if (ok) {
      setEditing(false)
      setNewEmail("")
      setCode("")
    }
  }

  async function handleVerify(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const ok = await run(
      "verify",
      () => verifyAccountEmail(request, code),
      (updated) => `${updated.email} is now your verified email.`
    )
    if (ok) setCode("")
  }

  async function handleRemove() {
    const ok = await run(
      "remove",
      update({ email: "" }),
      () => "Your email was removed."
    )
    if (ok) setConfirmRemove(false)
  }

  const busy = pending !== null

  return (
    <SettingsSection
      id="email"
      title="Email"
      description="Optional. A verified email lets others invite you by email and lets you sign in with a code if you lose your wallets."
    >
      <dl className="flex flex-col divide-y rounded-xl border">
        <EmailRow
          term="Verified email"
          email={account.email}
          badge={account.email && <Badge>Verified</Badge>}
        />
        {account.pending_email && (
          <EmailRow
            term="Email waiting for its code"
            email={account.pending_email}
            badge={<Badge variant="outline">Waiting for code</Badge>}
          />
        )}
      </dl>

      {recovery ? (
        <p className="text-sm text-muted-foreground">
          Sign in with a wallet to add, change or remove your email.
        </p>
      ) : (
        <>
          {account.pending_email && (
            <form onSubmit={handleVerify} className="flex flex-col gap-3">
              <Field>
                <FieldLabel htmlFor="email-code">
                  Code sent to {account.pending_email}
                </FieldLabel>
                <div className="flex flex-col gap-2 sm:flex-row">
                  <OneTimeCodeInput
                    id="email-code"
                    value={code}
                    onValueChange={setCode}
                    disabled={busy}
                    className="sm:max-w-48"
                  />
                  <Button
                    type="submit"
                    disabled={busy || code.length !== CODE_LENGTH}
                  >
                    {pending === "verify" && (
                      <LoaderCircle aria-hidden className="animate-spin" />
                    )}
                    Verify email
                  </Button>
                </div>
                <FieldDescription>
                  The code is valid for 10 minutes.
                </FieldDescription>
              </Field>
              <div className="flex flex-wrap gap-2">
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  disabled={busy}
                  onClick={() =>
                    void run(
                      "resend",
                      update({ email: account.pending_email ?? "" }),
                      (updated) =>
                        `We emailed a new code to ${updated.pending_email}.`
                    )
                  }
                >
                  {pending === "resend" && (
                    <LoaderCircle aria-hidden className="animate-spin" />
                  )}
                  Send a new code
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  disabled={busy}
                  onClick={() =>
                    void run(
                      "cancel",
                      update({ email: account.email ?? "" }),
                      () => "The email change was cancelled."
                    )
                  }
                >
                  {pending === "cancel" && (
                    <LoaderCircle aria-hidden className="animate-spin" />
                  )}
                  Cancel change
                </Button>
              </div>
            </form>
          )}

          {editing ? (
            <form onSubmit={handleSend} className="flex flex-col gap-3">
              <Field>
                <FieldLabel htmlFor="new-email">
                  {account.email ? "New email" : "Email"}
                </FieldLabel>
                <Input
                  id="new-email"
                  type="email"
                  autoComplete="email"
                  required
                  value={newEmail}
                  onChange={(event) => setNewEmail(event.target.value)}
                  disabled={busy}
                  placeholder="name@example.com"
                />
                <FieldDescription>
                  We will email a {CODE_LENGTH}-digit code to confirm it is
                  yours.
                </FieldDescription>
              </Field>
              <div className="flex flex-wrap gap-2">
                <Button type="submit" disabled={busy || !newEmail.trim()}>
                  {pending === "send" && (
                    <LoaderCircle aria-hidden className="animate-spin" />
                  )}
                  Send code
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  disabled={busy}
                  onClick={() => {
                    setEditing(false)
                    setNewEmail("")
                  }}
                >
                  Cancel
                </Button>
              </div>
            </form>
          ) : (
            <div className="flex flex-wrap gap-2">
              <Button
                variant="outline"
                disabled={busy}
                onClick={() => {
                  setEditing(true)
                  setFeedback(null)
                }}
              >
                {account.email ? "Change email" : "Add email"}
              </Button>
              {account.email && (
                <Button
                  variant="outline"
                  disabled={busy}
                  onClick={() => setConfirmRemove(true)}
                >
                  Remove email
                </Button>
              )}
            </div>
          )}
        </>
      )}

      <FeedbackMessage feedback={feedback} />

      <AlertDialog
        open={confirmRemove}
        onOpenChange={(open) => {
          if (!busy) setConfirmRemove(open)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Remove your email?</AlertDialogTitle>
            <AlertDialogDescription>
              Invitations sent to it will no longer reach you, and you will not
              be able to sign in with an email code if you lose your wallets.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={busy}>Keep email</AlertDialogCancel>
            <Button
              variant="destructive"
              disabled={busy}
              onClick={() => void handleRemove()}
            >
              {pending === "remove" && (
                <LoaderCircle aria-hidden className="animate-spin" />
              )}
              Remove email
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </SettingsSection>
  )
}

function EmailRow({
  term,
  email,
  badge,
}: {
  term: string
  email: string | null
  badge?: React.ReactNode
}) {
  return (
    <div className="p-3">
      <dt className="sr-only">{term}</dt>
      <dd className="flex flex-wrap items-center justify-between gap-2 text-sm">
        {email ? (
          <span className="min-w-0 truncate">{email}</span>
        ) : (
          <span className="text-muted-foreground">No verified email</span>
        )}
        {badge}
      </dd>
    </div>
  )
}
