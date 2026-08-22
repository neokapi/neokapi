/**
 * Unified Bowrain API client for all e2e test suites, covering health, auth,
 * workspaces, projects, file upload, pseudo-translate, content memory, terms,
 * invites, automations, brand profiles, streams, tasks, notifications, and
 * activities.
 *
 * Two route families reach the server, and the split is the whole of it
 * (Bowrain AD-011). `/api/v1/workspaces` carries the collection verbs alone —
 * list a user's workspaces, create one. Everything scoped to a workspace hangs
 * off the bare slug, `/api/v1/:ws/...`, and a project off `/api/v1/:ws/:id/...`
 * with no noun between them. A path of the shape `/workspaces/:ws/anything` is
 * registered nowhere and 404s.
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

/** A governance change-set as its list/detail routes report it. */
export interface ChangeSetSummary {
  id: string;
  name: string;
  status: string;
  created_by: string;
  governed?: boolean;
  merged_at?: string;
  merged_by?: string;
  reviews?: Array<{ reviewer: string; verdict: string; basis?: string }>;
  solo_review?: boolean;
}

/** What merging a change-set did (knowledge.MergeResult). */
export interface MergeResultInfo {
  applied?: number;
  conflicts?: unknown[];
  revisions_created?: number;
  pilots_stopped?: number;
}

/** One cost class of a change-set's reach (knowledge.ReachClass). */
export interface ReachClassInfo {
  blocks: number;
  words: number;
  collections: number;
  projects: number;
  targets: number;
  approved: number;
  locales: string[];
}

/** The blast radius, with the reach split the reviewer reads before approving. */
export interface ChangeSetImpactInfo {
  total_blocks: number;
  affected_blocks: number;
  new_violations: number;
  resolved: number;
  words: number;
  projects?: Array<{
    project_id: string;
    project_name: string;
    affected_blocks: number;
    collections?: Array<{ collection_name: string; affected_blocks: number }>;
  }> | null;
  reach?: {
    annotate: ReachClassInfo;
    transform: ReachClassInfo;
    transform_projects: Array<{ project_id: string; project_name: string }>;
  };
  partial?: boolean;
  stored?: boolean;
}

/** The findings diff for one stream (knowledge.TrialReport). */
export interface TrialReportInfo {
  changeset_id: string;
  project_id: string;
  stream: string;
  total_blocks: number;
  changed_blocks: number;
  raised: Array<{ kind: string; rule: string; replacement?: string; block_id: string }>;
  cleared: Array<{ kind: string; rule: string; block_id: string }>;
  raised_total: number;
  cleared_total: number;
  voice_bound?: string;
  terms_computed: boolean;
}

/**
 * One collection a recipe declares, as the context content type carries it:
 * the point it sits at and the voice governing it. Mirrors
 * sync.v1.SyncContextEntry.
 */
export interface SyncContextEntry {
  name: string;
  coordinates?: Record<string, string>;
  channel?: string;
  voice_profile?: string;
  /** The authored profile, base64 of its JSON — the push carries the
   *  governance itself, not merely a reference to one the server may hold. */
  voice_profile_json?: string;
  owner?: string;
  content_hash?: string;
}

/** The workspace's observation that two channel slugs look like one channel. */
export interface ChannelAliasProposal {
  profile?: string;
  proposed_channel: string;
  existing_channel: string;
  evidence?: string;
  project_id?: string;
  collection?: string;
  status?: string;
  judged_by?: string;
  judged_at?: string;
}

/** The profile aggregation behind the Context hub's landing. */
export interface ContextProfilesResponse {
  profiles: Array<{
    slug: string;
    name: string;
    label: string;
    is_default: boolean;
    declared: boolean;
    coordinates?: Record<string, string>;
    channel?: string;
    voice?: { id: string; name: string };
    collections: Array<{ name: string; project_name: string }>;
    pending_changes: number;
    checks?: {
      score: number;
      scored_blocks: number;
      findings: number;
      last_checked_at?: string;
    };
  }>;
  axes: string[];
  terms: { concept_count: number };
  scan_scope: string;
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

export interface Entry {
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
    const info = await this.info();
    return (info as unknown as { formats: unknown[] }).formats ?? [];
  }

