"use client"

import Link from "next/link"
import { usePathname, useRouter } from "next/navigation"
import { useState } from "react"
import { LoaderCircle, LogOut } from "lucide-react"
import { VecrowLogoWordmark } from "@/assets/vecrow-logo-wordmark"
import { Button } from "@/components/ui/button"
import { useAuth } from "@/context/auth"
import { cn } from "@/utils"

const NAV = [
  { href: "/app", label: "Escrows" },
  { href: "/app/settings", label: "Settings" },
]

export function AppHeader() {
  const { account, signOut } = useAuth()
  const router = useRouter()
  const pathname = usePathname()
  const [signingOut, setSigningOut] = useState(false)

  async function handleSignOut() {
    setSigningOut(true)
    await signOut()
    router.replace("/sign-in")
  }

  return (
    <header className="sticky top-0 z-10 border-b bg-background/80 backdrop-blur">
      <div className="mx-auto flex w-full max-w-5xl flex-wrap items-center justify-between gap-x-6 px-4">
        <Link
          href="/app"
          aria-label="Vecrow escrows"
          className="flex h-16 shrink-0 items-center"
        >
          <VecrowLogoWordmark className="w-28" />
        </Link>
        <nav
          aria-label="App"
          className="order-last -mx-1 flex w-full gap-1 pb-2 sm:order-none sm:mx-0 sm:w-auto sm:flex-1 sm:pb-0"
        >
          {NAV.map(({ href, label }) => {
            const active =
              href === "/app" ? pathname === href : pathname.startsWith(href)
            return (
              <Link
                key={href}
                href={href}
                aria-current={active ? "page" : undefined}
                className={cn(
                  "rounded-md px-3 py-1.5 text-sm font-medium text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none",
                  active && "bg-muted text-foreground"
                )}
              >
                {label}
              </Link>
            )
          })}
        </nav>
        <div className="flex h-16 min-w-0 items-center gap-2">
          <span
            className="truncate text-sm font-medium text-muted-foreground"
            title={account?.username}
          >
            {account?.username}
          </span>
          <Button
            variant="outline"
            size="sm"
            onClick={() => void handleSignOut()}
            disabled={signingOut}
          >
            {signingOut ? (
              <LoaderCircle aria-hidden className="animate-spin" />
            ) : (
              <LogOut aria-hidden />
            )}
            Log out
          </Button>
        </div>
      </div>
    </header>
  )
}
