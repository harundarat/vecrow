import { Button } from "@/components/ui/button"
import { cn } from "cn"
import { ArrowUpRight, FlaskConicalIcon } from "lucide-react"
import { DotedHands } from "../../_assets/doted-hands"
import { Advantages } from "../blocks/advantages"

export function HeroSection() {
  return (
    <section>
      <HeroPanel />
    </section>
  )
}

function HeroPanel() {
  return (
    <div
      className={cn(
        "relative z-0 mx-auto mt-6 flex min-h-[calc(100svh-10rem)] w-[calc(100%-3rem)] flex-col overflow-clip rounded-3xl p-10 pt-28",
        "bg-[#151A25] bg-[linear-gradient(180deg,#172D3B00_34%,#172D3BCC_78%,#1F3450E6_89%,#253A60_100%)]"
      )}
    >
      {/* Decorative dot pattern */}
      <DotedHands className="absolute top-24 -left-56 w-full text-primary/15 z-0" />

      {/* Top-right: Advantages card */}
      <Advantages className="ml-auto" />

      {/* Bottom: Headline + CTA */}
      <div className="mt-auto flex flex-col gap-6 sm:flex-row sm:items-end sm:justify-between z-10">
        {/* Headline block */}
        <div className="flex flex-col gap-2">
          {/* Badge */}
          <div className="flex w-fit items-center gap-2 rounded-full border border-primary/20 bg-primary/5 px-3 py-1 text-xs font-medium text-primary">
            <FlaskConicalIcon className="size-3" />
            <span>Currently on testnet</span>
          </div>

          {/* Title */}
          <h1 className="text-5xl leading-tight font-semibold tracking-tight text-foreground">
            Agree on the work.
            <br />
            <span className="text-primary">Secure the payment.</span>
          </h1>
        </div>

        {/* CTA block */}
        <div className="flex w-96 shrink-0 flex-col items-start gap-4">
          <p className="text-muted-foreground">
            Work together with confidence. Funds are held in a smart contract
            and released under the terms both parties agree on.
          </p>
          <Button size={"lg"} className={"px-6"}>
            Get Started
            <ArrowUpRight />
          </Button>
        </div>
      </div>
    </div>
  )
}
