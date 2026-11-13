"use client"

import { FileText, Wallet } from "lucide-react"
import { useAuth } from "@/context/auth"
import type { WalletScheme } from "@/lib/api/auth"
import { shortenAddress } from "@/lib/format"

const SCHEME_LABEL: Record<WalletScheme, string> = {
  eip191: "EVM",
  ed25519: "Solana",
  nep413: "NEAR",
}

export function AppHome() {
  const { account } = useAuth()
  if (!account) return null

  return (
    <main className="mx-auto flex w-full max-w-5xl flex-col gap-8 px-4 py-8">
      <div>
        <h1 className="text-2xl font-medium">Welcome, {account.username}</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Your escrows and the ones you are invited to will show up here.
        </p>
      </div>

      <section
        aria-labelledby="escrows-heading"
        className="flex flex-col gap-3"
      >
        <h2 id="escrows-heading" className="text-lg font-medium">
          Escrows
        </h2>
        <div className="flex flex-col items-center gap-3 rounded-2xl border border-dashed bg-card/40 px-4 py-12 text-center">
          <span className="flex size-12 items-center justify-center rounded-full bg-primary/15 text-primary">
            <FileText aria-hidden className="size-6" />
          </span>
          <div>
            <p className="font-semibold">No escrows yet</p>
            <p className="mt-1 max-w-sm text-sm text-muted-foreground">
              Creating and joining escrows is coming soon.
            </p>
          </div>
        </div>
      </section>

      <section
        aria-labelledby="wallets-heading"
        className="flex flex-col gap-3"
      >
        <h2 id="wallets-heading" className="text-lg font-medium">
          Linked wallets
        </h2>
        <ul className="flex flex-col divide-y rounded-2xl bg-card">
          {account.wallets.map((wallet) => (
            <li key={wallet.id} className="flex items-center gap-3 p-4">
              <Wallet aria-hidden className="size-5 shrink-0 text-primary" />
              <span className="min-w-0 flex-1">
                <span
                  className="block truncate font-mono text-sm"
                  title={wallet.address}
                >
                  {shortenAddress(wallet.address)}
                </span>
                <span className="text-xs text-muted-foreground">
                  {SCHEME_LABEL[wallet.scheme] ?? wallet.scheme}
                </span>
              </span>
            </li>
          ))}
        </ul>
      </section>
    </main>
  )
}
