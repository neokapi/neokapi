import { useNavigate, useParams } from "@tanstack/react-router";
import { TMExplorer, useWorkspace } from "@gokapi/ui";

export function MemoryRoute() {
  const navigate = useNavigate();
  const { workspace } = useParams({ strict: false });
  const { activeWorkspace } = useWorkspace();

  if (!activeWorkspace) {
    return (
      <div className="mt-8 p-8 text-center text-muted-foreground text-sm rounded-lg border border-dashed border-border">
        Select a workspace
      </div>
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
