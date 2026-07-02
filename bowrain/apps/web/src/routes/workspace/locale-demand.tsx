import { useEffect } from "react";
import { useRouteContext } from "@tanstack/react-router";
import type { WorkspaceRouteContext } from "..";
import { LocaleDemandView } from "../../locale-demand/LocaleDemandView";

/**
 * Locale demand — DESIGN PROTOTYPE (mock data only, no plan-tier gating).
 * The whole surface lives in `src/locale-demand/`; this route only wires the
 * workspace context (document title) around the router-free view.
 */
export function LocaleDemandRoute() {
  const { activeWorkspace } = useRouteContext({ strict: false }) as WorkspaceRouteContext;

  useEffect(() => {
    document.title = `Locale demand — ${activeWorkspace.name} — Bowrain`;
  }, [activeWorkspace.name]);

  return <LocaleDemandView />;
}
