"use client"

import Link from "next/link"
import { Button } from "@/components/ui/button"
import { useAuth } from "@/context/auth"

/** Opens the app for a signed-in visitor, or the sign-in page otherwise. */
export function AppEntryButton({
  signedOutLabel = "Sign in",
  icon,
  ...props
}: Omit<
  React.ComponentProps<typeof Button>,
  "render" | "nativeButton" | "children"
> & {
  signedOutLabel?: string
  icon?: React.ReactNode
}) {
  const { status } = useAuth()
  const signedIn = status === "authenticated"
  return (
    <Button
      {...props}
      nativeButton={false}
      render={<Link href={signedIn ? "/app" : "/sign-in"} />}
    >
      {signedIn ? "Open app" : signedOutLabel}
      {icon}
    </Button>
  )
}