  async listTools(): Promise<unknown[]> {
    const info = await this.info();
    return (info as unknown as { tools: unknown[] }).tools ?? [];
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
    return this.get(`/${slug}`);
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
  // Projects
  // -----------------------------------------------------------------------

  async createProject(
    wsSlug: string,
    name: string,
    sourceLanguage: string,
    targetLanguages: string[],
  ): Promise<Project> {
    return this.post(`/${wsSlug}/projects`, {
      name,
      default_source_language: sourceLanguage,
      target_languages: targetLanguages,
    });
  }

  async getProject(wsSlug: string, projectId: string): Promise<Project> {
    return this.get(`/${wsSlug}/${projectId}`);
  }

  async listProjects(wsSlug: string): Promise<Project[]> {
    return this.get(`/${wsSlug}/projects`);
  }

  async deleteProject(wsSlug: string, projectId: string): Promise<void> {
    return this.del(`/${wsSlug}/${projectId}`);
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

  /**
   * Uploads a file onto a project's stream. Every project gets a `main` stream
   * at creation, so that is the default a caller with no stream of its own
   * wants.
   */
  async uploadFile(
    wsSlug: string,
    projectId: string,
    fileName: string,
    content: string | Buffer | Uint8Array,
    stream = "main",
  ): Promise<void> {
    const formData = new FormData();
    formData.append("files", new Blob([content]), fileName);

    const resp = await fetch(
      `${this.apiUrl}/${wsSlug}/${projectId}/items/${encodeURIComponent(stream)}`,
      {
        method: "POST",
        headers: { Authorization: `Bearer ${this.token}` },
        body: formData,
      },
    );
    if (!resp.ok) throw new Error(`Upload ${fileName}: ${resp.status} ${await resp.text()}`);
  }

  /** The file is named in `?item=`; the action itself is scoped to the stream. */
  async pseudoTranslate(
    wsSlug: string,
    projectId: string,
    fileName: string,
    targetLocale: string,
    stream = "main",
  ): Promise<{ total_blocks: number; translated_blocks: number }> {
    return this.post(
      `/${wsSlug}/${projectId}/actions/${encodeURIComponent(stream)}/pseudo-translate?item=${encodeURIComponent(fileName)}`,
      { target_locale: targetLocale },
    );
  }

  // -----------------------------------------------------------------------
  // Content Memory
  // -----------------------------------------------------------------------

  /**
   * The content memory's route keeps the `translation-memory` spelling: it is a
   * wire path clients are already built against, and so sits behind the rename
   * boundary.
   */
  async addMemoryEntry(
    wsSlug: string,
    source: string,
    target: string,
    sourceLocale: string,
    targetLocale: string,
  ): Promise<unknown> {
    return this.post(`/${wsSlug}/translation-memory`, {
      source,
      target,
      source_locale: sourceLocale,
      target_locale: targetLocale,
    });
  }

  async searchMemory(wsSlug: string, query: string): Promise<unknown> {
    return this.get(`/${wsSlug}/translation-memory?q=${encodeURIComponent(query)}`);
  }

  // -----------------------------------------------------------------------
  // Terms
  // -----------------------------------------------------------------------

  /** Creates a concept through ordinary curation and returns it with its id. */
  async addConcept(
    wsSlug: string,
    concept: {
      domain?: string;
      definition?: string;
      terms: Array<{
        text: string;
        locale: string;
        status?: string;
        part_of_speech?: string;
        gender?: string;
      }>;
    },
  ): Promise<{ id: string }> {
    return this.post(`/${wsSlug}/concepts`, concept);
  }

  async searchTerms(wsSlug: string, query: string): Promise<unknown> {
    return this.get(`/${wsSlug}/concepts?q=${encodeURIComponent(query)}`);
  }

  // -----------------------------------------------------------------------
  // Context: the points a workspace's content occupies, and the channel-slug
  // equivalence the workspace observes between its projects
  // -----------------------------------------------------------------------

  /**
   * Pushes the context content type alone — the collections a recipe declares,
   * with their coordinates and the voice governing each. No chunks, so the
   * commit carries the recipe's structure and nothing else; the worker
   * reconciles it, and the completed push is what makes slug fragmentation
   * visible to the workspace.
   */
  async pushContext(
    wsSlug: string,
    projectId: string,
    stream: string,
    entries: SyncContextEntry[],
  ): Promise<{ push_id: string }> {
    return this.post(`/${wsSlug}/${projectId}/sync/${stream}/push/commit`, {
      chunks: [],
      contexts: entries,
    });
  }

  async listContextProfiles(wsSlug: string): Promise<ContextProfilesResponse> {
    return this.get(`/${wsSlug}/profiles`);
  }

  async listChannelProposals(
    wsSlug: string,
    status?: string,
  ): Promise<{ proposals: ChannelAliasProposal[] }> {
    const query = status ? `?status=${encodeURIComponent(status)}` : "";
    return this.get(`/${wsSlug}/context/channel-proposals${query}`);
  }

  async judgeChannelProposal(
    wsSlug: string,
    judgement: {
      profile?: string;
      proposed_channel: string;
      existing_channel: string;
      status: "accepted" | "dismissed";
    },
  ): Promise<ChannelAliasProposal> {
    return this.post(`/${wsSlug}/context/channel-proposals/judge`, judgement);
  }

  // -----------------------------------------------------------------------
  // Governance change-sets: the reviewed path into the graph, and the two
  // things a reviewer reads before approving one — its reach and its trial.
  // -----------------------------------------------------------------------

  async createChangeset(
    wsSlug: string,
    name: string,
    description?: string,
  ): Promise<ChangeSetSummary> {
    return this.post(`/${wsSlug}/changesets`, { name, description });
  }

  async addChangesetOp(
    wsSlug: string,
    changesetId: string,
    op: string,
    payload: unknown,
  ): Promise<unknown> {
    return this.post(`/${wsSlug}/changesets/${changesetId}/ops`, { op, payload });
  }

  async submitChangeset(wsSlug: string, changesetId: string): Promise<ChangeSetSummary> {
    return this.post(`/${wsSlug}/changesets/${changesetId}/submit`);
  }

  async approveChangeset(
    wsSlug: string,
    changesetId: string,
    comment?: string,
  ): Promise<ChangeSetSummary> {
    return this.post(`/${wsSlug}/changesets/${changesetId}/approve`, { comment });
  }

  async mergeChangeset(wsSlug: string, changesetId: string): Promise<MergeResultInfo> {
    return this.post(`/${wsSlug}/changesets/${changesetId}/merge`);
  }

  async getChangeset(wsSlug: string, changesetId: string): Promise<ChangeSetSummary> {
    return this.get(`/${wsSlug}/changesets/${changesetId}`);
  }

  /**
   * The blast radius. `fresh` runs the live walk, which is the only path that
   * carries the per-project breakdown; without it a submitted change-set
   * answers from the summary stored at submit.
   */
  async changesetBlastRadius(
    wsSlug: string,
    changesetId: string,
    fresh = false,
  ): Promise<ChangeSetImpactInfo> {
    return this.get(`/${wsSlug}/changesets/${changesetId}/blast-radius${fresh ? "?fresh=1" : ""}`);
  }

  /** The findings diff for one stream under a change-set's draft. */
  async trialFindings(
    wsSlug: string,
    changesetId: string,
    projectId: string,
    stream: string,
  ): Promise<TrialReportInfo> {
    return this.get(
      `/${wsSlug}/changesets/${changesetId}/pilots/${encodeURIComponent(projectId)}/${encodeURIComponent(stream)}/findings`,
    );
  }

  async startPilot(
    wsSlug: string,
    changesetId: string,
    projectId: string,
    stream: string,
  ): Promise<unknown> {
    return this.post(`/${wsSlug}/changesets/${changesetId}/pilots`, {
      project_id: projectId,
      stream,
    });
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
    return this.post(`/${wsSlug}/invites`, body);
  }

  async listInvites(wsSlug: string): Promise<Invite[]> {
    return this.get(`/${wsSlug}/invites`);
  }

  async acceptInvite(code: string): Promise<void> {
    await this.post(`/join/${code}`);
  }

  // -----------------------------------------------------------------------
  // Automations
  // -----------------------------------------------------------------------

  async listAutomationRules(wsSlug: string, projectId: string): Promise<AutomationRule[]> {
    return this.get(`/${wsSlug}/${projectId}/automations`);
  }

  async createAutomationRule(
    wsSlug: string,
    projectId: string,
    rule: Partial<AutomationRule>,
  ): Promise<AutomationRule> {
    return this.post(`/${wsSlug}/${projectId}/automations`, rule);
  }

  async updateAutomationRule(
    wsSlug: string,
    projectId: string,
    ruleId: string,
    rule: Partial<AutomationRule>,
  ): Promise<AutomationRule> {
    return this.put(`/${wsSlug}/${projectId}/automations/${ruleId}`, rule);
  }

  async deleteAutomationRule(wsSlug: string, projectId: string, ruleId: string): Promise<void> {
    return this.del(`/${wsSlug}/${projectId}/automations/${ruleId}`);
  }

  // -----------------------------------------------------------------------
  // Brand Profiles
  // -----------------------------------------------------------------------

  async listVoiceProfiles(wsSlug: string): Promise<BrandProfile[]> {
    const result = await this.get<BrandProfile[] | null>(`/${wsSlug}/voice-profiles`);
    return result ?? [];
  }

  async createVoiceProfile(wsSlug: string, profile: Partial<BrandProfile>): Promise<BrandProfile> {
    return this.post(`/${wsSlug}/voice-profiles`, profile);
  }

  async updateVoiceProfile(
    wsSlug: string,
    profileId: string,
    profile: Partial<BrandProfile>,
  ): Promise<BrandProfile> {
    return this.put(`/${wsSlug}/voice-profiles/${profileId}`, profile);
  }

  async deleteVoiceProfile(wsSlug: string, profileId: string): Promise<void> {
    return this.del(`/${wsSlug}/voice-profiles/${profileId}`);
  }

  async checkBrandProfile(wsSlug: string, profileId: string): Promise<unknown> {
    return this.post(`/${wsSlug}/voice-profiles/${profileId}/check`);
  }

  async createBrandProfileFromStarter(
    wsSlug: string,
    pack: string,
    name?: string,
  ): Promise<BrandProfile> {
    const body: Record<string, unknown> = { pack };
    if (name) body.name = name;
    return this.post(`/${wsSlug}/voice-profiles/from-starter`, body);
  }

  // -----------------------------------------------------------------------
  // Streams
  // -----------------------------------------------------------------------

  async listStreams(wsSlug: string, projectId: string): Promise<Stream[]> {
    return this.get(`/${wsSlug}/${projectId}/streams`);
  }

  async createStream(
    wsSlug: string,
    projectId: string,
    opts: { name: string; parent?: string; description?: string },
  ): Promise<Stream> {
    return this.post(`/${wsSlug}/${projectId}/streams`, opts);
  }

  async getStream(wsSlug: string, projectId: string, streamName: string): Promise<Stream> {
    return this.get(`/${wsSlug}/${projectId}/streams/${encodeURIComponent(streamName)}`);
  }

  async updateStream(
    wsSlug: string,
    projectId: string,
    streamName: string,
    updates: Partial<Stream>,
  ): Promise<Stream> {
    return this.patch(`/${wsSlug}/${projectId}/streams/${encodeURIComponent(streamName)}`, updates);
  }

  async deleteStream(wsSlug: string, projectId: string, streamName: string): Promise<void> {
    return this.del(`/${wsSlug}/${projectId}/streams/${encodeURIComponent(streamName)}`);
  }

  async mergeStream(wsSlug: string, projectId: string, streamName: string): Promise<unknown> {
    return this.post(`/${wsSlug}/${projectId}/streams/${encodeURIComponent(streamName)}/merge`);
  }

  async diffStream(wsSlug: string, projectId: string, streamName: string): Promise<unknown> {
    return this.get(`/${wsSlug}/${projectId}/streams/${encodeURIComponent(streamName)}/diff`);
  }

  // -----------------------------------------------------------------------
  // Tasks
  // -----------------------------------------------------------------------

  async listTasks(wsSlug: string): Promise<Task[]> {
    const result = await this.get<{ tasks: Task[]; next_cursor: string }>(`/${wsSlug}/tasks`);
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
    return this.post(`/${wsSlug}/tasks`, task);
  }

  async getTask(wsSlug: string, taskId: string): Promise<Task> {
    return this.get(`/${wsSlug}/tasks/${taskId}`);
  }

  async updateTask(wsSlug: string, taskId: string, updates: Partial<Task>): Promise<Task> {
    return this.patch(`/${wsSlug}/tasks/${taskId}`, updates);
  }

  async deleteTask(wsSlug: string, taskId: string): Promise<void> {
    return this.del(`/${wsSlug}/tasks/${taskId}`);
  }

  async assignTask(wsSlug: string, taskId: string, assigneeId?: string): Promise<unknown> {
    // If no assigneeId provided, get current user's ID for self-assignment.
    let uid = assigneeId;
    if (!uid) {
      const me = await this.me();
      uid = me.id;
    }
    return this.post(`/${wsSlug}/tasks/${taskId}/assign`, { assignee_id: uid });
  }

  async completeTask(wsSlug: string, taskId: string): Promise<unknown> {
    return this.post(`/${wsSlug}/tasks/${taskId}/complete`);
  }

  async cancelTask(wsSlug: string, taskId: string): Promise<unknown> {
    return this.post(`/${wsSlug}/tasks/${taskId}/cancel`);
  }

  /**
   * "My tasks" is a filter on the task list, not a route of its own: the server
   * resolves the literal `me` to the authenticated user.
   */
  async myTasks(wsSlug: string): Promise<Task[]> {
    const result = await this.get<{ tasks: Task[]; next_cursor: string }>(
      `/${wsSlug}/tasks?assignee_id=me`,
    );
    return result.tasks;
  }

  // -----------------------------------------------------------------------
  // Notifications
  // -----------------------------------------------------------------------

  async listNotifications(wsSlug: string): Promise<Notification[]> {
    return this.get(`/${wsSlug}/notifications`);
  }

  async markNotificationRead(wsSlug: string, notificationId: string): Promise<void> {
    await this.post(`/${wsSlug}/notifications/${notificationId}/read`);
  }

  async markAllNotificationsRead(wsSlug: string): Promise<void> {
    await this.post(`/${wsSlug}/notifications/read-all`);
  }

  async deleteNotification(wsSlug: string, notificationId: string): Promise<void> {
    return this.del(`/${wsSlug}/notifications/${notificationId}`);
  }

  /** Preferences are a workspace-level resource, not a child of the feed. */
  async getNotificationPreferences(wsSlug: string): Promise<NotificationPreferences> {
    const result = await this.get<{ preferences: unknown }>(`/${wsSlug}/notification-preferences`);
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
      const prefsArray = (prefs as Record<string, unknown>).preferences as Array<
        Record<string, unknown>
      >;
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
    return this.put(`/${wsSlug}/notification-preferences`, body);
  }

  // -----------------------------------------------------------------------
  // Activities
  // -----------------------------------------------------------------------

  async listActivities(wsSlug: string): Promise<Activity[]> {
    const result = await this.get<{ activities: Activity[]; next_cursor: string }>(
      `/${wsSlug}/activities`,
    );
    return result.activities ?? [];
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
export async function waitForReady(baseUrl: string, maxWaitMs = 120_000): Promise<ReadinessInfo> {
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
