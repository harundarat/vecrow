"use client"

import { useRouter, useSearchParams } from "next/navigation"
import { useEffect, useState } from "react"
import { Notice } from "@/components/feedback-message"
import { Button } from "@/components/ui/button"
import { useAuth } from "@/context/auth"
import {
  useWalletSigner,
  type WalletChain,
  type WalletSigner,
} from "@/hooks/use-wallet-signer"
import { shortenAddress } from "@/lib/format"
import { safeNextPath } from "@/lib/redirect"
import { describeWalletError } from "@/lib/wallet-errors"
import { WalletCard } from "./wallet-card"

const WALLETS: Array<{ chain: WalletChain; label: string; icons: string[] }> = [
  {
    chain: "near",
    label: "NEAR Wallets",
    icons: ["/crypto-icons/near-protocol.png"],
  },
  {
    chain: "solana",
    label: "Solana Wallets",
    icons: ["/crypto-icons/solana.png"],
  },
  {
    chain: "evm",
    label: "EVM Wallets",
    icons: [
      "/crypto-icons/polygon.png",
      "/crypto-icons/binance.png",
      "/crypto-icons/ethereum.png",
    ],
  },
]

const COMING_SOON = [
  { label: "Bitcoin Wallets", icons: ["/crypto-icons/bitcoin.png"] },
  { label: "Stellar Wallets", icons: ["/crypto-icons/stellar.png"] },
  { label: "Ton Wallets", icons: ["/crypto-icons/ton.png"] },
  { label: "Tron Wallets", icons: ["/crypto-icons/tron.png"] },
]

const CHAIN_NAME: Record<WalletChain, string> = {
  near: "NEAR",
  solana: "Solana",
  evm: "EVM",
}

type Step = "connecting" | "signing" | "redirecting"

export function SignInPanel() {
  const router = useRouter()
  const next = safeNextPath(useSearchParams().get("next"))
  const { status, recovery, signIn } = useAuth()
  const signers: Record<WalletChain, WalletSigner> = {
    near: useWalletSigner("near"),
    solana: useWalletSigner("solana"),
    evm: useWalletSigner("evm"),
  }
  const [active, setActive] = useState<{
    chain: WalletChain
    step: Step
  } | null>(null)
  const [failure, setFailure] = useState<{
    chain: WalletChain
    message: string
  } | null>(null)

  // A recovery session stays here so the visitor can sign in with a wallet.
  const signedIn = status === "authenticated" && !recovery

  useEffect(() => {
    if (signedIn) router.replace(next)
  }, [signedIn, next, router])

  async function handleSelect(chain: WalletChain) {
    if (active) return
    const signer = signers[chain]
    setFailure(null)
    setActive({ chain, step: "connecting" })
    try {
      const address = await signer.connect()
      setActive({ chain, step: "signing" })
      await signIn({ scheme: signer.scheme, address, sign: signer.sign })
      setActive({ chain, step: "redirecting" })
    } catch (err) {
      setActive(null)
      setFailure({ chain, message: describeWalletError(err) })
    }
  }

  async function handleDisconnect(chain: WalletChain) {
    setFailure(null)
    try {
      await signers[chain].disconnect()
    } catch (err) {
      setFailure({ chain, message: describeWalletError(err) })
    }
  }

  const failedSigner = failure ? signers[failure.chain] : null

  return (
    <div className="flex flex-col gap-4">
      {status === "authenticated" && recovery && (
        <p className="rounded-2xl bg-primary/10 p-4 text-sm">
          You are in a recovery session. Sign in with a wallet linked to your
          account to create and approve escrows again.
        </p>
      )}
      <div className="grid grid-cols-2 gap-2">
        {WALLETS.map(({ chain, label, icons }) => {
          const address = signers[chain].address
          return (
            <WalletCard
              key={chain}
              label={label}
              iconUrls={icons}
              detail={address ? shortenAddress(address) : undefined}
              busy={active?.chain === chain}
              disabled={active !== null && active.chain !== chain}
              onClick={() => void handleSelect(chain)}
            />
          )
        })}
        {COMING_SOON.map(({ label, icons }) => (
          <WalletCard
            key={label}
            label={label}
            iconUrls={icons}
            disabled
            disableLabel="Soon"
          />
        ))}
      </div>

      <p aria-live="polite" className="min-h-5 text-sm text-muted-foreground">
        {active && stepMessage(active.chain, active.step)}
      </p>

      {failure && (
        <Notice
          role="alert"
          action={
            failedSigner?.address && (
              <Button
                variant="outline"
                size="sm"
                onClick={() => void handleDisconnect(failure.chain)}
              >
                Use another wallet
              </Button>
            )
          }
        >
          <span className="font-semibold">
            Could not sign in with {CHAIN_NAME[failure.chain]}.
          </span>{" "}
          {failure.message}
        </Notice>
      )}
    </div>
  )
}

function stepMessage(chain: WalletChain, step: Step): string {
  switch (step) {
    case "connecting":
      return `Connect your ${CHAIN_NAME[chain]} wallet…`
    case "signing":
      return "Sign the message in your wallet. Signing is free and sends no transaction."
    case "redirecting":
      return "Signed in. Opening your dashboard…"
  }
}
