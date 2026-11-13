import type { Metadata } from "next"
import Link from "next/link"
import { ArrowLeft } from "lucide-react"
import { VecrowLogoWordmark } from "@/assets/vecrow-logo-wordmark"
import { EmailSignInForm } from "./_components/email-sign-in-form"

export const metadata: Metadata = {
  title: "Sign in with email - Vecrow",
}

export default function Page() {
  return (
    <main className="mx-auto flex min-h-svh max-w-md flex-col justify-center gap-6 p-4 py-6">
      <Link href="/" aria-label="Vecrow home" className="self-start">
        <VecrowLogoWordmark className="w-32" />
      </Link>
      <div>
        <h1 className="text-2xl font-medium">Recover your account</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Lost access to your wallet? Sign in with a code sent to the verified
          email on your account. This opens a short recovery session where you
          can link a new wallet and unlink old ones. It does not move funds or
          escrow rights from a lost wallet.
        </p>
      </div>
      <EmailSignInForm />
      <Link
        href="/sign-in"
        className="flex items-center gap-1 self-start text-sm font-medium text-muted-foreground hover:text-foreground"
      >
        <ArrowLeft aria-hidden className="size-4" />
        Sign in with a wallet
      </Link>
    </main>
  )
}
