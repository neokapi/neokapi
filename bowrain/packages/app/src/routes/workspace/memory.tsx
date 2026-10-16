import { useEffect } from "react";
import { MemorySection, useWorkspace, Card } from "@neokapi/ui";

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
      <Card className="mt-8 max-w-md mx-auto p-8 text-center text-muted-foreground text-sm">
        Select a workspace
      </Card>
    );
  }

  return <MemorySection />;
}
