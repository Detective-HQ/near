import { BentoGrid } from "./bento-grid";
import { ChainSlider } from "./chain-slider";
import { Faq } from "./faq";
import { FinalCta } from "./final-cta";
import { HowItWorks } from "./how-it-works";
import { ModulesGrid } from "./modules-grid";
import { Principles } from "./principles";
import { Ribbons } from "./ribbons";
import { SiteFooter } from "./site-footer";
import { TechGrid } from "./tech-grid";
import { GardenAmbience } from "./foliage";
import "./landing.css";

/** Everything below the hero. Server-rendered; no client JavaScript. */
export function LandingSections() {
  return (
    <div className="lp">
      <GardenAmbience />
      <ChainSlider />
      <BentoGrid />
      <Ribbons />
      <HowItWorks />
      <ModulesGrid />
      <TechGrid />
      <Principles />
      <Faq />
      <FinalCta />
      <SiteFooter />
    </div>
  );
}
