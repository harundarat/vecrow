"use client"
import { HeroSection } from "./_components/sections/hero-section"
import { HowItWorksSection } from "./_components/sections/how-it-works-section"
import { SeeItInActionSection } from "./_components/sections/see-it-in-action-section"

export default function Page() {
  return (
    <>
      <HeroSection />
      <HowItWorksSection />
      <SeeItInActionSection />
    </>
  )
}
