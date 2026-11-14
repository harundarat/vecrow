"use client"

import Link from "next/link"
import { CircleCheck } from "lucide-react"
import { CopyField } from "@/components/copy-button"
import { buttonVariants } from "@/components/ui/button"
import type { CreatedEscrow } from "@/lib/api/escrow"

export function EscrowCreated({ escrow }: { escrow: CreatedEscrow }) {
  const other = escrow.creator_role === "freelancer" ? "client" : "freelancer"

  return (
    <section
      aria-labelledby="escrow-created-heading"
      className="flex flex-col gap-5 rounded-2xl bg-card p-4 sm:p-6"
    >
      <div className="flex items-start gap-3">
        <CircleCheck
          aria-hidden
          className="mt-0.5 size-6 shrink-0 text-primary"
        />
        <div>
          <h1 id="escrow-created-heading" className="text-xl font-medium">
            Escrow created
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Send this link to the {other}. They open it, add their details and
            sign. Then you approve, and the escrow goes on-chain.
          </p>
        </div>
      </div>

      <CopyField value={escrow.link} label="Escrow link" />

      {escrow.invitation && (
        <p className="text-sm text-muted-foreground">
          If the person you invited has a Vecrow account, the escrow also shows
          up there.
        </p>
      )}

      <div className="flex flex-col gap-2 sm:flex-row">
        <Link href={`/escrows/${escrow.id}`} className={buttonVariants()}>
          Open escrow
        </Link>
        <Link href="/app" className={buttonVariants({ variant: "outline" })}>
          Back to escrows
        </Link>
      </div>
    </section>
  )
}
