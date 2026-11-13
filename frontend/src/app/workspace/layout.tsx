import type { ReactNode } from "react";
import { Suspense } from "react";
import { WorkspaceShell } from "../../components/workspace/shell";
import { Skeleton } from "../../components/workspace/ui";
import "./workspace.css";

export default function WorkspaceLayout({ children }: Readonly<{ children: ReactNode }>) {
  return (
    <Suspense fallback={null}>
      <WorkspaceShell>
        <Suspense fallback={<Skeleton rows={6} />}>{children}</Suspense>
      </WorkspaceShell>
    </Suspense>
  );
}
