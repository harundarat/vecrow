import type { Metadata } from "next"
import Link from "next/link"
import { VecrowLogoWordmark } from "@/assets/vecrow-logo-wordmark"
import { BackLink, PageHeader } from "@/components/page-header"
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
      <PageHeader
        title="Recover your account"
        description="Lost your wallet? Sign in with a code sent to your verified email. You can then link a new wallet and unlink old ones. Funds and escrow rights stay with the lost wallet."
      />
      <EmailSignInForm />
      <BackLink href="/sign-in">Sign in with a wallet</BackLink>
    </main>
  )
}
