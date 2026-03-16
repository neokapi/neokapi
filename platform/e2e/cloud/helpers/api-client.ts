/**
 * Bowrain API client for cloud e2e tests.
 * Operates against any live server via authenticated HTTP requests.
 */

export class BowrainAPI {
  private baseUrl: string;
  private token: string;

  constructor(baseUrl: string, token: string) {
    this.baseUrl = baseUrl;
    this.token = token;
  }

  get apiUrl(): string {
    return `${this.baseUrl}/api/v1`;
  }

  private async get<T = unknown>(path: string): Promise<T> {
    const resp = await fetch(`${this.apiUrl}${path}`, {
      headers: { Authorization: `Bearer ${this.token}` },
    });
    if (!resp.ok) throw new Error(`GET ${path}: ${resp.status} ${await resp.text()}`);
    return resp.json() as Promise<T>;
  }

  private async post<T = unknown>(path: string, body?: unknown): Promise<T> {
    const resp = await fetch(`${this.apiUrl}${path}`, {
      method: "POST",
      headers: {
        Authorization: `Bearer ${this.token}`,
        "Content-Type": "application/json",
      },
      body: body ? JSON.stringify(body) : undefined,
    });
    if (!resp.ok) throw new Error(`POST ${path}: ${resp.status} ${await resp.text()}`);
    if (resp.status === 204) return null as T;
    return resp.json() as Promise<T>;
  }

  private async put<T = unknown>(path: string, body?: unknown): Promise<T> {
    const resp = await fetch(`${this.apiUrl}${path}`, {
      method: "PUT",
      headers: {
        Authorization: `Bearer ${this.token}`,
        "Content-Type": "application/json",
      },
      body: body ? JSON.stringify(body) : undefined,
    });
    if (!resp.ok) throw new Error(`PUT ${path}: ${resp.status} ${await resp.text()}`);
    if (resp.status === 204) return null as T;
    return resp.json() as Promise<T>;
  }

  private async delete(path: string): Promise<void> {
    const resp = await fetch(`${this.apiUrl}${path}`, {
      method: "DELETE",
      headers: { Authorization: `Bearer ${this.token}` },
    });
    if (!resp.ok && resp.status !== 204) {
      throw new Error(`DELETE ${path}: ${resp.status} ${await resp.text()}`);
    }
  }

  // --- Health ---

  async health(): Promise<{ status: string }> {
    const resp = await fetch(`${this.apiUrl}/health`);
    return resp.json();
  }

  async ready(): Promise<ReadinessInfo> {
    const resp = await fetch(`${this.apiUrl}/ready`);
    return resp.json();
  }

  async info(): Promise<{ version: string; commit: string; build_date: string }> {
    const resp = await fetch(`${this.apiUrl}/info`);
    return resp.json();
  }

  // --- Auth ---

  async me(): Promise<{ email: string; name: string; id: string }> {
    return this.get("/auth/me");
  }

  // --- Workspaces ---

  async createWorkspace(name: string, slug: string): Promise<Workspace> {
    return this.post("/workspaces", { name, slug });
  }

  async getWorkspace(slug: string): Promise<Workspace> {
    return this.get(`/workspaces/${slug}`);
  }

  async listWorkspaces(): Promise<Workspace[]> {
    return this.get("/workspaces");
  }

  async getOrCreateWorkspace(name: string, slug: string): Promise<Workspace> {
    try {
      return await this.getWorkspace(slug);
    } catch {
      return this.createWorkspace(name, slug);
    }
  }

  // --- Editor Projects ---

  async createProject(
    wsSlug: string,
    name: string,
    sourceLanguage: string,
    targetLanguages: string[],
  ): Promise<Project> {
    return this.post(`/workspaces/${wsSlug}/editor/projects`, {
      name,
      default_source_language: sourceLanguage,
      target_languages: targetLanguages,
    });
  }

  async getProject(wsSlug: string, projectId: string): Promise<Project> {
    return this.get(`/workspaces/${wsSlug}/editor/projects/${projectId}`);
  }

  async listProjects(wsSlug: string): Promise<Project[]> {
    return this.get(`/workspaces/${wsSlug}/editor/projects`);
  }

  async deleteProject(wsSlug: string, projectId: string): Promise<void> {
    return this.delete(`/workspaces/${wsSlug}/editor/projects/${projectId}`);
  }

  async deleteAllProjects(wsSlug: string): Promise<void> {
    const projects = await this.listProjects(wsSlug);
    for (const p of projects) {
      await this.deleteProject(wsSlug, p.id);
    }
  }

  // --- File Operations ---

  async uploadFile(wsSlug: string, projectId: string, fileName: string, content: string): Promise<void> {
    const formData = new FormData();
    formData.append("files", new Blob([content]), fileName);

    const resp = await fetch(
      `${this.apiUrl}/workspaces/${wsSlug}/editor/projects/${projectId}/files`,
      {
        method: "POST",
        headers: { Authorization: `Bearer ${this.token}` },
        body: formData,
      },
    );
    if (!resp.ok) throw new Error(`Upload ${fileName}: ${resp.status} ${await resp.text()}`);
  }

