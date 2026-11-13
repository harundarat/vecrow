"use client"

import { useAuth } from "@/context/auth"
import { EmailSection } from "./email-section"
import { UsernameSection } from "./username-section"
import { WalletsSection } from "./wallets-section"

export function AccountSettings() {
  const { account } = useAuth()
  if (!account) return null

  return (
    <main className="mx-auto flex w-full max-w-3xl flex-col gap-6 px-4 py-8">
      <div>
        <h1 className="text-2xl font-medium">Settings</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Your username, email and the wallets you sign in with.
        </p>
      </div>
      <UsernameSection account={account} />
      <EmailSection account={account} />
      <WalletsSection account={account} />
    </main>
  )
}
