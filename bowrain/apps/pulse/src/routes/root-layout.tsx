import { Outlet, useParams } from "@tanstack/react-router";
import { PageContainer } from "@neokapi/ui";
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
      <main>
        <PageContainer width="wide">
          <Outlet />
        </PageContainer>
      </main>
      {workspace && (
        <footer>
          <PageContainer width="wide" className="flex justify-end pt-0 pb-4">
            <CacheFreshness queryKeyPrefix={["pulse", workspace]} />
          </PageContainer>
        </footer>
      )}
    </div>
  );
}
