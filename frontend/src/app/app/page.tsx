import type { Metadata } from "next"
import { AppHome } from "./_components/app-home"

export const metadata: Metadata = {
  title: "Escrows - Vecrow",
}

export default function Page() {
  return <AppHome />
}
