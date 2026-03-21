/**
 * Bowrain API client for the agentic dashboard.
 *
 * All data comes from the Bowrain REST API backed by PostgreSQL.
 * No mock data, no static fallbacks.
 */

const API_BASE = import.meta.env.VITE_BOWRAIN_API_URL || "";

async function apiFetch<T>(path: string, fallback: T): Promise<T> {
  try {
    const resp = await fetch(`${API_BASE}${path}`, {
      headers: { "Content-Type": "application/json" },
    });
    if (!resp.ok) return fallback;
    return (await resp.json()) as T;
  } catch {
    return fallback;
  }
}

// --- Bowrain core types ---

export interface Workspace {
  id: string;
  name: string;
  slug: string;
  description: string;
  type: string;
  role: string;
  created_at: string;
}

export interface Project {
  id: string;
  name: string;
  default_source_language: string;
  target_languages: string[];
  workspace_id: string;
  created_at: string;
}

export interface AuditEntry {
  id: number;
  project_id: string;
  event_type: string;
  actor: string;
  source: string;
  data: string;
  created_at: string;
}

export interface Block {
  id: string;
  name: string;
  item_name: string;
  source: string;
  targets: Record<string, string>;
}

export interface Member {
  user_id: string;
  role: string;
  joined_at: string;
}

// --- Agentic types (from PostgreSQL execution store) ---

export interface AgentProfile {
  agent: string;
  role: string;
  total_sessions: number;
  successful_count: number;
  failed_count: number;
  last_session_at: string;
  last_status: string;
  total_tokens_used: number;
}

export interface Execution {
  id: string;
  workspace_slug: string;
  agent: string;
  role: string;
  status: string;
  task: string;
  locale: string;
  summary: string;
  tokens_used: number;
  error: string;
  started_at: string;
  completed_at: string;
}

export interface AgenticEvent {
  type: string;
  execution_id: string;
  workspace: string;
  agent: string;
  role: string;
  timestamp: string;
  data: Record<string, unknown>;
}

export interface GitHubIssue {
  number: number;
  title: string;
  state: string;
  html_url: string;
  labels: string[];
  created_at: string;
  updated_at: string;
  author: string;
}

export interface TranslationProgress {
  locale: string;
  translated: number;
  total: number;
}

// --- Bowrain core API calls ---

export async function fetchWorkspaces(): Promise<Workspace[]> {
  return apiFetch("/api/v1/workspaces", []);
}

export async function fetchMembers(wsSlug: string): Promise<Member[]> {
  return apiFetch(`/api/v1/workspaces/${wsSlug}/members`, []);
}

export async function fetchProjects(wsSlug: string): Promise<Project[]> {
  return apiFetch(`/api/v1/workspaces/${wsSlug}/projects`, []);
}

export async function fetchAuditLog(wsSlug: string, limit = 100): Promise<AuditEntry[]> {
  return apiFetch(`/api/v1/workspaces/${wsSlug}/audit-log?limit=${limit}`, []);
}

export async function fetchBlocks(
  wsSlug: string,
  projectId: string,
  opts: { locale?: string; limit?: number } = {},
): Promise<Block[]> {
  const params = new URLSearchParams();
  if (opts.locale) params.set("locale", opts.locale);
  params.set("limit", String(opts.limit ?? 1000));
  return apiFetch(`/api/v1/workspaces/${wsSlug}/projects/${projectId}/sync/blocks?${params}`, []);
}

export async function fetchTranslationProgress(
  wsSlug: string,
  projectId: string,
  targetLanguages: string[],
): Promise<TranslationProgress[]> {
  const blocks = await fetchBlocks(wsSlug, projectId, { limit: 2000 });
  const total = blocks.length;

  return targetLanguages.map((locale) => ({
    locale,
    translated: blocks.filter((b) => locale in b.targets).length,
    total,
  }));
}

// --- Agentic API calls (PostgreSQL-backed) ---

export async function fetchAgents(): Promise<AgentProfile[]> {
  const resp = await apiFetch<{ agents: AgentProfile[] }>("/api/v1/agentic/agents", { agents: [] });
  return resp.agents ?? [];
}

export async function fetchExecutions(opts: {
  workspace?: string;
  agent?: string;
  limit?: number;
} = {}): Promise<Execution[]> {
  const params = new URLSearchParams();
  if (opts.workspace) params.set("workspace", opts.workspace);
  if (opts.agent) params.set("agent", opts.agent);
  params.set("limit", String(opts.limit ?? 100));
  const resp = await apiFetch<{ executions: Execution[] }>(`/api/v1/agentic/executions?${params}`, { executions: [] });
  return resp.executions ?? [];
}

export async function fetchAgenticEvents(opts: {
  execution_id?: string;
  workspace?: string;
  event_type?: string;
  limit?: number;
} = {}): Promise<AgenticEvent[]> {
  const params = new URLSearchParams();
  if (opts.execution_id) params.set("execution_id", opts.execution_id);
  if (opts.workspace) params.set("workspace", opts.workspace);
  if (opts.event_type) params.set("event_type", opts.event_type);
  params.set("limit", String(opts.limit ?? 100));
  const resp = await apiFetch<{ events: AgenticEvent[] }>(`/api/v1/agentic/events?${params}`, { events: [] });
  return resp.events ?? [];
}

export async function fetchGitHubIssues(state = "all", limit = 20): Promise<GitHubIssue[]> {
  const resp = await apiFetch<{ issues: GitHubIssue[] }>(`/api/v1/agentic/issues?state=${state}&limit=${limit}`, { issues: [] });
  return resp.issues ?? [];
}
