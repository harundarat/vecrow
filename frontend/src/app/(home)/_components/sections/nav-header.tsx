import { VecrowIcon } from "@/assets/vecrow-icon"
import { Button } from "@/components/ui/button"
import { cn } from "cn"
import { BookOpenIcon, CircleQuestionMarkIcon, Grid2x2Icon } from "lucide-react"
import Link from "next/link"

const navItems = [
  {
    label: "How it Works",
    href: "#how-it-works",
    icon: Grid2x2Icon,
  },
  {
    label: "Documentation",
    href: "#documentation",
    icon: BookOpenIcon,
  },
  {
    label: "FAQ",
    href: "#faq",
    icon: CircleQuestionMarkIcon,
  },
]

export function NavHeader() {
  return (
    <header
      className={cn(
        "absolute top-6 left-1/2 z-50 flex h-20 w-[calc(100%-3rem)] -translate-x-1/2 border-b border-primary/10 px-6"
      )}
    >
      <div className="flex flex-1 items-center gap-4 px-4">
        <Link href={"/"}>
          <VecrowIcon className="w-6" />
        </Link>
      </div>
      {navItems.map((item, index) => (
        <div
          key={index}
          className="flex flex-1 items-center gap-4 border-l border-primary/10 px-4"
        >
          <Link
            href={item.href}
            className="flex items-center gap-2 p-2 text-sm text-nowrap transition-colors hover:text-neptune-400"
          >
            <item.icon className="size-4" strokeWidth={1.5} />
            {item.label}
          </Link>
        </div>
      ))}
      <div className="flex flex-1 items-center gap-4 border-l border-primary/10 px-4">
        <Button variant={"primary-light"} className={"ml-auto px-5"}>
          Sign in
        </Button>
      </div>
    </header>
  )
}
