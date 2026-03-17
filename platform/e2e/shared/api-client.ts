/**
 * Unified Bowrain API client for all e2e test suites.
 * Merges the cloud and web-app clients into a single class covering
 * health, auth, workspaces, editor projects, file upload, pseudo-translate,
 * TM, terms, invites, automations, brand profiles, streams, tasks,
 * notifications, and activities.
 */

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

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
    gender?: string;
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
  max_uses?: number;
  use_count?: number;
  expires_at?: string;
}

export interface BrandProfile {
  id: string;
  name: string;
  pack?: string;
  [key: string]: unknown;
}

export interface Stream {
  id: string;
  name: string;
  parent?: string;
  description?: string;
  [key: string]: unknown;
}

export interface Task {
  id: string;
  title: string;
  description?: string;
  project_id?: string;
  type?: string;
  priority?: string;
  status?: string;
  assignee_id?: string;
  [key: string]: unknown;
}

export interface Notification {
  id: string;
  read: boolean;
  [key: string]: unknown;
}

export interface NotificationPreferences {
  [key: string]: unknown;
}

export interface Activity {
  id: string;
  [key: string]: unknown;
}

export interface TMEntry {
  source: string;
  target: string;
  source_locale: string;
  target_locale: string;
}

export interface ReadinessComponentStatus {
  status: string;
  type?: string;
  latency_ms?: number;
  providers?: Array<{ name: string; model?: string; configured: boolean }>;
  error?: string;
}

export interface ReadinessInfo {
  status: "ready" | "degraded" | "unhealthy";
  version: string;
  components: Record<string, ReadinessComponentStatus>;
}

