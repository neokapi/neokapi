import { Outlet, useParams } from "@tanstack/react-router";
import { PulseHeader } from "@neokapi/ui/components/pulse";
import { usePulseOverview } from "../hooks/use-pulse";

export function RootLayout() {
  const params = useParams({ strict: false });
  const workspace = (params as { workspace?: string }).workspace ?? "";
  const { data } = usePulseOverview(workspace);

  return (
    <div className="min-h-screen bg-background text-foreground">
      <PulseHeader
        workspaceName={data?.workspace.name ?? workspace}
        logoUrl={data?.workspace.logo_url}
      />
      <main className="mx-auto max-w-7xl px-4 py-6">
        <Outlet />
      </main>
    </div>
  );
}
