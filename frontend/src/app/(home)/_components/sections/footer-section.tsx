import Image from "next/image"
import Link from "next/link"
import { ArrowUpRight, CodeXml, X } from "lucide-react"
import { Button } from "@/components/ui/button"
import { VecrowLogoWordmark } from "@/assets/vecrow-logo-wordmark"

const repository = "https://github.com/harundarat/vecrow"

export function FooterSection() {
  return (
    <footer className="mx-auto w-[calc(100%-3rem)] pt-12 sm:w-[calc(100%-5rem)] sm:pt-16">
      <div className="flex items-center justify-between gap-6 border-b border-border/50 pb-8 sm:pb-10">
        <Link href="/" aria-label="Vecrow home">
          <VecrowLogoWordmark className="h-8 w-36 sm:w-44" />
        </Link>
        <div className="flex gap-3">
          <Button
            variant="primary-light"
            size="icon-lg"
            nativeButton={false}
            render={
              <a href={repository} target="_blank" rel="noreferrer" aria-label="Vecrow on GitHub" />
            }
          >
            <CodeXml />
          </Button>
          <Button
            variant="primary-light"
            size="icon-lg"
            nativeButton={false}
            render={
              <a href="https://x.com/vecrow" target="_blank" rel="noreferrer" aria-label="Vecrow on X" />
            }
          >
            <X />
          </Button>
        </div>
      </div>

      <div className="flex flex-col justify-between gap-8 py-8 sm:flex-row sm:items-center sm:py-10">
        <div className="flex flex-col items-start gap-4">
          <p className="text-primary">Provide us with feedback</p>
          <Button
            variant="primary-light"
            size="lg"
            nativeButton={false}
            render={
              <a href={`${repository}/issues/new`} target="_blank" rel="noreferrer" />
            }
          >
            <CodeXml data-icon="inline-start" />
            Open an issue on GitHub
            <ArrowUpRight data-icon="inline-end" />
          </Button>
        </div>
        <nav aria-label="Footer" className="flex flex-wrap gap-x-7 gap-y-3 text-primary">
          <a href={`${repository}/tree/main/docs`} className="hover:text-foreground">
            Docs <ArrowUpRight className="inline size-4" />
          </a>
          <a href={`${repository}/tree/main/contracts`} className="hover:text-foreground">
            Contract <ArrowUpRight className="inline size-4" />
          </a>
          <a href="https://near-faucet.io/" className="hover:text-foreground">
            Testnet Faucet <ArrowUpRight className="inline size-4" />
          </a>
        </nav>
      </div>

      <div className="relative flex min-h-44 items-end overflow-hidden pb-4 sm:min-h-64 sm:pb-6">
        <Image
          src="/vecrow-wordmark-escrow.svg"
          alt=""
          aria-hidden="true"
          width={1312}
          height={242}
          className="pointer-events-none absolute inset-x-0 top-0 h-auto w-full"
        />
        <div className="relative flex w-full flex-col justify-between gap-2 text-sm text-muted-foreground/60 sm:flex-row">
          <p>2026 <a className="underline underline-offset-2" href="https://near.org/">Near Hackathon</a> Project</p>
          <p>Currently, this project only runs on the testnet.</p>
        </div>
      </div>
    </footer>
  )
}
