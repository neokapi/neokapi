import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import {
  fetchWorkspaces,
  fetchProjects,
  fetchAuditLog,
  fetchMembers,
  fetchTranslationProgress,
  type Workspace,
  type Project,
  type AuditEntry,
  type Member,
  type TranslationProgress,
} from "@/lib/api";
import { agentMeta, type AgentMeta } from "@/data/agent-meta";

/** A fully-resolved agent profile: API member data merged with static persona metadata. */
export interface AgentProfile {
  id: string; // user_id from API
  displayName: string;
  role: string;
  model: string;
  schedule: string;
  targetLanguage?: string;
  accentColor: string;
  personality: string[];
  avatar: string;
  joinedAt: string;
  /** Derived: most recent audit event timestamp for this actor */
  lastActive: string | null;
  /** Derived: number of audit events from this actor in the current day */
  eventsToday: number;
  /** Derived: number of audit events from this actor in the last 7 days */
  eventsThisWeek: number;
}

export interface ApiState {
  workspaces: Workspace[];
  projects: Project[];
  auditLog: AuditEntry[];
  members: Member[];
  progress: TranslationProgress[];
  agents: AgentProfile[];
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
  loading: true,
  connected: false,
});

function buildAgentProfiles(members: Member[], auditLog: AuditEntry[]): AgentProfile[] {
  const now = Date.now();
  const dayMs = 24 * 3_600_000;
  const weekMs = 7 * dayMs;

  // Index audit entries by actor
  const byActor = new Map<string, AuditEntry[]>();
  for (const entry of auditLog) {
    if (!entry.actor) continue;
    const existing = byActor.get(entry.actor) ?? [];
    existing.push(entry);
    byActor.set(entry.actor, existing);
  }

  // Build a profile for each known agent in agentMeta.
  // If members endpoint returns data, only include agents that are actual members.
  // If members endpoint is empty (API not populated), show all from agentMeta.
  const metaMap = new Map<string, AgentMeta>();
  for (const m of agentMeta) {
    metaMap.set(m.userId, m);
  }

  const memberIds =
    members.length > 0
      ? new Set(members.map((m) => m.user_id))
      : new Set(agentMeta.map((m) => m.userId));

  const profiles: AgentProfile[] = [];
  for (const userId of memberIds) {
    const meta = metaMap.get(userId);
    if (!meta) continue; // Skip members we don't have metadata for

    const member = members.find((m) => m.user_id === userId);
    const entries = byActor.get(userId) ?? [];

    // Sort entries by created_at descending
    const sorted = [...entries].sort(
      (a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime(),
    );
    const lastActive = sorted.length > 0 ? sorted[0].created_at : null;
    const eventsToday = entries.filter(
      (e) => now - new Date(e.created_at).getTime() < dayMs,
    ).length;
    const eventsThisWeek = entries.filter(
      (e) => now - new Date(e.created_at).getTime() < weekMs,
    ).length;

    profiles.push({
      id: userId,
      displayName: meta.displayName,
      role: meta.role,
      model: meta.model,
      schedule: meta.schedule,
      targetLanguage: meta.targetLanguage,
      accentColor: meta.accentColor,
      personality: meta.personality,
      avatar: meta.avatar,
      joinedAt: member?.joined_at ?? "",
      lastActive,
      eventsToday,
      eventsThisWeek,
    });
  }

  return profiles;
}

export function ApiProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<ApiState>({
    workspaces: [],
    projects: [],
    auditLog: [],
    members: [],
    progress: [],
    agents: [],
    loading: true,
    connected: false,
  });

  useEffect(() => {
    let cancelled = false;

    async function load() {
      const workspaces = await fetchWorkspaces();
      if (cancelled) return;

      if (workspaces.length === 0) {
        // Not connected -- build agent profiles from metadata alone (no API data)
        const agents = buildAgentProfiles([], []);
        setState((s) => ({ ...s, loading: false, connected: false, agents }));
        return;
      }

      // Use the first active workspace
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

      const agents = buildAgentProfiles(members, auditLog);

      setState({
        workspaces,
        projects,
        auditLog,
        members,
        progress,
        agents,
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
      // Close previous session
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

  // Close last session
  sessions.push({
    id: `session-${sessions.length}`,
    actor: currentActor,
    events: current,
    startTime: current[0].created_at,
    endTime: current[current.length - 1].created_at,
    eventCount: current.length,
  });

  // Return descending by start time
  return sessions.reverse();
}
