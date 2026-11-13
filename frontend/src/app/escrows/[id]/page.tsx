import type { Metadata } from "next"
import { EscrowOverview } from "./_components/escrow-overview"

export const metadata: Metadata = {
  title: "Escrow - Vecrow",
}

export default async function Page({
  params,
}: {
  params: Promise<{ id: string }>
}) {
  const { id } = await params
  return <EscrowOverview id={id} />
}
