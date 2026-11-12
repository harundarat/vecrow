import { cn } from "@/utils"
import { SVGProps } from "react"

export function VecrowIcon({ className, ...props }: SVGProps<SVGSVGElement>) {
  return (
    <svg
      width="154"
      height="173"
      viewBox="0 0 154 173"
      fill="currentColor"
      xmlns="http://www.w3.org/2000/svg"
      className={cn("h-full", className)}
      {...props}
    >
      <path
        d="M0 85.9666L39.3378 62.1773L105.118 173H52.1733L0 85.9666Z"
        fill="currentColor"
      />
      <path d="M108.142 0H154V172.715H108.142V0Z" fill="currentColor" />
    </svg>
  )
}
