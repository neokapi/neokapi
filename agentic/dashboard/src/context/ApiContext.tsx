import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import {
  fetchWorkspaces,
  fetchProjects,
  fetchAuditLog,
  fetchMembers,
  fetchTranslationProgress,
  fetchAgents,
  fetchExecutions,
  fetchGitHubIssues,
  type Workspace,
  type Project,
  type AuditEntry,
  type Member,
  type TranslationProgress,
  type AgentProfile,
  type Execution,
  type GitHubIssue,
} from "@/lib/api";

export type { AgentProfile, Execution, GitHubIssue };

export interface ApiState {
  workspaces: Workspace[];
  projects: Project[];
  auditLog: AuditEntry[];
  members: Member[];
  progress: TranslationProgress[];
  agents: AgentProfile[];
  executions: Execution[];
  issues: GitHubIssue[];
  loading: boolean;
  connected: boolean;
}

const ApiContext = createContext<ApiState>({
  workspaces: [],
  projects: [],
  auditLog: [],
  members: [],
  progress: [],
  agents: [],
  executions: [],
  issues: [],
  loading: true,
  connected: false,
});

export function ApiProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<ApiState>({
    workspaces: [],
    projects: [],
    auditLog: [],
    members: [],
    progress: [],
    agents: [],
    executions: [],
    issues: [],
    loading: true,
    connected: false,
  });

  useEffect(() => {
    let cancelled = false;

    async function load() {
      // Fetch agentic data (from PostgreSQL execution store) in parallel with Bowrain data.
      const [workspaces, agents, executions, issues] = await Promise.all([
        fetchWorkspaces(),
        fetchAgents(),
        fetchExecutions({ limit: 200 }),
        fetchGitHubIssues("all", 20),
      ]);
      if (cancelled) return;

      const connected = workspaces.length > 0 || agents.length > 0;

      if (workspaces.length === 0) {
        setState((s) => ({
          ...s,
          loading: false,
          connected,
          agents,
          executions,
          issues,
        }));
        return;
      }

      // Use the first active workspace for Bowrain content data.
      const ws = workspaces[0];
      const [projects, auditLog, members] = await Promise.all([
        fetchProjects(ws.slug),
        fetchAuditLog(ws.slug, 1000),
        fetchMembers(ws.slug),
      ]);
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
        members,
        progress,
        agents,
        executions,
        issues,
        loading: false,
        connected: true,
      });
    }

    load();

    // Refresh every 30 seconds.
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

/**
 * Derive "audit event sessions" by grouping consecutive events from the
 * same actor within a time window. Returns groups sorted descending by
 * first event timestamp.
 */
export interface AuditSession {
  id: string;
  actor: string;
  events: AuditEntry[];
  startTime: string;
  endTime: string;
  eventCount: number;
}

const SESSION_GAP_MS = 5 * 60_000; // 5 minute gap = new session

export function groupAuditSessions(auditLog: AuditEntry[]): AuditSession[] {
  if (auditLog.length === 0) return [];

  // Sort ascending by time
  const sorted = [...auditLog].sort(
    (a, b) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime(),
  );

  const sessions: AuditSession[] = [];
  let current: AuditEntry[] = [sorted[0]];
  let currentActor = sorted[0].actor || "unknown";

  for (let i = 1; i < sorted.length; i++) {
    const entry = sorted[i];
    const actor = entry.actor || "unknown";
    const prevTime = new Date(current[current.length - 1].created_at).getTime();
    const thisTime = new Date(entry.created_at).getTime();

    if (actor === currentActor && thisTime - prevTime < SESSION_GAP_MS) {
      current.push(entry);
    } else {
      sessions.push({
        id: `session-${sessions.length}`,
        actor: currentActor,
        events: current,
        startTime: current[0].created_at,
        endTime: current[current.length - 1].created_at,
        eventCount: current.length,
      });
      current = [entry];
      currentActor = actor;
    }
  }

  sessions.push({
    id: `session-${sessions.length}`,
    actor: currentActor,
    events: current,
    startTime: current[0].created_at,
    endTime: current[current.length - 1].created_at,
    eventCount: current.length,
  });

  return sessions.reverse();
}
