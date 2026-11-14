"use client"

import Link from "next/link"
import { useState } from "react"
import { Lock } from "lucide-react"
import { buttonVariants } from "@/components/ui/button"
import { useAuth } from "@/context/auth"
import type { CreatedEscrow } from "@/lib/api/escrow"
import { EmptyState } from "@/components/empty-state"
import { PageHeader } from "@/components/page-header"
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
          <PageHeader
            title="New escrow"
            description="Set the terms. You get a link to send to the other party."
            back={{ href: "/app", label: "Escrows" }}
          />
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
