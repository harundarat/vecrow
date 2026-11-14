"use client"

import { useAppKitAccount } from "@reown/appkit/react"
import type { Provider as EVMProvider } from "@reown/appkit-adapter-ethers"
import type { Provider as SolanaProvider } from "@reown/appkit-adapter-solana"
import { BrowserProvider } from "ethers"
import { useMemo } from "react"
import { appKit } from "@/context/appkit"
import { useNearWallet } from "@/context/near"
import type { SignRequest, WalletScheme, WalletSignature } from "@/lib/api/auth"
import { WalletCancelledError } from "@/lib/wallet-errors"
import {
  nep413Nonce,
  toEip191Signature,
  toNep413Proof,
  toSolanaSignature,
} from "@/lib/wallet-encoding"

export type WalletChain = "evm" | "solana" | "near"

export const WALLET_SCHEME: Record<WalletChain, WalletScheme> = {
  evm: "eip191",
  solana: "ed25519",
  near: "nep413",
}

export const SCHEME_CHAIN: Record<WalletScheme, WalletChain> = {
  eip191: "evm",
  ed25519: "solana",
  nep413: "near",
}

type AppKitNamespace = "eip155" | "solana"

const NAMESPACE: Record<Exclude<WalletChain, "near">, AppKitNamespace> = {
  evm: "eip155",
  solana: "solana",
}

export type WalletSigner = {
  scheme: WalletScheme
  /** The connected address, or undefined when no wallet of this chain is connected. */
  address: string | undefined
  /** Connects a wallet of this chain (if needed) and resolves with its address. */
  connect: () => Promise<string>
  disconnect: () => Promise<void>
  /** Signs `message` exactly as given and returns it in the backend's encoding. */
  sign: (request: SignRequest) => Promise<WalletSignature>
}

export function useWalletSigner(chain: WalletChain): WalletSigner {
  const near = useNearWallet()
  const namespace = chain === "near" ? undefined : NAMESPACE[chain]
  const appKitAccount = useAppKitAccount({ namespace: namespace ?? "eip155" })

  const address =
    chain === "near"
      ? near.accountId || undefined
      : appKitAccount.isConnected
        ? appKitAccount.address
        : undefined

  const { connect: connectNear, disconnect: disconnectNear } = near
  const signNearMessage = near.signMessage

  return useMemo<WalletSigner>(() => {
    if (chain === "near") {
      return {
        scheme: WALLET_SCHEME.near,
        address,
        connect: connectNear,
        disconnect: disconnectNear,
        sign: async ({ message, nonce, recipient }) =>
          toNep413Proof(
            await signNearMessage({
              message,
              recipient,
              nonce: nep413Nonce(nonce),
            })
          ),
      }
    }

    const ns = NAMESPACE[chain]
    return {
      scheme: WALLET_SCHEME[chain],
      address,
      connect: () => connectAppKit(ns),
      disconnect: () => appKit.disconnect(ns),
      sign: ({ message }) =>
        ns === "eip155" ? signEvm(message) : signSolana(message),
    }
  }, [chain, address, connectNear, disconnectNear, signNearMessage])
}

/** One signer per chain, for pages that let the visitor pick. */
export function useWalletSigners(): Record<WalletChain, WalletSigner> {
  const near = useWalletSigner("near")
  const solana = useWalletSigner("solana")
  const evm = useWalletSigner("evm")
  return useMemo(() => ({ near, solana, evm }), [near, solana, evm])
}

function sameAddress(scheme: WalletScheme, a: string, b: string) {
  return scheme === "eip191" ? a.toLowerCase() === b.toLowerCase() : a === b
}

/**
 * Connects the signer's wallet and checks it is `expected`. Another
 * connected account is disconnected first so the wallet can switch.
 */
export async function connectWalletAs(
  signer: WalletSigner,
  expected: string
): Promise<string> {
  if (signer.address && !sameAddress(signer.scheme, signer.address, expected)) {
    await signer.disconnect()
  }
  const address = await signer.connect()
  if (!sameAddress(signer.scheme, address, expected)) {
    throw new Error(
      `Your wallet is on ${address}. Switch to ${expected} and try again.`
    )
  }
  return address
}

function connectedAddress(namespace: AppKitNamespace): string | undefined {
  const account = appKit.getAccount(namespace)
  return account?.isConnected ? account.address : undefined
}

/** Opens the AppKit modal and waits until a wallet connects or the modal closes. */
function connectAppKit(namespace: AppKitNamespace): Promise<string> {
  const existing = connectedAddress(namespace)
  if (existing) return Promise.resolve(existing)

  return new Promise<string>((resolve, reject) => {
    let opened = false
    let settled = false
    const unsubscribers: Array<() => void> = []
    const finish = (address?: string) => {
      if (settled) return
      settled = true
      unsubscribers.forEach((unsubscribe) => unsubscribe())
      if (address) resolve(address)
      else reject(new WalletCancelledError())
    }

    unsubscribers.push(
      appKit.subscribeAccount((account) => {
        if (account.isConnected && account.address) finish(account.address)
      }, namespace),
      appKit.subscribeState((state) => {
        if (state.open) {
          opened = true
        } else if (opened) {
          // The modal closes as soon as a wallet connects, sometimes before
          // the account state is published, so give it a moment to arrive.
          setTimeout(() => finish(connectedAddress(namespace)), 500)
        }
      })
    )

    appKit.open({ view: "Connect", namespace }).catch((err: unknown) => {
      if (settled) return
      settled = true
      unsubscribers.forEach((unsubscribe) => unsubscribe())
      reject(err)
    })
  })
}

async function signEvm(message: string): Promise<WalletSignature> {
  const provider = appKit.getProvider<EVMProvider>("eip155")
  const address = connectedAddress("eip155")
  if (!provider || !address) throw new Error("Connect an EVM wallet first.")
  const signer = await new BrowserProvider(provider).getSigner(address)
  return { signature: toEip191Signature(await signer.signMessage(message)) }
}

async function signSolana(message: string): Promise<WalletSignature> {
  const provider = appKit.getProvider<SolanaProvider>("solana")
  if (!provider || !connectedAddress("solana")) {
    throw new Error("Connect a Solana wallet first.")
  }
  const signature = await provider.signMessage(
    new TextEncoder().encode(message)
  )
  return { signature: toSolanaSignature(signature) }
}
