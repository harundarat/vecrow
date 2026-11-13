import { useAppKitAccount, useAppKitProvider } from "@reown/appkit/react"
import { BrowserProvider } from "ethers"
import { useCallback } from "react"
import type { Provider as EVMProvider } from "@reown/appkit-adapter-ethers"
import type { Provider as SolanaProvider } from "@reown/appkit-adapter-solana"
import type { BitcoinConnector } from "@reown/appkit-adapter-bitcoin"

export const useEVMSign = () => {
  const { walletProvider } = useAppKitProvider<EVMProvider>("eip155")
  const signMessage = useCallback(
    async (message: string) => {
      const provider = new BrowserProvider(walletProvider)
      const signer = await provider.getSigner()
      const signature = await signer?.signMessage(message)
      return signature
    },
    [walletProvider]
  )

  return { signMessage }
}

export const useSolanaSign = () => {
  const { address } = useAppKitAccount()
  const { walletProvider } = useAppKitProvider<SolanaProvider>("solana")

  const signMessage = useCallback(
    async (message: string) => {
      if (!walletProvider || !address) throw Error("user is disconnected")
      const encoded = new TextEncoder().encode(message)
      const signature = await walletProvider.signMessage(encoded)
      return signature
    },
    [walletProvider, address]
  )

  return { signMessage }
}

export const useBitcoinSign = () => {
  const { address } = useAppKitAccount()
  const { walletProvider } = useAppKitProvider<BitcoinConnector>("bip122")

  const signMessage = useCallback(
    async (message: string) => {
      if (!walletProvider || !address) throw Error("user is disconnected")
      const signature = await walletProvider.signMessage({
        address,
        message,
      })
      return signature
    },
    [walletProvider, address]
  )

  return { signMessage }
}
