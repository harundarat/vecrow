import { Geist_Mono, Manrope } from "next/font/google"
import { ThemeProvider } from "@/components/theme-provider"
import { cn } from "@/utils"
import { rootMetadata } from "./metadata"
import "./globals.css"
import { AppKitContext } from "@/context/appkit"
import { NearProvider } from "@/context/near"

const manrope = Manrope({ subsets: ["latin"], variable: "--font-sans" })
const fontMono = Geist_Mono({
  subsets: ["latin"],
  variable: "--font-mono",
})

export const metadata = rootMetadata

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode
}>) {
  return (
    <html
      lang="en"
      suppressHydrationWarning
      className={cn(
        "antialiased",
        fontMono.variable,
        "font-sans",
        manrope.variable
      )}
    >
      <body>
        <ThemeProvider>
          <AppKitContext>
            <NearProvider>{children}</NearProvider>
          </AppKitContext>
        </ThemeProvider>
      </body>
    </html>
  )
}
