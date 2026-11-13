"use client"

import Image from "next/image"
import { useState } from "react"
import { LoaderCircle, Wallet as WalletIcon } from "lucide-react"
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Button } from "@/components/ui/button"
import { useAuth } from "@/context/auth"
import {
  useWalletSigner,
  type WalletChain,
  type WalletSigner,
} from "@/hooks/use-wallet-signer"
import { linkWallet, unlinkWallet } from "@/lib/api/account"
import {
  proveWallet,
  type Account,
  type Wallet,
  type WalletScheme,
} from "@/lib/api/auth"
import { describeApiError } from "@/lib/api/errors"
import { formatDate, shortenAddress, WALLET_SCHEME_LABEL } from "@/lib/format"
import { describeWalletError } from "@/lib/wallet-errors"
import {
  FeedbackMessage,
  SettingsSection,
  type Feedback,
} from "./settings-section"

const CHAINS: Array<{ chain: WalletChain; label: string; icon: string }> = [
  { chain: "near", label: "NEAR", icon: "/crypto-icons/near-protocol.png" },
  { chain: "solana", label: "Solana", icon: "/crypto-icons/solana.png" },
  { chain: "evm", label: "EVM", icon: "/crypto-icons/ethereum.png" },
]

type Step = "connecting" | "signing"

export function WalletsSection({ account }: { account: Account }) {
  const { recovery, request, setAccount } = useAuth()
  const signers: Record<WalletChain, WalletSigner> = {
    near: useWalletSigner("near"),
    solana: useWalletSigner("solana"),
    evm: useWalletSigner("evm"),
  }
  const [linking, setLinking] = useState<{
    chain: WalletChain
    step: Step
  } | null>(null)
  const [unlinkTarget, setUnlinkTarget] = useState<Wallet | null>(null)
  const [unlinking, setUnlinking] = useState(false)
  const [feedback, setFeedback] = useState<Feedback>(null)

  const onlyWallet = account.wallets.length === 1
  const busy = linking !== null || unlinking

  function isLinked(scheme: WalletScheme, address: string) {
    return account.wallets.some(
      (wallet) =>
        wallet.scheme === scheme && sameAddress(scheme, wallet.address, address)
    )
  }

  async function handleLink(chain: WalletChain) {
    if (busy) return
    const signer = signers[chain]
    setFeedback(null)
    setLinking({ chain, step: "connecting" })
    try {
      // The wallet used to sign in usually stays connected; drop it so the
      // wallet window opens and another account can be picked.
      if (signer.address && isLinked(signer.scheme, signer.address)) {
        await signer.disconnect()
      }
      const address = await signer.connect()
      if (isLinked(signer.scheme, address)) {
        setFeedback({
          kind: "error",
          message: `${shortenAddress(address)} is already linked. Pick another account in your wallet and try again.`,
        })
        return
      }
      setLinking({ chain, step: "signing" })
      const proof = await proveWallet({
        scheme: signer.scheme,
        address,
        sign: signer.sign,
      })
      setAccount(await linkWallet(request, proof))
      setFeedback({
        kind: "success",
        message: recovery
          ? `Linked ${shortenAddress(address)}. Sign in with it to use everything again.`
          : `Linked ${shortenAddress(address)}. You can now sign in with it.`,
      })
    } catch (err) {
      setFeedback({ kind: "error", message: describeWalletError(err) })
    } finally {
      setLinking(null)
    }
  }

  async function handleUnlink() {
    if (!unlinkTarget) return
    setUnlinking(true)
    setFeedback(null)
    try {
      await unlinkWallet(request, unlinkTarget.id)
      setAccount({
        ...account,
        wallets: account.wallets.filter((w) => w.id !== unlinkTarget.id),
      })
      setFeedback({
        kind: "success",
        message: `Unlinked ${shortenAddress(unlinkTarget.address)}.`,
      })
    } catch (err) {
      setFeedback({ kind: "error", message: describeApiError(err) })
    } finally {
      setUnlinking(false)
      setUnlinkTarget(null)
    }
  }

  return (
    <SettingsSection
      id="wallets"
      title="Wallets"
      description="You can sign in with any of these wallets. Each one is linked by signing a message with it."
    >
      <ul className="flex flex-col divide-y rounded-xl border">
        {account.wallets.map((wallet) => (
          <li key={wallet.id} className="flex items-center gap-3 p-3">
            <WalletIcon aria-hidden className="size-5 shrink-0 text-primary" />
            <span className="min-w-0 flex-1">
              <span
                className="block truncate font-mono text-sm"
                title={wallet.address}
              >
                {shortenAddress(wallet.address)}
              </span>
              <span className="text-xs text-muted-foreground">
                {WALLET_SCHEME_LABEL[wallet.scheme] ?? wallet.scheme} · linked{" "}
                {formatDate(wallet.created_at)}
              </span>
            </span>
            <Button
              variant="outline"
              size="sm"
              disabled={onlyWallet || busy}
              aria-describedby={onlyWallet ? "only-wallet-note" : undefined}
              onClick={() => setUnlinkTarget(wallet)}
            >
              Unlink
              <span className="sr-only"> {wallet.address}</span>
            </Button>
          </li>
        ))}
      </ul>
      {onlyWallet && (
        <p id="only-wallet-note" className="text-sm text-muted-foreground">
          Your only wallet cannot be unlinked. Link another wallet first.
        </p>
      )}

      <div className="flex flex-col gap-2">
        <h3 className="text-sm font-medium">Link a wallet</h3>
        <div className="grid grid-cols-3 gap-2">
          {CHAINS.map(({ chain, label, icon }) => (
            <Button
              key={chain}
              variant="outline"
              disabled={busy}
              onClick={() => void handleLink(chain)}
            >
              {linking?.chain === chain ? (
                <LoaderCircle aria-hidden className="animate-spin" />
              ) : (
                <Image
                  src={icon}
                  alt=""
                  width={16}
                  height={16}
                  className="rounded-full"
                />
              )}
              {label}
            </Button>
          ))}
        </div>
        <p aria-live="polite" className="min-h-5 text-sm text-muted-foreground">
          {linking?.step === "connecting" &&
            `Connect the ${chainLabel(linking.chain)} wallet you want to link…`}
          {linking?.step === "signing" &&
            "Sign the message in your wallet. Signing is free and sends no transaction."}
        </p>
      </div>

      <FeedbackMessage feedback={feedback} />

      <AlertDialog
        open={unlinkTarget !== null}
        onOpenChange={(open) => {
          if (!open && !unlinking) setUnlinkTarget(null)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Unlink this wallet?</AlertDialogTitle>
            <AlertDialogDescription>
              You will no longer be able to sign in with{" "}
              <span className="font-mono">
                {unlinkTarget && shortenAddress(unlinkTarget.address)}
              </span>
              , and escrows you take part in only through it will stop showing
              in your account. You can link it again later.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={unlinking}>
              Keep wallet
            </AlertDialogCancel>
            <Button
              variant="destructive"
              disabled={unlinking}
              onClick={() => void handleUnlink()}
            >
              {unlinking && (
                <LoaderCircle aria-hidden className="animate-spin" />
              )}
              Unlink
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </SettingsSection>
  )
}

function chainLabel(chain: WalletChain) {
  return CHAINS.find((c) => c.chain === chain)?.label ?? chain
}

// EVM addresses are hex, so letter case carries no meaning there.
function sameAddress(scheme: WalletScheme, a: string, b: string) {
  return scheme === "eip191" ? a.toLowerCase() === b.toLowerCase() : a === b
}
