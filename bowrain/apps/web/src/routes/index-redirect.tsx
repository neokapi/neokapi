import { useEffect, useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import { useApi, useAuth, useWorkspace, type ConfigResponse, type Workspace } from "@gokapi/ui";
import { useUIStore } from "../stores/ui-store";

/**
 * Index route at `/` — resolves the workspace and redirects.
 *
 * In standalone mode: redirects to `/local`.
 * In server mode: authenticates, loads workspaces, redirects to the
 * last-used workspace or the first available one.
 */
export function IndexRedirect() {
  const navigate = useNavigate();
  const adapter = useApi();
  const { setUser } = useAuth();
  const { setWorkspaces, setActiveWorkspace } = useWorkspace();
  const lastWorkspaceSlug = useUIStore((s) => s.lastWorkspaceSlug);
  const setLastWorkspaceSlug = useUIStore((s) => s.setLastWorkspaceSlug);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    (async () => {
      try {
        const config: ConfigResponse = await adapter.getConfig();

        if (config.mode === "standalone") {
          const localWs: Workspace = {
            id: "local", name: "Local", slug: "local",
            description: "", logo_url: "", type: "personal", role: "owner",
          };
          setUser({ id: "local", email: "", name: "Local User", avatar_url: "" });
          setWorkspaces([localWs]);
          setActiveWorkspace(localWs);
          navigate({ to: "/$workspace", params: { workspace: "local" }, replace: true });
          return;
        }

        // Server mode — authenticate.
        const currentUser = await adapter.getCurrentUser();
        if (!currentUser) {
          window.location.href = "/api/v1/auth/login";
          return;
        }
        setUser(currentUser);
        const ws = (await adapter.listWorkspaces()) || [];
        setWorkspaces(ws);

        if (ws.length === 0) {
          setError("No workspaces available. Please contact your administrator.");
          return;
        }

        // Prefer the last-used workspace if it still exists.
        const target = (lastWorkspaceSlug && ws.find((w) => w.slug === lastWorkspaceSlug)) || ws[0];
        setActiveWorkspace(target);
        setLastWorkspaceSlug(target.slug);
        navigate({ to: "/$workspace", params: { workspace: target.slug }, replace: true });
      } catch (e) {
        console.error("Failed to initialize:", e);
        setError("Failed to load. Please refresh the page.");
      }
    })();
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  if (error) {
    return (
      <div className="flex items-center justify-center h-screen bg-background text-muted-foreground text-sm">
        {error}
      </div>
    );
  }

  return (
    <div className="flex items-center justify-center h-screen bg-background text-muted-foreground">
      Loading...
    </div>
  );
}
