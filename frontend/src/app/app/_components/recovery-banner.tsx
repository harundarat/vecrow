"use client"

import Link from "next/link"
import { usePathname } from "next/navigation"
import { ShieldAlert } from "lucide-react"
import { buttonVariants } from "@/components/ui/button"
import { useAuth } from "@/context/auth"
import { formatTime } from "@/lib/format"

/** Explains what a session opened with an email code cannot do. */
export function RecoveryBanner() {
  const { recovery, expiresAt } = useAuth()
  const pathname = usePathname()
  if (!recovery) return null

  const onSettings = pathname.startsWith("/app/settings")

  return (
    <div className="border-b bg-primary/10">
      <div
        role="status"
        className="mx-auto flex w-full max-w-5xl flex-col gap-3 px-4 py-3 sm:flex-row sm:items-center sm:justify-between"
      >
        <p className="flex items-start gap-2 text-sm">
          <ShieldAlert
            aria-hidden
            className="mt-0.5 size-4 shrink-0 text-primary"
          />
          <span>
            <span className="font-semibold">Recovery session.</span> You signed
            in with an email code
            {expiresAt ? `, valid until ${formatTime(expiresAt)}` : ""}. You can
            view your account and link or unlink wallets. To create or approve
            escrows or change your username or email, link a wallet you control
            and sign in with it.
          </span>
        </p>
        <div className="flex shrink-0 gap-2">
          {!onSettings && (
            <Link
              href="/app/settings#wallets"
              className={buttonVariants({ size: "sm" })}
            >
              Link a wallet
            </Link>
          )}
          <Link
            href="/sign-in"
            className={buttonVariants({ variant: "outline", size: "sm" })}
          >
            Sign in with a wallet
          </Link>
        </div>
      </div>
    </div>
  )
}
