"use client"

import { VecrowLogoWordmark } from "@/assets/vecrow-logo-wordmark"
import { Separator } from "@/components/ui/separator"
import { useAppKit } from "@reown/appkit/react"
import Image from "next/image"
import Link from "next/link"

export default function Page() {
  const { open } = useAppKit()

  return (
    <main className="mx-auto mt-auto flex min-h-svh max-w-xl flex-col justify-center gap-6 p-4 py-6">
      <VecrowLogoWordmark className="w-32" />
      <div>
        <h4 className="text-2xl font-medium">Sign in or create an account</h4>
        <p className="mt-1 text-sm font-medium text-muted-foreground">
          Use any Web3 wallet you own.
        </p>
      </div>
      <div className="grid grid-cols-2 gap-2">
        <WalletCard
          label="Near Wallets"
          iconUrls={["/crypto-icons/near-protocol.png"]}
        />
        <WalletCard
          onClick={() => open({ namespace: "solana" })}
          label="Solana Wallets"
          iconUrls={["/crypto-icons/solana.png"]}
        />
        <WalletCard
          onClick={() => open({ namespace: "eip155" })}
          label="EVM Wallets"
          iconUrls={[
            "/crypto-icons/polygon.png",
            "/crypto-icons/binance.png",
            "/crypto-icons/ethereum.png",
          ]}
        />
        <WalletCard
          onClick={() => open({ namespace: "bip122" })}
          label="Bitcoin Wallets"
          iconUrls={["/crypto-icons/bitcoin.png"]}
        />
        <WalletCard
          disabled
          disableLabel="Soon"
          label="Stellar Wallets"
          iconUrls={["/crypto-icons/stellar.png"]}
        />
        <WalletCard
          disabled
          disableLabel="Soon"
          label="Ton Wallets"
          iconUrls={["/crypto-icons/ton.png"]}
        />
        <WalletCard
          disabled
          disableLabel="Soon"
          label="Tron Wallets"
          iconUrls={["/crypto-icons/tron.png"]}
        />
      </div>
      <Separator className={"opacity-50"} />
      <p className="text-sm text-muted-foreground">
        By signing in or creating an account, you agree to the{" "}
        <Link href={"/terms-of-services"} className="font-medium underline">
          Terms of Service
        </Link>{" "}
        and acknowledge the{" "}
        <Link href={"/terms-of-services"} className="font-medium underline">
          Privacy Policy
        </Link>
        .
      </p>
    </main>
  )
}

function WalletCard({
  iconUrls,
  label,
  disabled = false,
  disableLabel,
  onClick,
}: {
  iconUrls: string[]
  label: string
  disabled?: boolean
  disableLabel?: string
  onClick?: () => void
}) {
  return (
    <button
      onClick={onClick}
      disabled={disabled}
      className="relative flex cursor-pointer flex-col items-start gap-3 rounded-2xl bg-card p-5 transition-all select-none hover:ring-2 hover:ring-primary/50 active:scale-98 disabled:pointer-events-none"
    >
      {/*Icons*/}
      <div className="flex items-center">
        {iconUrls.map((url, index) => (
          <Image
            width={64}
            height={64}
            key={index}
            src={url}
            alt={label}
            className="size-8 rounded-full border-2 border-card not-first:-ml-2"
          />
        ))}
      </div>
      {/*Label*/}
      <span className="font-semibold">{label}</span>
      {disabled && (
        <div className="absolute inset-0 flex items-center justify-center bg-background/70 font-semibold">
          {disableLabel}
        </div>
      )}
    </button>
  )
}
