import { createContext, useContext, useState, useCallback, type ReactNode } from "react";
import type { Workspace } from "../types/api";

interface WorkspaceContextValue {
  workspaces: Workspace[];
  setWorkspaces: (ws: Workspace[]) => void;
  activeWorkspace: Workspace | null;
  setActiveWorkspace: (ws: Workspace | null) => void;
}

const WorkspaceContext = createContext<WorkspaceContextValue | null>(null);

export function WorkspaceProvider({
  children,
  initialWorkspace,
  initialWorkspaces,
}: {
  children: ReactNode;
  initialWorkspace?: Workspace;
  initialWorkspaces?: Workspace[];
}) {
  const [workspaces, setWorkspaces] = useState<Workspace[]>(
    initialWorkspaces ?? (initialWorkspace ? [initialWorkspace] : []),
  );
  const [activeWorkspace, setActiveWorkspace] = useState<Workspace | null>(
    initialWorkspace ?? null,
  );

  const value: WorkspaceContextValue = {
    workspaces,
    setWorkspaces: useCallback((ws: Workspace[]) => setWorkspaces(ws), []),
    activeWorkspace,
    setActiveWorkspace: useCallback((ws: Workspace | null) => setActiveWorkspace(ws), []),
  };

  return <WorkspaceContext value={value}>{children}</WorkspaceContext>;
}

export function useWorkspace(): WorkspaceContextValue {
  const ctx = useContext(WorkspaceContext);
  if (!ctx) throw new Error("useWorkspace must be used within WorkspaceProvider");
  return ctx;
}

/**
 * The workspace when there is one, null when there is not.
 *
 * For a component that works either way and only NEEDS the workspace to reach a
 * workspace-scoped route: the desktop app and the component stories mount the
 * same views with no provider around them, and throwing there would make a
 * feature that simply is not offered look like a crash. A caller that cannot
 * proceed without a workspace still uses `useWorkspace` and gets the error.
 */
export function useOptionalWorkspace(): WorkspaceContextValue | null {
  return useContext(WorkspaceContext);
}
