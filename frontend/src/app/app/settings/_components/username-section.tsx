"use client"

import { useState } from "react"
import { LoaderCircle } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { useAuth } from "@/context/auth"
import { updateAccount } from "@/lib/api/account"
import type { Account } from "@/lib/api/auth"
import { describeApiError } from "@/lib/api/errors"
import {
  FeedbackMessage,
  SettingsSection,
  type Feedback,
} from "./settings-section"

export function UsernameSection({ account }: { account: Account }) {
  const { recovery, request, setAccount } = useAuth()
  const [username, setUsername] = useState(account.username)
  const [saving, setSaving] = useState(false)
  const [feedback, setFeedback] = useState<Feedback>(null)

  const unchanged = username.trim().toLowerCase() === account.username

  async function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (unchanged || saving) return
    setSaving(true)
    setFeedback(null)
    try {
      const updated = await updateAccount(request, { username })
      setAccount(updated)
      setUsername(updated.username)
      setFeedback({
        kind: "success",
        message: `Your username is now ${updated.username}.`,
      })
    } catch (err) {
      setFeedback({ kind: "error", message: describeApiError(err) })
    } finally {
      setSaving(false)
    }
  }

  return (
    <SettingsSection
      id="username"
      title="Username"
      description="Others can invite you to an escrow by this name."
    >
      <form onSubmit={handleSubmit} className="flex flex-col gap-4">
        <Field data-disabled={recovery || undefined}>
          <FieldLabel htmlFor="username-input">Username</FieldLabel>
          <div className="flex flex-col gap-2 sm:flex-row">
            <Input
              id="username-input"
              name="username"
              value={username}
              onChange={(event) => setUsername(event.target.value)}
              disabled={recovery || saving}
              autoComplete="off"
              autoCapitalize="none"
              spellCheck={false}
              maxLength={30}
              aria-describedby="username-hint"
            />
            <Button
              type="submit"
              disabled={recovery || unchanged || saving || !username.trim()}
              className="sm:w-28"
            >
              {saving && <LoaderCircle aria-hidden className="animate-spin" />}
              Save
            </Button>
          </div>
          <FieldDescription id="username-hint">
            {recovery
              ? "Sign in with a wallet to change your username."
              : "3–30 characters: lowercase letters, digits or _, starting with a letter."}
          </FieldDescription>
        </Field>
        <FeedbackMessage feedback={feedback} />
      </form>
    </SettingsSection>
  )
}
