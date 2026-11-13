"use client"

import { NearConnector, type EventMap } from "@hot-labs/near-connect"
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from "react"

const NearContext = createContext<{
  accountId: string
  connect: () => Promise<void>
} | null>(null)

export function NearProvider({ children }: React.PropsWithChildren) {
  const connector = useMemo(() => new NearConnector({ network: "mainnet" }), [])
  const [accountId, setAccountId] = useState("")

  useEffect(() => {
    let cancelled = false
    const onSignIn = ({ accounts }: EventMap["wallet:signIn"]) =>
      setAccountId(accounts[0]?.accountId ?? "")
    const onSignOut = () => setAccountId("")

    connector.on("wallet:signIn", onSignIn)
    connector.on("wallet:signOut", onSignOut)
    void connector.getConnectedWallet().then(
      ({ accounts }) => !cancelled && setAccountId(accounts[0]?.accountId ?? ""),
      () => {},
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
    setAccountId(accounts[0]?.accountId ?? "")
  }, [connector])

  return (
    <NearContext.Provider value={{ accountId, connect }}>
      {children}
    </NearContext.Provider>
  )
}

export function useNearWallet() {
  const context = useContext(NearContext)
  if (!context) throw new Error("useNearWallet must be used within NearProvider")
  return context
}
