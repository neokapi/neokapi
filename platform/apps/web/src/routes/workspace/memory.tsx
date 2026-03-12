import { useEffect } from "react";
import { useNavigate, useParams } from "@tanstack/react-router";
import { TMExplorer, useWorkspace, GlassCard } from "@gokapi/ui";

export function MemoryRoute() {
  const navigate = useNavigate();
  const { workspace } = useParams({ strict: false });
  const { activeWorkspace } = useWorkspace();

  useEffect(() => {
    if (activeWorkspace) {
      document.title = `Translation Memory — ${activeWorkspace.name} — Bowrain`;
    }
  }, [activeWorkspace]);

  if (!activeWorkspace) {
    return (
      <GlassCard intensity="subtle" className="mt-8 max-w-md mx-auto p-8 text-center text-muted-foreground text-sm">
        Select a workspace
      </GlassCard>
    );
  }

  return (
    <TMExplorer
      sourceLocale=""
      targetLocales={[]}
      onBack={() => navigate({ to: "/$workspace", params: { workspace: workspace ?? "" } })}
    />
  );
}
