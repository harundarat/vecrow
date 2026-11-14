import { cn } from "@/utils"
import Image from "next/image"

export function Advantages({ className }: { className?: string }) {
  return (
    <div
      className={cn(
        "inline-flex h-fit w-96 flex-col items-start justify-start gap-4 rounded-xl -bg-linear-147 from-neptune-200/6 from-16% to-neptune-300/6 to-84% p-6 text-pretty",
        className
      )}
    >
      <div className="justify-start text-2xl font-medium">
        Cross-chain transaction. Held in escrow until agreed terms are met.
      </div>
      <div className="flex items-center gap-2">
        <div className="flex shrink-0 items-center">
          <Image
            height={32}
            width={32}
            className="relative size-8 rounded-full border-2 border-neptune-950"
            src="/crypto-icons/solana.png"
            alt="Solana"
          />
          <Image
            height={32}
            width={32}
            className="relative -ml-2 size-8 rounded-full border-2 border-neptune-950"
            src="/crypto-icons/bitcoin.png"
            alt="bitcoin"
          />
          <Image
            height={32}
            width={32}
            className="relative -ml-2 size-8 rounded-full border-2 border-neptune-950"
            src="/crypto-icons/ethereum.png"
            alt="Ethereum"
          />
          <Image
            height={32}
            width={32}
            className="relative z-0 -ml-2 size-8 rounded-full border-2 border-neptune-950"
            src="/crypto-icons/near-protocol.png"
            alt="Near Protocol"
          />
          <span className="z-20 -ml-2 flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-neptune-900 text-xs font-semibold">
            +25
          </span>
        </div>
        <p className="grow text-sm text-muted-foreground">
          Supported chains on near protocol
        </p>
      </div>
    </div>
  )
}
