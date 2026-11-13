import type { Metadata } from "next"
import Link from "next/link"
import { Suspense } from "react"
import { VecrowLogoWordmark } from "@/assets/vecrow-logo-wordmark"
import { Separator } from "@/components/ui/separator"
import { SignInPanel } from "./_components/sign-in-panel"

export const metadata: Metadata = {
  title: "Sign in - Vecrow",
}

export default function Page() {
  return (
    <main className="mx-auto flex min-h-svh max-w-xl flex-col justify-center gap-6 p-4 py-6">
      <Link href="/" aria-label="Vecrow home" className="self-start">
        <VecrowLogoWordmark className="w-32" />
      </Link>
      <div>
        <h1 className="text-2xl font-medium">Sign in or create an account</h1>
        <p className="mt-1 text-sm font-medium text-muted-foreground">
          Use any Web3 wallet you own. Your first sign-in creates your account.
        </p>
      </div>
      <Suspense fallback={<WalletGridFallback />}>
        <SignInPanel />
      </Suspense>
      <p className="text-sm text-muted-foreground">
        <Link
          href="/sign-in/email"
          className="font-medium text-foreground underline"
        >
          Lost your wallet?
        </Link>{" "}
        Sign in with the verified email on your account and link a new wallet.
      </p>
      <Separator className="opacity-50" />
      <p className="text-sm text-muted-foreground">
        By signing in or creating an account, you agree to the{" "}
        <Link href="/terms" className="font-medium underline">
          Terms of Service
        </Link>{" "}
        and acknowledge the{" "}
        <Link href="/privacy" className="font-medium underline">
          Privacy Policy
        </Link>
        .
      </p>
    </main>
  )
}

function WalletGridFallback() {
  return (
    <div className="grid grid-cols-2 gap-2" aria-hidden>
      {Array.from({ length: 7 }, (_, i) => (
        <div key={i} className="h-28 animate-pulse rounded-2xl bg-card" />
      ))}
    </div>
  )
}