// ---------------------------------------------------------------------------
// BowrainAPI class
// ---------------------------------------------------------------------------

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

  // --- HTTP primitives ---

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

  private async patch<T = unknown>(path: string, body?: unknown): Promise<T> {
    const resp = await fetch(`${this.apiUrl}${path}`, {
      method: "PATCH",
      headers: {
        Authorization: `Bearer ${this.token}`,
        "Content-Type": "application/json",
      },
      body: body ? JSON.stringify(body) : undefined,
    });
    if (!resp.ok) throw new Error(`PATCH ${path}: ${resp.status} ${await resp.text()}`);
    if (resp.status === 204) return null as T;
    return resp.json() as Promise<T>;
  }

  private async del(path: string): Promise<void> {
    const resp = await fetch(`${this.apiUrl}${path}`, {
      method: "DELETE",
      headers: { Authorization: `Bearer ${this.token}` },
    });
    if (!resp.ok && resp.status !== 204) {
      throw new Error(`DELETE ${path}: ${resp.status} ${await resp.text()}`);
    }
  }

  // -----------------------------------------------------------------------
  // Health (no auth required, but using class for convenience)
  // -----------------------------------------------------------------------

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

  async listFormats(): Promise<unknown[]> {
    return this.get("/formats");
  }

  async listTools(): Promise<unknown[]> {
    return this.get("/tools");
  }

  // -----------------------------------------------------------------------
  // Auth
  // -----------------------------------------------------------------------

  async me(): Promise<{ email: string; name: string; id: string }> {
    return this.get("/auth/me");
  }

  // -----------------------------------------------------------------------
  // Workspaces
  // -----------------------------------------------------------------------

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

  // -----------------------------------------------------------------------
  // Editor Projects
  // -----------------------------------------------------------------------

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
    return this.del(`/workspaces/${wsSlug}/editor/projects/${projectId}`);
  }

  async deleteAllProjects(wsSlug: string): Promise<void> {
    const projects = await this.listProjects(wsSlug);
    for (const p of projects) {
      await this.deleteProject(wsSlug, p.id);
    }
  }

  /** Find an item ID by filename within a project's items array. */
  findItemId(project: { items?: Array<{ id: string; name: string }> }, fileName: string): string {
    const item = project.items?.find((i) => i.name === fileName);
    if (!item) throw new Error(`Item not found: ${fileName}`);
    return item.id;
  }

  // -----------------------------------------------------------------------
  // File Operations
  // -----------------------------------------------------------------------

  async uploadFile(
    wsSlug: string,
    projectId: string,
    fileName: string,
    content: string | Buffer | Uint8Array,
  ): Promise<void> {
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

  // -----------------------------------------------------------------------
  // Translation Memory
  // -----------------------------------------------------------------------

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

  // -----------------------------------------------------------------------
  // Terminology
  // -----------------------------------------------------------------------

  async addConcept(wsSlug: string, concept: Concept): Promise<unknown> {
    return this.post(`/workspaces/${wsSlug}/terms`, concept);
  }

  async searchTerms(wsSlug: string, query: string): Promise<unknown> {
    return this.get(`/workspaces/${wsSlug}/terms?q=${encodeURIComponent(query)}`);
  }

  // -----------------------------------------------------------------------
  // Invitations
  // -----------------------------------------------------------------------

  async createInvite(
    wsSlug: string,
    role: string,
    email?: string,
    maxUses?: number,
    ttlDays?: number,
  ): Promise<Invite> {
    const body: Record<string, unknown> = { role };
    if (email) body.email = email;
    if (maxUses !== undefined) body.max_uses = maxUses;
    if (ttlDays !== undefined) body.ttl_days = ttlDays;
    return this.post(`/workspaces/${wsSlug}/invites`, body);
  }

  async listInvites(wsSlug: string): Promise<Invite[]> {
    return this.get(`/workspaces/${wsSlug}/invites`);
  }

  async acceptInvite(code: string): Promise<void> {
    await this.post(`/join/${code}`);
  }

  // -----------------------------------------------------------------------
  // Automations
  // -----------------------------------------------------------------------

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
    return this.del(`/workspaces/${wsSlug}/projects/${projectId}/automations/${ruleId}`);
  }

  // -----------------------------------------------------------------------
  // Brand Profiles
  // -----------------------------------------------------------------------

  async listBrandProfiles(wsSlug: string): Promise<BrandProfile[]> {
    const result = await this.get<BrandProfile[] | null>(`/workspaces/${wsSlug}/brand-profiles`);
    return result ?? [];
  }

  async createBrandProfile(wsSlug: string, profile: Partial<BrandProfile>): Promise<BrandProfile> {
    return this.post(`/workspaces/${wsSlug}/brand-profiles`, profile);
  }

  async updateBrandProfile(
    wsSlug: string,
    profileId: string,
    profile: Partial<BrandProfile>,
  ): Promise<BrandProfile> {
    return this.put(`/workspaces/${wsSlug}/brand-profiles/${profileId}`, profile);
  }

  async deleteBrandProfile(wsSlug: string, profileId: string): Promise<void> {
    return this.del(`/workspaces/${wsSlug}/brand-profiles/${profileId}`);
  }

  async checkBrandProfile(wsSlug: string, profileId: string): Promise<unknown> {
    return this.post(`/workspaces/${wsSlug}/brand-profiles/${profileId}/check`);
  }

  async createBrandProfileFromStarter(
    wsSlug: string,
    pack: string,
    name?: string,
  ): Promise<BrandProfile> {
    const body: Record<string, unknown> = { pack };
    if (name) body.name = name;
    return this.post(`/workspaces/${wsSlug}/brand-profiles/from-starter`, body);
  }

  // -----------------------------------------------------------------------
  // Streams
  // -----------------------------------------------------------------------

  async listStreams(wsSlug: string, projectId: string): Promise<Stream[]> {
    return this.get(`/workspaces/${wsSlug}/projects/${projectId}/streams`);
  }

  async createStream(
    wsSlug: string,
    projectId: string,
    opts: { name: string; parent?: string; description?: string },
  ): Promise<Stream> {
    return this.post(`/workspaces/${wsSlug}/projects/${projectId}/streams`, opts);
  }

  async getStream(wsSlug: string, projectId: string, streamName: string): Promise<Stream> {
    return this.get(`/workspaces/${wsSlug}/projects/${projectId}/streams/${encodeURIComponent(streamName)}`);
  }

  async updateStream(
    wsSlug: string,
    projectId: string,
    streamName: string,
    updates: Partial<Stream>,
  ): Promise<Stream> {
    return this.patch(
      `/workspaces/${wsSlug}/projects/${projectId}/streams/${encodeURIComponent(streamName)}`,
      updates,
    );
  }

  async deleteStream(wsSlug: string, projectId: string, streamName: string): Promise<void> {
    return this.del(`/workspaces/${wsSlug}/projects/${projectId}/streams/${encodeURIComponent(streamName)}`);
  }

  async mergeStream(wsSlug: string, streamId: string): Promise<unknown> {
    return this.post(`/workspaces/${wsSlug}/${streamId}/merge`);
  }

  async diffStream(wsSlug: string, streamId: string): Promise<unknown> {
    return this.get(`/workspaces/${wsSlug}/${streamId}/diff`);
  }

  // -----------------------------------------------------------------------
  // Tasks
  // -----------------------------------------------------------------------

  async listTasks(wsSlug: string): Promise<Task[]> {
    const result = await this.get<{ tasks: Task[]; next_cursor: string }>(`/workspaces/${wsSlug}/tasks`);
    return result.tasks;
  }

  async createTask(
    wsSlug: string,
    task: {
      title: string;
      project_id: string;
      description?: string;
      type?: string;
      priority?: string;
      assignee_id?: string;
    },
  ): Promise<Task> {
    return this.post(`/workspaces/${wsSlug}/tasks`, task);
  }

  async getTask(wsSlug: string, taskId: string): Promise<Task> {
    return this.get(`/workspaces/${wsSlug}/tasks/${taskId}`);
  }

  async updateTask(wsSlug: string, taskId: string, updates: Partial<Task>): Promise<Task> {
    return this.patch(`/workspaces/${wsSlug}/tasks/${taskId}`, updates);
  }

  async deleteTask(wsSlug: string, taskId: string): Promise<void> {
    return this.del(`/workspaces/${wsSlug}/tasks/${taskId}`);
  }

  async assignTask(wsSlug: string, taskId: string, assigneeId?: string): Promise<unknown> {
    // If no assigneeId provided, get current user's ID for self-assignment.
    let uid = assigneeId;
    if (!uid) {
      const me = await this.me();
      uid = me.id;
    }
    return this.post(`/workspaces/${wsSlug}/tasks/${taskId}/assign`, { assignee_id: uid });
  }

  async completeTask(wsSlug: string, taskId: string): Promise<unknown> {
    return this.post(`/workspaces/${wsSlug}/tasks/${taskId}/complete`);
  }

  async cancelTask(wsSlug: string, taskId: string): Promise<unknown> {
    return this.post(`/workspaces/${wsSlug}/tasks/${taskId}/cancel`);
  }

  async myTasks(wsSlug: string): Promise<Task[]> {
    const result = await this.get<{ tasks: Task[]; next_cursor: string }>(`/workspaces/${wsSlug}/my/tasks`);
    return result.tasks;
  }

  // -----------------------------------------------------------------------
  // Notifications
  // -----------------------------------------------------------------------

  async listNotifications(wsSlug: string): Promise<Notification[]> {
    return this.get(`/workspaces/${wsSlug}/notifications`);
  }

  async markNotificationRead(wsSlug: string, notificationId: string): Promise<void> {
    await this.post(`/workspaces/${wsSlug}/notifications/${notificationId}/read`);
  }

  async markAllNotificationsRead(wsSlug: string): Promise<void> {
    await this.post(`/workspaces/${wsSlug}/notifications/read-all`);
  }

  async deleteNotification(wsSlug: string, notificationId: string): Promise<void> {
    return this.del(`/workspaces/${wsSlug}/notifications/${notificationId}`);
  }

  async getNotificationPreferences(wsSlug: string): Promise<NotificationPreferences> {
    const result = await this.get<{ preferences: unknown }>(`/workspaces/${wsSlug}/notifications/preferences`);
    return result as NotificationPreferences;
  }

  async updateNotificationPreferences(
    wsSlug: string,
    prefs: NotificationPreferences,
  ): Promise<NotificationPreferences> {
    // The server expects { preferences: [{category, channels: {web, email, push, desktop}}] }.
    // If prefs already has a preferences key (from getNotificationPreferences), reshape it
    // into the format the server expects.
    let body: unknown = prefs;
    if (Array.isArray((prefs as Record<string, unknown>).preferences)) {
      const prefsArray = (prefs as Record<string, unknown>).preferences as Array<Record<string, unknown>>;
      body = {
        preferences: prefsArray.map((p) => ({
          category: p.category,
          channels: {
            web: p.channel_web ?? false,
            email: p.channel_email ?? false,
            push: p.channel_push ?? false,
            desktop: p.channel_desktop ?? false,
          },
        })),
      };
    }
    return this.put(`/workspaces/${wsSlug}/notifications/preferences`, body);
  }

  // -----------------------------------------------------------------------
  // Activities
  // -----------------------------------------------------------------------

  async listActivities(wsSlug: string): Promise<Activity[]> {
    const result = await this.get<{ activities: Activity[]; next_cursor: string }>(`/workspaces/${wsSlug}/activities`);
    return result.activities;
  }
}

