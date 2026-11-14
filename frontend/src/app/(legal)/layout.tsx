import Link from "next/link"
import { VecrowLogoWordmark } from "@/assets/vecrow-logo-wordmark"

export default function LegalLayout({ children }: React.PropsWithChildren) {
  return (
    <div className="mx-auto flex min-h-svh w-full max-w-2xl flex-col gap-8 px-4 py-8">
      <Link href="/" aria-label="Vecrow home" className="self-start">
        <VecrowLogoWordmark className="w-28" />
      </Link>
      <main className="flex flex-col gap-6 text-sm leading-relaxed text-muted-foreground [&_h1]:text-2xl [&_h1]:font-medium [&_h1]:text-foreground [&_h2]:text-base [&_h2]:font-semibold [&_h2]:text-foreground [&_section]:flex [&_section]:flex-col [&_section]:gap-2 [&_ul]:list-disc [&_ul]:pl-5">
        {children}
      </main>
      <p className="text-sm text-muted-foreground">
        <Link href="/sign-in" className="font-medium underline">
          Back to sign in
        </Link>
      </p>
    </div>
  )
}
