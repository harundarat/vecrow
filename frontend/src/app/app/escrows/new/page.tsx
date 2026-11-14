import type { Metadata } from "next"
import { NewEscrow } from "./_components/new-escrow"

export const metadata: Metadata = {
  title: "New escrow - Vecrow",
}

export default function Page() {
  return <NewEscrow />
}