// ---------------------------------------------------------------------------
// Static helpers (no auth required)
// ---------------------------------------------------------------------------

/**
 * Perform the device auth flow and return a JWT access token.
 * If BOWRAIN_TOKEN is set, returns it directly (fast path for CI / external server).
 */
export async function deviceAuth(
  baseUrl: string,
  email = "admin@example.com",
  name = "Demo User",
): Promise<string> {
  // Fast path: pre-supplied token.
  const preSupplied = process.env.BOWRAIN_TOKEN;
  if (preSupplied) return preSupplied;

  const api = `${baseUrl}/api/v1`;

  // Step 1: Start device auth
  const startResp = await fetch(`${api}/auth/device/start`, {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body: "client_id=e2e-shared",
  });
  if (!startResp.ok) throw new Error(`Device start failed: ${startResp.status}`);
  const startData = await startResp.json();
  const { device_code, user_code } = startData;

  // Step 2: Verify (simulates user approving in browser)
  const verifyResp = await fetch(`${api}/auth/device/verify`, {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body: `user_code=${user_code}&email=${encodeURIComponent(email)}&name=${encodeURIComponent(name)}`,
    redirect: "manual",
  });
  if (!verifyResp.ok && verifyResp.status !== 302) {
    throw new Error(`Device verify failed: ${verifyResp.status}`);
  }

  // Step 3: Poll for token
  const pollResp = await fetch(`${api}/auth/device/poll`, {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body: `device_code=${device_code}&grant_type=urn:ietf:params:oauth:grant-type:device_code`,
  });
  if (!pollResp.ok) throw new Error(`Device poll failed: ${pollResp.status}`);
  const pollData = await pollResp.json();
  return pollData.access_token;
}

/**
 * Wait for the server to become ready.
 * Accepts "unhealthy" status when the database reports "up" or "unconfigured"
 * (e.g. SQLite mode or non-critical components missing).
 */
export async function waitForReady(
  baseUrl: string,
  maxWaitMs = 120_000,
): Promise<ReadinessInfo> {
  const start = Date.now();
  let lastError: string | undefined;

  while (Date.now() - start < maxWaitMs) {
    try {
      const resp = await fetch(`${baseUrl}/api/v1/ready`);
      if (resp.ok || resp.status === 503) {
        const info: ReadinessInfo = await resp.json();
        // Accept ready or degraded immediately.
        if (info.status !== "unhealthy") return info;
        // Accept unhealthy when core database is functional.
        const dbStatus = info.components?.database?.status;
        if (dbStatus === "up" || dbStatus === "unconfigured") return info;
        lastError = `status=${info.status}`;
      }
    } catch {
      // Server not reachable yet.
    }
    await new Promise((r) => setTimeout(r, 2000));
  }
  throw new Error(`Server not ready after ${maxWaitMs}ms (${lastError ?? "unreachable"})`);
}
