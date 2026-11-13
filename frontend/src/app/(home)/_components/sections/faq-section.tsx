import { Minus, Plus } from "lucide-react"

const questions = [
  {
    question: "What is Vecrow?",
    answer:
      "Vecrow helps clients and freelancers agree on work and hold payment in escrow until the agreed terms are met.",
  },
  {
    question: "How does escrow protect clients and freelancers?",
    answer:
      "Clients keep funds in escrow until the agreed terms are met. Freelancers can see that payment has been funded before completing the work.",
  },
  {
    question: "Can I connect a wallet from another chain?",
    answer:
      "Yes. Vecrow supports wallets across multiple chains, so you can connect using a supported wallet you already use.",
  },
  {
    question: "When are funds released?",
    answer:
      "Funds are released when the agreed terms are met and the work is approved according to the agreement.",
  },
  {
    question: "Can I use Vecrow beyond freelance projects?",
    answer:
      "Yes. Vecrow can support other projects where two parties want to agree on terms and secure payment through escrow.",
  },
]

export function FaqSection() {
  return (
    <section
      id="faq"
      className="mx-auto w-[calc(100%-3rem)] max-w-5xl py-20 sm:w-[calc(100%-5rem)] sm:py-28"
      aria-labelledby="faq-title"
    >
      <h2
        id="faq-title"
        className="mb-10 text-center text-4xl font-medium tracking-tight sm:mb-14 sm:text-5xl"
      >
        Frequently asked questions
      </h2>
      <div className="flex flex-col gap-3">
        {questions.map(({ question, answer }, index) => (
          <details
            key={question}
            open={index === 1}
            className="group rounded-2xl border border-border/70 bg-gradient-to-br from-card to-background open:border-primary/70 open:bg-accent"
          >
            <summary className="flex cursor-pointer list-none items-center justify-between gap-6 rounded-2xl px-5 py-5 text-lg font-medium marker:hidden sm:px-6 sm:py-6 sm:text-xl [&::-webkit-details-marker]:hidden">
              {question}
              <span className="grid size-10 shrink-0 place-items-center rounded-full border border-border text-primary group-open:border-primary/70 group-open:bg-primary/10">
                <Plus className="size-4 group-open:hidden" />
                <Minus className="hidden size-4 group-open:block" />
              </span>
            </summary>
            <p className="px-5 pb-6 text-base leading-relaxed text-muted-foreground sm:px-6 sm:pb-7 sm:text-lg">
              {answer}
            </p>
          </details>
        ))}
      </div>
    </section>
  )
}
