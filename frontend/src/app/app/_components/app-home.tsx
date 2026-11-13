"use client"

import Link from "next/link"
import { FileText, Lock } from "lucide-react"
import { buttonVariants } from "@/components/ui/button"
import { useAuth } from "@/context/auth"

export function AppHome() {
  const { account, recovery } = useAuth()
  if (!account) return null

  return (
    <main className="mx-auto flex w-full max-w-5xl flex-col gap-8 px-4 py-8">
      <div>
        <h1 className="text-2xl font-medium">Escrows</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Welcome, {account.username}. Your escrows and the ones you are invited
          to will show up here.
        </p>
      </div>

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
        <EmptyState
          icon={<FileText aria-hidden className="size-6" />}
          title="No escrows yet"
          description="Creating and joining escrows is coming soon."
        />
      )}
    </main>
  )
}

function EmptyState({
  icon,
  title,
  description,
  action,
}: {
  icon: React.ReactNode
  title: string
  description: string
  action?: React.ReactNode
}) {
  return (
    <div className="flex flex-col items-center gap-3 rounded-2xl border border-dashed bg-card/40 px-4 py-12 text-center">
      <span className="flex size-12 items-center justify-center rounded-full bg-primary/15 text-primary">
        {icon}
      </span>
      <div>
        <p className="font-semibold">{title}</p>
        <p className="mt-1 max-w-sm text-sm text-muted-foreground">
          {description}
        </p>
      </div>
      {action}
    </div>
  )
}
