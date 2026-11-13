import { Button } from "@/components/ui/button"
import { cn } from "cn"
import {
  ArrowUpRight,
  BadgeCheckIcon,
  DiagramFlowVerticalIcon,
  FlaskConicalIcon,
  LayersIcon,
  LockKeyholeIcon,
} from "lucide-react"
import { DotedHands } from "../../_assets/doted-hands"
import Image from "next/image"

export function HeroSection() {
  return (
    <section>
      <HeroPanel />
      <VecrowBuiltIn />
    </section>
  )
}

function HeroPanel() {
  return (
    <div
      className={cn(
        "relative z-0 mx-auto mt-2 flex min-h-[calc(100svh-2rem)] w-[calc(100%-1rem)] flex-col overflow-clip rounded-3xl p-6 pt-28 sm:mt-6 sm:min-h-[calc(100svh-10rem)] sm:w-[calc(100%-3rem)] sm:p-10 lg:flex-row",
        "relative bg-[#151A25] bg-[linear-gradient(180deg,#172D3B00_34%,#172D3BCC_78%,#1F3450E6_89%,#253A60_100%)]"
      )}
    >
      {/* Decorative dot pattern */}
      <DotedHands className="absolute -top-40 left-1/2 z-0 w-[170%] -translate-x-1/2 rotate-0 text-primary/15 sm:top-16 sm:-left-24 sm:w-[130%] sm:translate-x-0 lg:top-28 lg:-left-56 lg:w-full lg:rotate-0" />

      {/* Headline block */}
      <div className="mt-auto mb-8 flex flex-col gap-2 sm:mb-0">
        <div className="flex w-fit items-center gap-2 rounded-full border border-primary/20 bg-primary/5 px-3 py-1 text-xs font-medium text-primary">
          <FlaskConicalIcon className="size-3" />
          <span>Currently on testnet</span>
        </div>

        <h1 className="text-5xl font-semibold tracking-tight text-pretty text-foreground sm:leading-tight">
          Agree on the work. <br className="hidden sm:inline" />
          <span className="text-primary">Secure the payment.</span>
        </h1>
      </div>

      {/* Top-right: Advantages card */}
      <Advantages className="top-28 right-10 w-full sm:absolute sm:max-w-96" />

      {/* CTA block */}
      <div className="mt-6 flex w-full shrink-0 flex-col items-start gap-4 sm:max-w-96 lg:mt-auto lg:ml-auto">
        <p className="text-justify text-muted-foreground sm:text-left">
          Work together with confidence. Funds are held in a smart contract and
          released under the terms both parties agree on.
        </p>
        <Button size={"lg"} className={"w-full px-6 sm:w-fit"}>
          Get Started
          <ArrowUpRight />
        </Button>
      </div>
    </div>
  )
}

function VecrowBuiltIn() {
  const builts = [
    {
      title: "Smart contract",
      lable: "Funds held in ",
      icon: LockKeyholeIcon,
    },
    {
      title: "Agreed Terms",
      lable: "Payment Release",
      icon: BadgeCheckIcon,
    },
    {
      title: "Cross-chain",
      lable: "Transaction Reach",
      icon: DiagramFlowVerticalIcon,
    },
    {
      title: "Build on",
      lable: "NEAR Protocol",
      icon: LayersIcon,
    },
  ]
  return (
    <ul className="mx-auto mt-6 grid min-h-22 w-[calc(100%-3rem)] grid-cols-2 gap-8 py-8 sm:px-10 md:flex md:gap-10">
      {builts.map((built, index) => (
        <li
          key={index}
          className="flex flex-1 items-center gap-4 border-primary/10 md:not-last:border-r"
        >
          <built.icon className="size-5 text-primary/80" />
          <div className="gap- flex flex-col">
            <span className="text-xs text-muted-foreground">{built.lable}</span>
            <span className="text-lg font-medium">{built.title}</span>
          </div>
        </li>
      ))}
    </ul>
  )
}

function Advantages({ className }: { className?: string }) {
  const cryptoIcons = [
    { src: "/crypto-icons/solana.png", alt: "Solana" },
    { src: "/crypto-icons/bitcoin.png", alt: "Bitcoin" },
    { src: "/crypto-icons/ethereum.png", alt: "Ethereum" },
    { src: "/crypto-icons/near-protocol.png", alt: "Near Protocol" },
  ]

  return (
    <div
      className={cn(
        "inline-flex h-fit flex-col items-start justify-start gap-4 rounded-xl -bg-linear-147 from-neptune-200/6 from-16% to-neptune-300/6 to-84% p-6 text-pretty",
        className
      )}
    >
      <div className="justify-start text-xl font-medium sm:text-2xl">
        Cross-chain transaction. Held in escrow until agreed terms are met.
      </div>
      <div className="flex items-center gap-2">
        <div className="flex shrink-0 items-center">
          {cryptoIcons.map((icon, index) => (
            <Image
              key={index}
              height={32}
              width={32}
              className={cn(
                `relative size-8 rounded-full border-2 border-neptune-950`,
                index > 0 && "-ml-2"
              )}
              src={icon.src}
              alt={icon.alt}
            />
          ))}
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
