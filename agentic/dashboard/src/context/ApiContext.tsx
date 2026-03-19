import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import {
  fetchWorkspaces,
  fetchProjects,
  fetchAuditLog,
  fetchTranslationProgress,
  type Workspace,
  type Project,
  type AuditEntry,
  type TranslationProgress,
} from "@/lib/api";

interface ApiState {
  workspaces: Workspace[];
  projects: Project[];
  auditLog: AuditEntry[];
  progress: TranslationProgress[];
  loading: boolean;
  connected: boolean;
}

const ApiContext = createContext<ApiState>({
  workspaces: [],
  projects: [],
  auditLog: [],
  progress: [],
  loading: true,
  connected: false,
});

export function ApiProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<ApiState>({
    workspaces: [],
    projects: [],
    auditLog: [],
    progress: [],
    loading: true,
    connected: false,
  });

  useEffect(() => {
    let cancelled = false;

    async function load() {
      const workspaces = await fetchWorkspaces();
      if (cancelled) return;

      if (workspaces.length === 0) {
        setState((s) => ({ ...s, loading: false, connected: false }));
        return;
      }

      // Use the first active workspace
      const ws = workspaces[0];
      const projects = await fetchProjects(ws.slug);
      if (cancelled) return;

      const auditLog = await fetchAuditLog(ws.slug, 200);
      if (cancelled) return;

      let progress: TranslationProgress[] = [];
      if (projects.length > 0) {
        progress = await fetchTranslationProgress(
          ws.slug,
          projects[0].id,
          projects[0].target_languages,
        );
      }
      if (cancelled) return;

      setState({
        workspaces,
        projects,
        auditLog,
        progress,
        loading: false,
        connected: true,
      });
    }

    load();

    // Refresh every 30 seconds
    const interval = setInterval(load, 30_000);
    return () => {
      cancelled = true;
      clearInterval(interval);
    };
  }, []);

  return <ApiContext.Provider value={state}>{children}</ApiContext.Provider>;
}

export function useApi() {
  return useContext(ApiContext);
}
