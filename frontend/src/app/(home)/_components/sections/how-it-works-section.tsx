import { Check, CheckCheck, CircleCheck, Send, Wallet } from "lucide-react"

const steps = [
  {
    number: "01",
    title: "Connect your wallet",
    description:
      "Connect from any supported chain, start with the wallet you already use.",
    illustration: "wallet",
  },
  {
    number: "02",
    title: "Set the agreement",
    description:
      "Agree on the scope, deadline, and payment terms before work begins.",
    illustration: "agreement",
  },
  {
    number: "03",
    title: "Fund the escrow",
    description:
      "Deposit the agreed amount. Funds stay in a smart contract until the terms are met.",
    illustration: "secured",
  },
  {
    number: "04",
    title: "Deliver as agreed",
    description:
      "Complete and submit the work for review, following the shared agreement.",
    illustration: "deliver",
  },
  {
    number: "05",
    title: "Release funds",
    description:
      "Once the agreed terms are met, release the payment to the freelancer.",
    illustration: "released",
  },
]

export function HowItWorksSection() {
  return (
    <section
      id="how-it-works"
      className="mx-auto grid w-[calc(100%-3rem)] grid-cols-1 gap-4 py-20 sm:w-[calc(100%-5rem)] sm:grid-cols-2 lg:grid-cols-3 lg:gap-5 lg:py-28"
      aria-labelledby="how-it-works-title"
    >
      <div className="flex flex-col justify-center pb-4 sm:col-span-2 lg:col-span-1 lg:row-start-1 lg:pb-0">
        <h2
          id="how-it-works-title"
          className="text-4xl font-medium tracking-tight sm:text-5xl"
        >
          How it works
        </h2>
        <p className="mt-4 max-w-xl text-base leading-relaxed text-muted-foreground">
          From the first agreement to the final payment. Five steps to keep both
          sides on the same page.
        </p>
      </div>
      {steps.map((step, index) => (
        <article
          key={step.number}
          className={`flex min-h-[22rem] flex-col rounded-2xl border border-border/70 bg-gradient-to-br from-card to-background p-6 sm:min-h-[25rem] sm:p-8 ${
            index === 0
              ? "lg:col-start-2 lg:row-start-1"
              : index === 1
                ? "lg:col-start-3 lg:row-start-1"
                : index === 2
                  ? "lg:col-start-1 lg:row-start-2"
                  : index === 3
                    ? "lg:col-start-2 lg:row-start-2"
                    : "lg:col-start-3 lg:row-start-2"
          }`}
        >
          <span className="font-mono text-sm tracking-widest text-neptune-300">
            {step.number}
          </span>
          <StepIllustration kind={step.illustration} />
          <div className="mt-auto pt-8">
            <h3 className="text-2xl font-medium tracking-tight">
              {step.title}
            </h3>
            <p className="mt-2 leading-relaxed text-muted-foreground">
              {step.description}
            </p>
          </div>
        </article>
      ))}
    </section>
  )
}

function StepIllustration({ kind }: { kind: string }) {
  if (kind === "wallet") {
    return (
      <div className="flex flex-1 flex-col items-center justify-center gap-5 text-primary/80">
        <Wallet className="size-10" strokeWidth={1.8} />
        <span className="rounded-full bg-primary/10 px-4 py-2 text-xs text-neptune-300">
          <span className="mr-2 inline-block size-1.5 rounded-full bg-primary" />
          Any supported chain
        </span>
      </div>
    )
  }

  if (kind === "agreement") {
    return (
      <ul className="my-7 flex flex-1 flex-col justify-center gap-4 rounded-xl border border-border bg-background/50 p-5 text-sm text-muted-foreground">
        {["Scope defined", "Deadline set", "Payment agreed"].map((item) => (
          <li key={item} className="flex items-center gap-3">
            <Check className="size-4 text-primary" />
            {item}
          </li>
        ))}
      </ul>
    )
  }

  if (kind === "secured") {
    return (
      <div className="my-7 flex flex-1 flex-col items-center justify-center rounded-xl border border-border bg-background/50 p-6 text-center">
        <CircleCheck className="size-10 text-primary" />
        <span className="mt-4 text-2xl font-medium">Funds secured</span>
        <span className="mt-2 text-sm text-muted-foreground">
          Held in smart contract
        </span>
      </div>
    )
  }

  if (kind === "deliver") {
    return (
      <div className="flex flex-1 items-center justify-center text-primary/80">
        <Send className="size-11" strokeWidth={1.6} />
      </div>
    )
  }

  return (
    <div className="flex flex-1 flex-col items-center justify-center text-center">
      <div className="flex size-20 items-center justify-center rounded-full border border-primary/30 bg-primary/10 text-primary">
        <CheckCheck className="size-9" strokeWidth={1.6} />
      </div>
      <span className="mt-5 text-xl font-medium">Payment released</span>
      <span className="mt-1 text-sm text-muted-foreground">
        Work verified. Payment secured.
      </span>
    </div>
  )
}
