"use client"

import Image from "next/image"
import { Button } from "@/components/ui/button"
import type { WalletChain } from "@/hooks/use-wallet-signer"
import { shortenAddress } from "@/lib/format"

export const CHAIN_LABEL: Record<WalletChain, string> = {
  near: "NEAR",
  evm: "EVM",
  solana: "Solana",
}

const CHAIN_ICON: Record<WalletChain, string> = {
  near: "/crypto-icons/near-protocol.png",
  evm: "/crypto-icons/ethereum.png",
  solana: "/crypto-icons/solana.png",
}

function ChainIcon({ chain }: { chain: WalletChain }) {
  return (
    <Image
      src={CHAIN_ICON[chain]}
      alt=""
      width={32}
      height={32}
      className="size-5 rounded-full"
    />
  )
}

/** One button per chain the visitor can connect. */
export function WalletChoice({
  chains,
  busy,
  onConnect,
}: {
  chains: WalletChain[]
  busy: boolean
  onConnect: (chain: WalletChain) => void
}) {
  return (
    <div className="grid grid-cols-1 gap-2 sm:grid-cols-3">
      {chains.map((chain) => (
        <Button
          key={chain}
          type="button"
          variant="outline"
          disabled={busy}
          onClick={() => onConnect(chain)}
        >
          <ChainIcon chain={chain} />
          {chains.length === 1
            ? `Connect ${CHAIN_LABEL[chain]} wallet`
            : CHAIN_LABEL[chain]}
        </Button>
      ))}
    </div>
  )
}

/** The connected wallet, with a way to pick another one. */
export function ConnectedWallet({
  chain,
  address,
  busy,
  onChange,
}: {
  chain: WalletChain
  address: string
  busy: boolean
  onChange: () => void
}) {
  return (
    <div className="flex items-center justify-between gap-3 rounded-xl border p-3">
      <span className="flex min-w-0 items-center gap-2 text-sm">
        <ChainIcon chain={chain} />
        <span className="text-muted-foreground">{CHAIN_LABEL[chain]}</span>
        <span className="truncate font-mono" title={address}>
          {shortenAddress(address)}
        </span>
      </span>
      <Button
        type="button"
        variant="ghost"
        size="sm"
        disabled={busy}
        onClick={onChange}
      >
        Change
      </Button>
    </div>
  )
}
