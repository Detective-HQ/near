import { notFound } from "next/navigation";
import { WorkspaceShell } from "../../../components/workspace-shell";
import type { SectionKey } from "../../../components/workspace-shell";

const sections = new Set<SectionKey>([
  "overview",
  "swap",
  "activity",
  "portfolio",
  "markets",
  "intelligence",
  "research",
  "risk"
]);

export default async function WorkspaceSectionPage({
  params
}: {
  params: Promise<{ section: string }>;
}) {
  const { section } = await params;
  if (!sections.has(section as SectionKey)) notFound();
  return <WorkspaceShell initialSection={section as SectionKey} />;
}
