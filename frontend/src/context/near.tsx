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

type NearContextValue = {
  accountId: string
  /** Opens the wallet selector and resolves with the connected account ID. */
  connect: () => Promise<string>
  disconnect: () => Promise<void>
  signMessage: (
    params: Omit<SignMessageParams, "network" | "signerId">
  ) => Promise<SignedMessage>
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
    const wallet = await connector.connect()
    const accounts = await wallet.getAccounts()
    const id = accounts[0]?.accountId ?? ""
    setAccountId(id)
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
      return wallet.signMessage({
        ...params,
        network: connector.network,
        signerId: accounts[0]?.accountId,
      })
    },
    [connector]
  )

  const value = useMemo(
    () => ({ accountId, connect, disconnect, signMessage }),
    [accountId, connect, disconnect, signMessage]
  )

  return <NearContext.Provider value={value}>{children}</NearContext.Provider>
}

export function useNearWallet() {
  const context = useContext(NearContext)
  if (!context)
    throw new Error("useNearWallet must be used within NearProvider")
  return context
}
