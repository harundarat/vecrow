import { cn } from "@/utils"
import { NavHeader } from "./_components/sections/nav-header"
import styles from "./landing.module.css"

export default function Layout({ children }: { children: React.ReactNode }) {
  return (
    <div className={cn("text-base", styles["landing-element"])}>
      <NavHeader />
      {children}
    </div>
  )
}
