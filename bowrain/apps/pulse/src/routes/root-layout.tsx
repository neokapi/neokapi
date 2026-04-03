import { Outlet, useParams } from "@tanstack/react-router";
import { PulseHeader } from "@neokapi/ui/components/pulse";
import { usePulseOverview } from "../hooks/use-pulse";
import { CacheFreshness } from "../components/CacheFreshness";

export function RootLayout() {
  const params = useParams({ strict: false });
  const workspace = (params as { workspace?: string }).workspace ?? "";
  const { data } = usePulseOverview(workspace, { enabled: !!workspace });

  return (
    <div className="min-h-screen bg-background text-foreground">
      <PulseHeader
        workspaceName={data?.workspace.name ?? (workspace || "Pulse")}
        logoUrl={data?.workspace.logo_url}
      />
      <main className="mx-auto max-w-7xl px-4 py-6">
        <Outlet />
      </main>
      {workspace && (
        <footer className="mx-auto max-w-7xl px-4 pb-4 flex justify-end">
          <CacheFreshness queryKeyPrefix={["pulse", workspace]} />
        </footer>
      )}
    </div>
  );
}
