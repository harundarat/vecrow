"use client"
import { HeroSection } from "./_components/sections/hero-section"
import { HowItWorksSection } from "./_components/sections/how-it-works-section"
import { SeeItInActionSection } from "./_components/sections/see-it-in-action-section"
import { FaqSection } from "./_components/sections/faq-section"
import { FooterSection } from "./_components/sections/footer-section"

export default function Page() {
  return (
    <>
      <HeroSection />
      <HowItWorksSection />
      <SeeItInActionSection />
      <FaqSection />
      <FooterSection />
    </>
  )
}
