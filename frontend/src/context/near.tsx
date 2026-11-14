"use client"

import {
  NearConnector,
  type EventMap,
  type SignedMessage,
  type SignMessageParams,
} from "@hot-labs/near-connect"
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from "react"
import { env } from "@/env"
import type { DelegateActionRequest } from "@/lib/delegate-action"
import { DelegateUnsupportedError } from "@/lib/wallet-errors"

type NearContextValue = {
  accountId: string
  /** Resolves with the connected account ID, opening the wallet selector if none is connected. */
  connect: () => Promise<string>
  disconnect: () => Promise<void>
  /** Signs with the connected account and rejects a signature from another one. */
  signMessage: (
    params: Omit<SignMessageParams, "network" | "signerId">
  ) => Promise<SignedMessage>
  /** Signs one delegate action with the connected account; base64 Borsh. */
  signDelegateAction: (request: DelegateActionRequest) => Promise<string>
}

const NearContext = createContext<NearContextValue | null>(null)

export function NearProvider({ children }: React.PropsWithChildren) {
  const connector = useMemo(() => {
    const network = env.NEXT_PUBLIC_NEAR_NETWORK
    return new NearConnector({
      network,
      // Login needs signMessage; hide wallets that cannot do it.
      features: {
        signMessage: true,
        ...(network === "testnet" && { testnet: true }),
      },
    })
  }, [])
  const [accountId, setAccountId] = useState("")

  useEffect(() => {
    let cancelled = false
    const onSignIn = ({ accounts }: EventMap["wallet:signIn"]) =>
      setAccountId(accounts[0]?.accountId ?? "")
    const onSignOut = () => setAccountId("")

    connector.on("wallet:signIn", onSignIn)
    connector.on("wallet:signOut", onSignOut)
    void connector.getConnectedWallet().then(
      ({ accounts }) =>
        !cancelled && setAccountId(accounts[0]?.accountId ?? ""),
      () => {}
    )

    return () => {
      cancelled = true
      connector.off("wallet:signIn", onSignIn)
      connector.off("wallet:signOut", onSignOut)
    }
  }, [connector])

  const connect = useCallback(async () => {
    const connected = await connector.getConnectedWallet().catch(() => null)
    let id = connected?.accounts[0]?.accountId
    if (!id) {
      const wallet = await connector.connect()
      id = (await wallet.getAccounts())[0]?.accountId
    }
    setAccountId(id ?? "")
    if (!id) throw new Error("The NEAR wallet returned no account.")
    return id
  }, [connector])

  const disconnect = useCallback(async () => {
    await connector.disconnect()
    setAccountId("")
  }, [connector])

  const signMessage = useCallback<NearContextValue["signMessage"]>(
    async (params) => {
      const { wallet, accounts } = await connector.getConnectedWallet()
      const signerId = accounts[0]?.accountId
      const signed = await wallet.signMessage({
        ...params,
        network: connector.network,
        signerId,
      })
      if (signerId && signed.accountId && signed.accountId !== signerId) {
        throw new Error(
          `The wallet signed as ${signed.accountId}, not ${signerId}.`
        )
      }
      return signed
    },
    [connector]
  )

  const signDelegateAction = useCallback<
    NearContextValue["signDelegateAction"]
  >(
    async (request) => {
      const { wallet, accounts } = await connector.getConnectedWallet()
      if (!wallet.manifest.features?.signDelegateActions) {
        throw new DelegateUnsupportedError(wallet.manifest.name)
      }
      const { signedDelegateActions } = await wallet.signDelegateActions({
        delegateActions: [request],
        network: connector.network,
        signerId: accounts[0]?.accountId,
      })
      const signed = signedDelegateActions?.[0]
      if (!signed) throw new Error("The NEAR wallet returned no signature.")
      return signed
    },
    [connector]
  )

  const value = useMemo(
    () => ({ accountId, connect, disconnect, signMessage, signDelegateAction }),
    [accountId, connect, disconnect, signMessage, signDelegateAction]
  )

  return <NearContext.Provider value={value}>{children}</NearContext.Provider>
}

export function useNearWallet() {
  const context = useContext(NearContext)
  if (!context)
    throw new Error("useNearWallet must be used within NearProvider")
  return context
}
