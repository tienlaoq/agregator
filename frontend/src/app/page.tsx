"use client"

import { HeroSection } from "@/components/banya/hero-section"
import { FeaturesSection } from "@/components/banya/features-section"
import { PopularVenuesSection } from "@/components/banya/popular-venues-section"
import { CTASection } from "@/components/banya/cta-section"

export default function HomePage() {
  return (
    <>
      <HeroSection />
      <FeaturesSection />
      <PopularVenuesSection />
    </>
  )
}
