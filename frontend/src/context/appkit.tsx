"use client"

import { createAppKit } from "@reown/appkit/react"
import { SolanaAdapter } from "@reown/appkit-adapter-solana"
import { EthersAdapter } from "@reown/appkit-adapter-ethers"
import { BitcoinAdapter } from "@reown/appkit-adapter-bitcoin"
import { TonAdapter } from "@reown/appkit-adapter-ton"
import {
  mainnet,
  arbitrum,
  sepolia,
  AppKitNetwork,
} from "@reown/appkit/networks"
import { solana, solanaTestnet } from "@reown/appkit/networks"
import { bitcoin, bitcoinTestnet } from "@reown/appkit/networks"
import { ton, tonTestnet } from "@reown/appkit/networks"
import { env } from "@/env"

// 1. Get projectId at https://dashboard.reown.com
const projectId = env.NEXT_PUBLIC_REOWN_PROJECT_ID

// 2. Create a metadata object
const metadata = {
  name: "Vecrow",
  description:
    "Vecrow is a escrow service for crypto transactions, built on NEAR Protocol.",
  url: "http://localhost:3000", // origin must match your domain & subdomain
  icons: ["https://avatars.mywebsite.com/"],
}

const ethersAdapter = new EthersAdapter()
const solanaWeb3JsAdapter = new SolanaAdapter()
const bitcoinAdapter = new BitcoinAdapter()
const tonAdapter = new TonAdapter()

const networks = [
  mainnet,
  arbitrum,
  sepolia,
  solana,
  solanaTestnet,
  bitcoin,
  bitcoinTestnet,
  ton,
  tonTestnet,
] as [AppKitNetwork, ...AppKitNetwork[]]

createAppKit({
  adapters: [ethersAdapter, solanaWeb3JsAdapter, bitcoinAdapter, tonAdapter],
  metadata,
  networks,
  projectId,
  features: {
    analytics: true, // Optional - defaults to your Cloud configuration
    socials: false,
    email: false,
  },
})

export function AppKitContext({ children }: React.PropsWithChildren) {
  return <>{children}</>
}
