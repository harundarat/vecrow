import { VecrowIcon } from "@/assets/vecrow-icon"
import { VecrowLogoWordmark } from "@/assets/vecrow-logo-wordmark"
import { Button } from "@/components/ui/button"
import {
  BookOpenIcon,
  CircleQuestionMarkIcon,
  Grid2x2Icon,
  MenuIcon,
  XIcon,
} from "lucide-react"
import {
  Drawer,
  DrawerClose,
  DrawerContent,
  DrawerFooter,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@/components/ui/drawer"
import Link from "next/link"
import { AppEntryButton } from "../app-entry-button"
import { DOCS_URL } from "../links"

const navItems = [
  {
    label: "How it Works",
    href: "#how-it-works",
    icon: Grid2x2Icon,
  },
  {
    label: "Documentation",
    href: DOCS_URL,
    icon: BookOpenIcon,
    external: true,
  },
  {
    label: "FAQ",
    href: "#faq",
    icon: CircleQuestionMarkIcon,
  },
]

export function NavHeader() {
  return (
    <header className="absolute top-2 left-1/2 z-50 flex h-20 w-[calc(100%-1rem)] -translate-x-1/2 border-b border-primary/10 px-2 sm:top-0 sm:w-[calc(100%-3rem)] sm:px-6">
      <div className="flex flex-1 items-center gap-4 px-4">
        <Link href={"/"} aria-label="Vecrow home">
          <VecrowIcon className="hidden w-6 sm:inline" />
          <VecrowLogoWordmark className="w-28 sm:hidden" />
        </Link>
      </div>
      {navItems.map((item, index) => (
        <div
          key={index}
          className="hidden flex-1 items-center gap-4 border-l border-primary/10 px-4 md:flex"
        >
          <NavLink item={item} />
        </div>
      ))}
      <div className="flex flex-1 items-center gap-4 border-primary/10 px-4 md:border-l">
        <AppEntryButton
          variant="primary-light"
          className="ml-auto hidden px-5 md:flex"
        />
        <MobileMenuDrawer />
      </div>
    </header>
  )
}

function MobileMenuDrawer() {
  return (
    <Drawer swipeDirection="right">
      <DrawerTrigger
        render={
          <Button
            variant={"primary-light"}
            size={"icon-lg"}
            className={"ml-auto md:hidden"}
            aria-label="Open menu"
          >
            <MenuIcon aria-hidden />
          </Button>
        }
      />
      <DrawerContent>
        <DrawerHeader className="flex min-h-18 flex-row items-center justify-between border-b border-border/50 py-4 pr-4 pl-6">
          <VecrowLogoWordmark className="w-24" />
          <DrawerTitle className="sr-only">Menu</DrawerTitle>
          <DrawerClose
            render={
              <Button variant={"ghost"} size={"icon"} aria-label="Close menu">
                <XIcon aria-hidden />
              </Button>
            }
          />
        </DrawerHeader>
        <nav aria-label="Main">
          {navItems.map((item, index) => (
            <div
              key={index}
              className="flex min-h-18 flex-1 items-center gap-4 border-b border-border/50 px-4 py-2"
            >
              <NavLink item={item} />
            </div>
          ))}
        </nav>
        <DrawerFooter>
          <AppEntryButton />
        </DrawerFooter>
      </DrawerContent>
    </Drawer>
  )
}

function NavLink({ item }: { item: (typeof navItems)[number] }) {
  return (
    <Link
      href={item.href}
      {...(item.external && { target: "_blank", rel: "noreferrer" })}
      className="flex items-center gap-2 rounded-md p-2 text-sm text-nowrap transition-colors outline-none hover:text-neptune-400 focus-visible:ring-3 focus-visible:ring-ring/50"
    >
      <item.icon aria-hidden className="size-4" strokeWidth={1.5} />
      {item.label}
    </Link>
  )
}
