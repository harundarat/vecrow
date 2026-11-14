"use client"

import { PageHeader } from "@/components/page-header"
import { useAuth } from "@/context/auth"
import { EmailSection } from "./email-section"
import { UsernameSection } from "./username-section"
import { WalletsSection } from "./wallets-section"

export function AccountSettings() {
  const { account } = useAuth()
  if (!account) return null

  return (
    <main className="mx-auto flex w-full max-w-3xl flex-col gap-6 px-4 py-8">
      <PageHeader
        title="Settings"
        description="Your username, email and the wallets you sign in with."
      />
      <UsernameSection account={account} />
      <EmailSection account={account} />
      <WalletsSection account={account} />
    </main>
  )
}
