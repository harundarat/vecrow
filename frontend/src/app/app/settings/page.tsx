import type { Metadata } from "next"
import { AccountSettings } from "./_components/account-settings"

export const metadata: Metadata = {
  title: "Settings - Vecrow",
}

export default function Page() {
  return <AccountSettings />
}
