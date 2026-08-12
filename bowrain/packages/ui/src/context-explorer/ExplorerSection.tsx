// ExplorerSection — the context explorer inside the Context hub.
//
// It frames @neokapi/context-explorer in the hub's page shell and drives it
// with a source built over the workspace's ApiAdapter. The same components run
// in Kapi Desktop against one project on disk; the only difference is which
// dimensions the mount pins. Here it pins the workspace and nothing else, so
// the ladder starts above the projects and "which projects use this concept"
// is a question the surface can actually answer.
import { useMemo } from "react";
import { ContextExplorer, ScopeProvider } from "@neokapi/context-explorer";
import type { ContextDataSource, Dimension, ScopeTuple } from "@neokapi/context-explorer";
import { useApi } from "../context/ApiContext";
import { useWorkspace } from "../context/WorkspaceContext";
import { BrandHub } from "../brand-hub/shell/BrandHub";
import { createRestContextSource } from "./restContextSource";

/** The workspace is where the reader already is; everything below it moves. */
const PINNED: Dimension[] = ["workspace"];

export interface ExplorerSectionProps {
  /** The point to open at, when a link carried one. */
  scope?: ScopeTuple;
  /** Mirror the tuple into the URL, so a point can be linked to. */
  onScopeChange?: (next: ScopeTuple) => void;
  /** Injected in tests and stories; production reads the workspace API. */
  source?: ContextDataSource;
}

export function ExplorerSection({ scope, onScopeChange, source }: ExplorerSectionProps) {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";
  const client = useMemo(() => source ?? createRestContextSource(api, ws), [source, api, ws]);
  const at = useMemo<ScopeTuple>(() => ({ workspace: ws, ...scope }), [ws, scope]);

  return (
    <BrandHub
      title="Explorer"
      description="Stand at a point in the workspace and ask what governs there, what lives there, and how it relates."
      width="wide"
    >
      <ScopeProvider source={client} pinned={PINNED} scope={at} onScopeChange={onScopeChange}>
        <ContextExplorer />
      </ScopeProvider>
    </BrandHub>
  );
}
