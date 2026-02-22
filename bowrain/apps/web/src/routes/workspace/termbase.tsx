import { useNavigate, useParams } from "@tanstack/react-router";
import { TermExplorer, useWorkspace, GlassCard, CardContent } from "@gokapi/ui";

export function TermbaseRoute() {
  const navigate = useNavigate();
  const { workspace } = useParams({ strict: false });
  const { activeWorkspace } = useWorkspace();

  if (!activeWorkspace) {
    return (
      <GlassCard intensity="subtle" className="mt-8 max-w-md mx-auto">
        <CardContent className="p-8 text-center text-muted-foreground text-sm">
          Select a workspace
        </CardContent>
      </GlassCard>
    );
  }

  return (
    <TermExplorer
      sourceLocale=""
      targetLocales={[]}
      onBack={() => navigate({ to: "/$workspace", params: { workspace: workspace ?? "" } })}
    />
  );
}
