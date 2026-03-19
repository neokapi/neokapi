/**
 * Bowrain API client for the agentic dashboard.
 *
 * Fetches real data from the Bowrain REST API. Falls back to empty results
 * when the server is unreachable (e.g. during static deployment).
 */

// In dev mode, Vite proxies /api → localhost:8080 so no base URL needed.
// In production, set VITE_BOWRAIN_API_URL to the actual server URL.
const API_BASE = import.meta.env.VITE_BOWRAIN_API_URL || "";
const API_TOKEN = import.meta.env.VITE_BOWRAIN_TOKEN || "";

async function apiFetch<T>(path: string, fallback: T): Promise<T> {
  try {
    const headers: Record<string, string> = { "Content-Type": "application/json" };
    if (API_TOKEN) {
      headers["Authorization"] = `Bearer ${API_TOKEN}`;
    }
    const resp = await fetch(`${API_BASE}${path}`, { headers });
    if (!resp.ok) return fallback;
    return (await resp.json()) as T;
  } catch {
    return fallback;
  }
}

// --- Types matching Bowrain API responses ---

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

// --- API calls ---

export async function fetchWorkspaces(): Promise<Workspace[]> {
  return apiFetch("/api/v1/workspaces", []);
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
  return apiFetch(
    `/api/v1/workspaces/${wsSlug}/projects/${projectId}/sync/blocks?${params}`,
    [],
  );
}

export interface TranslationProgress {
  locale: string;
  translated: number;
  total: number;
}

/**
 * Compute translation progress by fetching all blocks and counting targets.
 */
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
