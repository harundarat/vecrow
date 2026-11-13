import { VecrowIcon } from "@/assets/vecrow-icon"
import { VecrowLogoWordmark } from "@/assets/vecrow-logo-wordmark"
import { Button } from "@/components/ui/button"
import { cn } from "cn"
import {
  BookOpenIcon,
  CircleQuestionMarkIcon,
  Ghost,
  Grid2x2Icon,
  MenuIcon,
  XIcon,
} from "lucide-react"
import {
  Drawer,
  DrawerClose,
  DrawerContent,
  DrawerDescription,
  DrawerFooter,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@/components/ui/drawer"
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
        "absolute top-2 left-1/2 z-50 flex h-20 w-[calc(100%-1rem)] -translate-x-1/2 border-b border-primary/10 px-2 sm:top-0 sm:w-[calc(100%-3rem)] sm:px-6"
      )}
    >
      <div className="flex flex-1 items-center gap-4 px-4">
        <Link href={"/"}>
          <VecrowIcon className="hidden w-6 sm:inline" />
          <VecrowLogoWordmark className="w-28 sm:hidden" />
        </Link>
      </div>
      {/*<div className="flex-2 border-l border-primary/10 px-4"></div>*/}
      {navItems.map((item, index) => (
        <div
          key={index}
          className="hidden flex-1 items-center gap-4 border-l border-primary/10 px-4 md:flex"
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
      <div className="flex flex-1 items-center gap-4 border-primary/10 px-4 md:border-l">
        <Button
          variant={"primary-light"}
          nativeButton={false}
          className={"ml-auto hidden px-5 md:flex"}
          render={<Link href={"/sign-in"}>Sign in</Link>}
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
          >
            <MenuIcon />
          </Button>
        }
      >
        Open
      </DrawerTrigger>
      <DrawerContent>
        <DrawerHeader className="flex min-h-18 flex-row items-center justify-between border-b border-border/50 py-4 pr-4 pl-6">
          <VecrowLogoWordmark className="w-24" />
          <DrawerClose
            render={
              <Button variant={"ghost"} size={"icon"}>
                <XIcon />
              </Button>
            }
          />
        </DrawerHeader>
        <div className="">
          {navItems.map((item, index) => (
            <div
              key={index}
              className="flex min-h-18 flex-1 items-center gap-4 border-b border-border/50 px-4 py-2"
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
        </div>
        <DrawerFooter>
          <Button
            nativeButton={false}
            render={<Link href={"/sign-in"}>Sign in</Link>}
          >
            Sign in
          </Button>
        </DrawerFooter>
      </DrawerContent>
    </Drawer>
  )
}
