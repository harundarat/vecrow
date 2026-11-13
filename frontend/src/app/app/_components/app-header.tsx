"use client"

import Link from "next/link"
import { useRouter } from "next/navigation"
import { useState } from "react"
import { LoaderCircle, LogOut } from "lucide-react"
import { VecrowLogoWordmark } from "@/assets/vecrow-logo-wordmark"
import { Button } from "@/components/ui/button"
import { useAuth } from "@/context/auth"

export function AppHeader() {
  const { account, signOut } = useAuth()
  const router = useRouter()
  const [signingOut, setSigningOut] = useState(false)

  async function handleSignOut() {
    setSigningOut(true)
    await signOut()
    router.replace("/sign-in")
  }

  return (
    <header className="sticky top-0 z-10 border-b bg-background/80 backdrop-blur">
      <div className="mx-auto flex h-16 w-full max-w-5xl items-center justify-between gap-4 px-4">
        <Link href="/app" aria-label="Vecrow dashboard" className="shrink-0">
          <VecrowLogoWordmark className="w-28" />
        </Link>
        <div className="flex min-w-0 items-center gap-2">
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
