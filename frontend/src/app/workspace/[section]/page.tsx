import { notFound, redirect } from "next/navigation";
import type { ComponentType } from "react";
import { ActivityPage } from "../../../components/workspace/pages/activity";
import { ExplorePage } from "../../../components/workspace/pages/explore";
import { IntelligencePage } from "../../../components/workspace/pages/intelligence";
import { MarketsPage } from "../../../components/workspace/pages/markets";
import { PortfolioPage } from "../../../components/workspace/pages/portfolio";
import { ResearchPage } from "../../../components/workspace/pages/research";
import { RiskPage } from "../../../components/workspace/pages/risk";
import { SwapPage } from "../../../components/workspace/pages/swap";

const PAGES: Record<string, ComponentType> = {
  explore: ExplorePage,
  swap: SwapPage,
  activity: ActivityPage,
  portfolio: PortfolioPage,
  markets: MarketsPage,
  intelligence: IntelligencePage,
  research: ResearchPage,
  risk: RiskPage,
};

export default async function WorkspaceSectionPage({ params }: { params: Promise<{ section: string }> }) {
  const { section } = await params;
  // "overview" was the old landing section; Explore replaces it.
  if (section === "overview") redirect("/workspace/explore");
  const Page = PAGES[section];
  if (!Page) notFound();
  return <Page />;
}
