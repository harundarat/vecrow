import type { Metadata } from "next"
import { EscrowPage } from "./_components/escrow-page"

export const metadata: Metadata = {
  title: "Escrow - Vecrow",
}

export default async function Page({
  params,
}: {
  params: Promise<{ id: string }>
}) {
  const { id } = await params
  return <EscrowPage id={id} />
}
