import { useEffect } from "react";
import { MemorySection, useWorkspace, Card, PageContainer } from "@neokapi/ui";

/**
 * MemoryRoute is the workspace's content memory as a list of pairs beside a
 * detail pane. The section reads the workspace's languages off its projects,
 * so the route only names the page.
 */
export function MemoryRoute() {
  const { activeWorkspace } = useWorkspace();

  useEffect(() => {
    if (activeWorkspace) {
      document.title = `Content memory · ${activeWorkspace.name} · Bowrain`;
    }
  }, [activeWorkspace]);

  if (!activeWorkspace) {
    return (
      <PageContainer width="narrow">
        <Card className="mt-8 p-8 text-center text-muted-foreground text-sm">
          Select a workspace
        </Card>
      </PageContainer>
    );
  }

  return (
    <PageContainer width="full" className="px-6 py-5">
      <MemorySection />
    </PageContainer>
  );
}
