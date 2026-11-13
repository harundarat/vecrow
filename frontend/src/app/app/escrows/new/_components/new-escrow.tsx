"use client"

import Link from "next/link"
import { useState } from "react"
import { ArrowLeft, Lock } from "lucide-react"
import { buttonVariants } from "@/components/ui/button"
import { useAuth } from "@/context/auth"
import type { CreatedEscrow } from "@/lib/api/escrow"
import { EmptyState } from "@/app/app/_components/empty-state"
import { EscrowCreated } from "./escrow-created"
import { NewEscrowForm } from "./new-escrow-form"

export function NewEscrow() {
  const { account, recovery } = useAuth()
  const [created, setCreated] = useState<CreatedEscrow | null>(null)
  if (!account) return null

  return (
    <main className="mx-auto flex w-full max-w-3xl flex-col gap-6 px-4 py-8">
      {created ? (
        <EscrowCreated escrow={created} />
      ) : (
        <>
          <div>
            <Link
              href="/app"
              className="inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
            >
              <ArrowLeft aria-hidden className="size-4" />
              Escrows
            </Link>
            <h1 className="mt-2 text-2xl font-medium">New escrow</h1>
            <p className="mt-1 text-sm text-muted-foreground">
              Set the terms. You get a link to send to the other party.
            </p>
          </div>
          {recovery ? (
            <EmptyState
              icon={<Lock aria-hidden className="size-6" />}
              title="Creating an escrow needs a wallet sign-in"
              description="You signed in with an email code. Link a wallet you control, then sign in with it."
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
            <NewEscrowForm account={account} onCreated={setCreated} />
          )}
        </>
      )}
    </main>
  )
}
