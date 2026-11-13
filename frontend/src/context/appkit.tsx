"use client"

import { createAppKit } from "@reown/appkit/react"
import { SolanaAdapter } from "@reown/appkit-adapter-solana"
import { EthersAdapter } from "@reown/appkit-adapter-ethers"
import {
  arbitrum,
  mainnet,
  sepolia,
  solana,
  solanaTestnet,
  type AppKitNetwork,
} from "@reown/appkit/networks"
import { env } from "@/env"

// Wallets check that the metadata URL matches the page, so it has to follow
// whatever origin the app is served from. This module also runs during
// server rendering, where no wallet connects.
const origin = typeof window === "undefined" ? "" : window.location.origin

const networks = [mainnet, arbitrum, sepolia, solana, solanaTestnet] as [
  AppKitNetwork,
  ...AppKitNetwork[],
]

export const appKit = createAppKit({
  adapters: [new EthersAdapter(), new SolanaAdapter()],
  metadata: {
    name: "Vecrow",
    description:
      "Vecrow is an escrow service for crypto payments, built on NEAR Protocol.",
    url: origin,
    icons: [`${origin}/favicon.svg`],
  },
  networks,
  projectId: env.NEXT_PUBLIC_REOWN_PROJECT_ID,
  features: {
    analytics: true,
    socials: false,
    email: false,
  },
})

export function AppKitContext({ children }: React.PropsWithChildren) {
  return <>{children}</>
}