  async pseudoTranslate(
    wsSlug: string,
    projectId: string,
    fileName: string,
    targetLocale: string,
  ): Promise<{ total_blocks: number; translated_blocks: number }> {
    return this.post(
      `/workspaces/${wsSlug}/editor/projects/${projectId}/file-pseudo/${encodeURIComponent(fileName)}`,
      { target_locale: targetLocale },
    );
  }

  // --- TM ---

  async addTMEntry(
    wsSlug: string,
    source: string,
    target: string,
    sourceLocale: string,
    targetLocale: string,
  ): Promise<unknown> {
    return this.post(`/workspaces/${wsSlug}/tm`, {
      source,
      target,
      source_locale: sourceLocale,
      target_locale: targetLocale,
    });
  }

  async searchTM(wsSlug: string, query: string): Promise<unknown> {
    return this.get(`/workspaces/${wsSlug}/tm?q=${encodeURIComponent(query)}`);
  }

  // --- Terminology ---

  async addConcept(wsSlug: string, concept: Concept): Promise<unknown> {
    return this.post(`/workspaces/${wsSlug}/terms`, concept);
  }

  async searchTerms(wsSlug: string, query: string): Promise<unknown> {
    return this.get(`/workspaces/${wsSlug}/terms?q=${encodeURIComponent(query)}`);
  }

  // --- Automations ---

  async listAutomationRules(wsSlug: string, projectId: string): Promise<AutomationRule[]> {
    return this.get(`/workspaces/${wsSlug}/projects/${projectId}/automations`);
  }

  async createAutomationRule(
    wsSlug: string,
    projectId: string,
    rule: Partial<AutomationRule>,
  ): Promise<AutomationRule> {
    return this.post(`/workspaces/${wsSlug}/projects/${projectId}/automations`, rule);
  }

  async updateAutomationRule(
    wsSlug: string,
    projectId: string,
    ruleId: string,
    rule: Partial<AutomationRule>,
  ): Promise<AutomationRule> {
    return this.put(
      `/workspaces/${wsSlug}/projects/${projectId}/automations/${ruleId}`,
      rule,
    );
  }

  async deleteAutomationRule(
    wsSlug: string,
    projectId: string,
    ruleId: string,
  ): Promise<void> {
    return this.delete(`/workspaces/${wsSlug}/projects/${projectId}/automations/${ruleId}`);
  }

  // --- Formats & Tools ---

  async listFormats(): Promise<unknown[]> {
    return this.get("/formats");
  }

  async listTools(): Promise<unknown[]> {
    return this.get("/tools");
  }

  // --- Invitations ---

  async createInvite(wsSlug: string, role: string, email?: string): Promise<Invite> {
    const body: Record<string, unknown> = { role };
    if (email) body.email = email;
    return this.post(`/workspaces/${wsSlug}/invites`, body);
  }

  async listInvites(wsSlug: string): Promise<Invite[]> {
    return this.get(`/workspaces/${wsSlug}/invites`);
  }
}

// --- Static helpers (no auth required) ---

export async function waitForReady(baseUrl: string, maxWaitMs = 120_000): Promise<ReadinessInfo> {
  const start = Date.now();
  while (Date.now() - start < maxWaitMs) {
    try {
      const resp = await fetch(`${baseUrl}/api/v1/ready`);
      if (resp.ok || resp.status === 503) {
        const info: ReadinessInfo = await resp.json();
        // Accept ready, degraded, or unhealthy if core components work.
        if (info.status !== "unhealthy") return info;
        const dbStatus = info.components.database?.status;
        if (dbStatus === "up" || dbStatus === "unconfigured") return info;
      }
    } catch {
      // Server not reachable yet.
    }
    await new Promise((r) => setTimeout(r, 2000));
  }
  throw new Error(`Server not ready after ${maxWaitMs}ms`);
}

// --- Types ---

export interface Workspace {
  id: string;
  slug: string;
  name: string;
}

export interface Project {
  id: string;
  name: string;
  items?: Array<{ id: string; name: string }>;
}

export interface Concept {
  domain: string;
  definition: string;
  terms: Array<{
    text: string;
    locale: string;
    status?: string;
    part_of_speech?: string;
  }>;
}

export interface AutomationRule {
  id: string;
  name: string;
  trigger: string;
  conditions: unknown[];
  actions: unknown[];
  enabled: boolean;
}

export interface Invite {
  id: string;
  code: string;
  email: string;
  role: string;
}

export interface ReadinessInfo {
  status: "ready" | "degraded" | "unhealthy";
  version: string;
  components: Record<
    string,
    {
      status: string;
      type?: string;
      providers?: Array<{ name: string; model?: string; configured: boolean }>;
    }
  >;
}
