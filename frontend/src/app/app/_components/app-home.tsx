"use client"

import Link from "next/link"
import { Lock, Plus } from "lucide-react"
import { buttonVariants } from "@/components/ui/button"
import { useAuth } from "@/context/auth"
import { EmptyState } from "@/components/empty-state"
import { PageHeader } from "@/components/page-header"
import { EscrowList } from "./escrow-list"

export function AppHome() {
  const { account, recovery } = useAuth()
  if (!account) return null

  return (
    <main className="mx-auto flex w-full max-w-5xl flex-col gap-6 px-4 py-8">
      <PageHeader
        title="Escrows"
        description="Escrows you created, joined or were invited to."
        action={
          !recovery && (
            <Link href="/app/escrows/new" className={buttonVariants()}>
              <Plus aria-hidden />
              New escrow
            </Link>
          )
        }
      />

      {recovery ? (
        <EmptyState
          icon={<Lock aria-hidden className="size-6" />}
          title="Escrows need a wallet sign-in"
          description="A recovery session cannot open or create escrows. Link a wallet you control, then sign in with it."
          action={
            <Link
              href="/app/settings#wallets"
              className={buttonVariants({ size: "sm" })}
            >
              Link a wallet
            </Link>
          }
        />
      ) : (
        <EscrowList />
      )}
    </main>
  )
}
