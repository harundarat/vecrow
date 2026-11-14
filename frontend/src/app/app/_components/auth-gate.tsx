"use client"

import { usePathname, useRouter } from "next/navigation"
import { useEffect } from "react"
import { CircleAlert } from "lucide-react"
import { EmptyState } from "@/components/empty-state"
import { LoadingState } from "@/components/loading-state"
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

  return (
    <main className="mx-auto flex min-h-svh w-full max-w-md flex-col justify-center p-4">
      {status === "error" ? (
        <EmptyState
          tone="error"
          icon={<CircleAlert aria-hidden className="size-6" />}
          title="Could not load your account"
          description={error}
          action={
            <Button variant="outline" size="sm" onClick={reload}>
              Try again
            </Button>
          }
        />
      ) : (
        <LoadingState label="Loading your account" />
      )}
    </main>
  )
}
