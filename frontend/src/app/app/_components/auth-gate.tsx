"use client"

import { usePathname, useRouter } from "next/navigation"
import { useEffect } from "react"
import { CircleAlert, LoaderCircle } from "lucide-react"
import { Button } from "@/components/ui/button"
import { useAuth } from "@/context/auth"

/** Renders its children only for a signed-in account; otherwise sends the visitor to sign in. */
export function AuthGate({ children }: React.PropsWithChildren) {
  const { status, error, reload } = useAuth()
  const router = useRouter()
  const pathname = usePathname()

  useEffect(() => {
    if (status === "unauthenticated") {
      router.replace(`/sign-in?next=${encodeURIComponent(pathname)}`)
    }
  }, [status, pathname, router])

  if (status === "authenticated") return children

  if (status === "error") {
    return (
      <main className="mx-auto flex min-h-svh max-w-md flex-col items-center justify-center gap-4 p-4 text-center">
        <CircleAlert aria-hidden className="size-8 text-destructive" />
        <div>
          <h1 className="text-lg font-semibold">Could not load your account</h1>
          <p className="mt-1 text-sm text-muted-foreground">{error}</p>
        </div>
        <Button variant="outline" onClick={reload}>
          Try again
        </Button>
      </main>
    )
  }

  return (
    <main
      className="flex min-h-svh items-center justify-center p-4"
      aria-busy="true"
    >
      <LoaderCircle
        aria-label="Loading your account"
        className="size-6 animate-spin text-muted-foreground"
      />
    </main>
  )
}
