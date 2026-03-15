import type { ApiAdapter } from "./adapter";
import type {
  User,
  Workspace,
  Membership,
  ProjectInfo,
  ConfigResponse,
  BlockInfo,
  UpdateBlockRequest,
  UpdateBlockTargetCodedRequest,
  AITranslateFileRequest,
  TranslationStats,
  WordCountResult,
  ProviderConfig,
  ProviderConfigWithKey,
  TMEntryInfo,
  TMSearchResult,
  TMUpdateRequest,
  TMMatchInfo,
  ConceptInfo,
  TermSearchResult,
  AddConceptRequest,
  UpdateConceptRequest,
  BlockTermMatch,
  BlockNote,
  BlockHistoryEntry,
  LocaleInfo,
  FormatInfo,
  ToolInfo,
  Invite,
  AcceptInviteResponse,
  ClaimProjectResponse,
  ApiToken,
  CreateApiTokenResponse,
  QAIssue,
  FileQAResult,
  AutomationRule,
  AutomationEvent,
  AutomationHistoryEntry,
  SaveAutomationRuleRequest,
  NotificationInfo,
  EntityInfo,
  StreamInfo,
  StreamDiffResult,
  StreamMergeResult,
  CreateStreamRequest,
  CollectionInfo,
  CreateCollectionRequest,
  AuditEntry,
  AuditQuery,
  ArchivedProject,
} from "../types/api";
import type {
  VoiceProfile,
  StoredScore,
  ScoreTrend,
  CreateVoiceProfileRequest,
  UpdateVoiceProfileRequest,
} from "../brand/types";

/**
 * RestApiAdapter talks to the neokapi REST server.
 * Used by the web apps (apps/web and apps/kapi-web).
 *
 * Supports two auth modes:
 * - Bearer token mode (desktop/CLI): token set via setToken(), sent as Authorization header.
 * - Cookie mode (web): no token set, browser sends HttpOnly cookies automatically.
 */
export class RestApiAdapter implements ApiAdapter {
  private baseUrl: string;
  private token: string | null;
  private refreshToken: string | null;

  /** Called when tokens are refreshed so the consumer can persist them. */
  onTokenRefresh?: (token: string, refreshToken: string) => void;

  /** Called when the session is invalid and the user must re-authenticate. */
  onSessionExpired?: () => void;

  private refreshPromise: Promise<boolean> | null = null;

  constructor(baseUrl: string = "", token: string | null = null) {
    this.baseUrl = baseUrl;
    this.token = token;
    this.refreshToken = null;
  }

  setToken(token: string) {
    this.token = token || null;
  }

  getToken(): string | null {
    return this.token;
  }

  setRefreshToken(rt: string) {
    this.refreshToken = rt || null;
  }

  private headers(): Record<string, string> {
    const h: Record<string, string> = { "Content-Type": "application/json" };
    if (this.token) {
      h["Authorization"] = `Bearer ${this.token}`;
    }
    return h;
  }

  /** Attempt to refresh the access token using the stored refresh token or cookie. */
  private async tryRefresh(): Promise<boolean> {
    try {
      const resp = await fetch(`${this.baseUrl}/api/v1/auth/refresh`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "same-origin",
        body: JSON.stringify(this.refreshToken ? { refresh_token: this.refreshToken } : {}),
      });
      if (!resp.ok) return false;
      const data = await resp.json();
      if (data.access_token) {
        this.token = data.access_token;
        this.refreshToken = data.refresh_token;
        this.onTokenRefresh?.(data.access_token, data.refresh_token);
      }
      return true;
    } catch {
      return false;
    }
  }

  private async fetchJSON<T>(path: string, init?: RequestInit): Promise<T> {
    const resp = await fetch(`${this.baseUrl}${path}`, {
      ...init,
      headers: { ...this.headers(), ...(init?.headers as Record<string, string>) },
      credentials: "same-origin",
    });
    if (resp.status === 401) {
      // Deduplicate concurrent refresh attempts.
      if (!this.refreshPromise) {
        this.refreshPromise = this.tryRefresh().finally(() => {
          this.refreshPromise = null;
        });
      }
      const refreshed = await this.refreshPromise;
      if (refreshed) {
        // Retry the original request with the new token.
        const retry = await fetch(`${this.baseUrl}${path}`, {
          ...init,
          headers: { ...this.headers(), ...(init?.headers as Record<string, string>) },
          credentials: "same-origin",
        });
        if (!retry.ok) {
          const body = await retry.text();
          throw new Error(`${retry.status}: ${body}`);
        }
        if (retry.status === 204) return undefined as T;
        return retry.json();
      }
      this.onSessionExpired?.();
    }
    if (!resp.ok) {
      const body = await resp.text();
      throw new Error(`${resp.status}: ${body}`);
    }
    if (resp.status === 204) return undefined as T;
    return resp.json();
  }

  private async fetchBlob(path: string, init?: RequestInit): Promise<Blob> {
    const resp = await fetch(`${this.baseUrl}${path}`, {
      ...init,
      headers: { ...this.headers(), ...(init?.headers as Record<string, string>) },
      credentials: "same-origin",
    });
    if (!resp.ok) {
      const body = await resp.text();
      throw new Error(`${resp.status}: ${body}`);
    }
    return resp.blob();
  }

  private async fetchText(path: string, init?: RequestInit): Promise<string> {
    const resp = await fetch(`${this.baseUrl}${path}`, {
      ...init,
      headers: { ...this.headers(), ...(init?.headers as Record<string, string>) },
      credentials: "same-origin",
    });
    if (resp.status === 401) {
      if (!this.refreshPromise) {
        this.refreshPromise = this.tryRefresh().finally(() => {
          this.refreshPromise = null;
        });
      }
      const refreshed = await this.refreshPromise;
      if (refreshed) {
        const retry = await fetch(`${this.baseUrl}${path}`, {
          ...init,
          headers: { ...this.headers(), ...(init?.headers as Record<string, string>) },
          credentials: "same-origin",
        });
        if (!retry.ok) {
          const body = await retry.text();
          throw new Error(`${retry.status}: ${body}`);
        }
        return retry.text();
      }
      this.onSessionExpired?.();
    }
    if (!resp.ok) {
      const body = await resp.text();
      throw new Error(`${resp.status}: ${body}`);
    }
    return resp.text();
  }

  /** Encode a file path for use in URLs, keeping `/` separators intact. */
  private encodeFilePath(name: string): string {
    return name.split("/").map(encodeURIComponent).join("/");
  }

  /** Appends ?stream=X to a path (or &stream=X if query params exist). */
  private withStream(path: string, stream?: string): string {
    if (!stream || stream === "main") return path;
    const sep = path.includes("?") ? "&" : "?";
    return `${path}${sep}stream=${encodeURIComponent(stream)}`;
  }

  // ── Config ──────────────────────────────────────────────────────────────

  async getConfig(): Promise<ConfigResponse> {
    return this.fetchJSON("/api/v1/config");
  }

  // ── Auth ─────────────────────────────────────────────────────────────────

  async getCurrentUser(): Promise<User | null> {
    try {
      return await this.fetchJSON("/api/v1/auth/me");
    } catch {
      return null;
    }
  }

  // ── Workspaces ───────────────────────────────────────────────────────────

  async listWorkspaces(): Promise<Workspace[]> {
    return this.fetchJSON("/api/v1/workspaces");
  }

  async createWorkspace(name: string, slug: string): Promise<Workspace> {
    return this.fetchJSON("/api/v1/workspaces", {
      method: "POST",
      body: JSON.stringify({ name, slug }),
    });
  }

  async getWorkspace(slug: string): Promise<Workspace> {
    return this.fetchJSON(`/api/v1/workspaces/${slug}`);
  }

  async updateWorkspace(slug: string, data: Partial<Workspace>): Promise<Workspace> {
    return this.fetchJSON(`/api/v1/workspaces/${slug}`, {
      method: "PUT",
      body: JSON.stringify(data),
    });
  }

  async deleteWorkspace(slug: string): Promise<void> {
    await this.fetchJSON(`/api/v1/workspaces/${slug}`, { method: "DELETE" });
  }

  // ── Members ──────────────────────────────────────────────────────────────

  async listMembers(workspaceSlug: string): Promise<Membership[]> {
    return this.fetchJSON(`/api/v1/workspaces/${workspaceSlug}/members`);
  }

  async addMember(workspaceSlug: string, userId: string, role: string): Promise<void> {
    await this.fetchJSON(`/api/v1/workspaces/${workspaceSlug}/members`, {
      method: "POST",
      body: JSON.stringify({ user_id: userId, role }),
    });
  }

  async updateMemberRole(workspaceSlug: string, userId: string, role: string): Promise<void> {
    await this.fetchJSON(`/api/v1/workspaces/${workspaceSlug}/members/${userId}/role`, {
      method: "PUT",
      body: JSON.stringify({ role }),
    });
  }

  async removeMember(workspaceSlug: string, userId: string): Promise<void> {
    await this.fetchJSON(`/api/v1/workspaces/${workspaceSlug}/members/${userId}`, {
      method: "DELETE",
    });
  }

  // ── Invites ──────────────────────────────────────────────────────────────

  async listInvites(workspaceSlug: string): Promise<Invite[]> {
    return this.fetchJSON(`/api/v1/workspaces/${workspaceSlug}/invites`);
  }

  async createInvite(
    workspaceSlug: string,
    email: string,
    role: string,
    maxUses: number,
  ): Promise<Invite> {
    return this.fetchJSON(`/api/v1/workspaces/${workspaceSlug}/invites`, {
      method: "POST",
      body: JSON.stringify({ email, role, max_uses: maxUses }),
    });
  }

  async deleteInvite(workspaceSlug: string, inviteId: string): Promise<void> {
    await this.fetchJSON(`/api/v1/workspaces/${workspaceSlug}/invites/${inviteId}`, {
      method: "DELETE",
    });
  }

  // ── API Tokens ─────────────────────────────────────────────────────────

  async listApiTokens(workspaceSlug: string): Promise<ApiToken[]> {
    return this.fetchJSON(`/api/v1/workspaces/${workspaceSlug}/tokens`);
  }

  async createApiToken(
    workspaceSlug: string,
    name: string,
    expireDays: number,
  ): Promise<CreateApiTokenResponse> {
    return this.fetchJSON(`/api/v1/workspaces/${workspaceSlug}/tokens`, {
      method: "POST",
      body: JSON.stringify({ name, expire_days: expireDays }),
    });
  }

  async deleteApiToken(workspaceSlug: string, tokenId: string): Promise<void> {
    await this.fetchJSON(`/api/v1/workspaces/${workspaceSlug}/tokens/${tokenId}`, {
      method: "DELETE",
    });
  }

  async acceptInvite(code: string): Promise<AcceptInviteResponse> {
    return this.fetchJSON(`/api/v1/join/${code}`, { method: "POST" });
  }

  async claimProject(claimToken: string): Promise<ClaimProjectResponse> {
    return this.fetchJSON(`/api/v1/projects/claim`, {
      method: "POST",
      body: JSON.stringify({ claim_token: claimToken }),
    });
  }

  // ── Streams ─────────────────────────────────────────────────────────────

  private streamEp(ws: string, projectId: string) {
    return `/api/v1/workspaces/${ws}/projects/${encodeURIComponent(projectId)}/streams`;
  }

  async listStreams(workspaceSlug: string, projectId: string): Promise<StreamInfo[]> {
    return this.fetchJSON(this.streamEp(workspaceSlug, projectId));
  }

  async createStream(
    workspaceSlug: string,
    projectId: string,
    req: CreateStreamRequest,
  ): Promise<StreamInfo> {
    return this.fetchJSON(this.streamEp(workspaceSlug, projectId), {
      method: "POST",
      body: JSON.stringify(req),
    });
  }

  async getStream(
    workspaceSlug: string,
    projectId: string,
    streamName: string,
  ): Promise<StreamInfo> {
    return this.fetchJSON(
      `${this.streamEp(workspaceSlug, projectId)}/${encodeURIComponent(streamName)}`,
    );
  }

  async updateStream(
    workspaceSlug: string,
    projectId: string,
    streamName: string,
    data: { description?: string; visibility?: string },
  ): Promise<StreamInfo> {
    return this.fetchJSON(
      `${this.streamEp(workspaceSlug, projectId)}/${encodeURIComponent(streamName)}`,
      {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(data),
      },
    );
  }

  async deleteStream(workspaceSlug: string, projectId: string, streamName: string): Promise<void> {
    await this.fetchJSON(
      `${this.streamEp(workspaceSlug, projectId)}/${encodeURIComponent(streamName)}`,
      {
        method: "DELETE",
      },
    );
  }

  async diffStream(
    workspaceSlug: string,
    projectId: string,
    streamName: string,
  ): Promise<StreamDiffResult> {
    return this.fetchJSON(
      `${this.streamEp(workspaceSlug, projectId)}/${encodeURIComponent(streamName)}/diff`,
    );
  }

  async mergeStream(
    workspaceSlug: string,
    projectId: string,
    streamName: string,
    dryRun?: boolean,
  ): Promise<StreamMergeResult> {
    const params = dryRun ? "?dry_run=true" : "";
    return this.fetchJSON(
      `${this.streamEp(workspaceSlug, projectId)}/${encodeURIComponent(streamName)}/merge${params}`,
      {
        method: "POST",
      },
    );
  }

  // ── Projects ─────────────────────────────────────────────────────────────

  private ep(ws: string) {
    return `/api/v1/workspaces/${ws}/editor/projects`;
  }

  async listProjects(workspaceSlug: string): Promise<ProjectInfo[]> {
    return this.fetchJSON(this.ep(workspaceSlug));
  }

  async createProject(
    workspaceSlug: string,
    name: string,
    defaultSourceLanguage: string,
    targetLanguages: string[],
  ): Promise<ProjectInfo> {
    return this.fetchJSON(this.ep(workspaceSlug), {
      method: "POST",
      body: JSON.stringify({ name, default_source_language: defaultSourceLanguage, target_languages: targetLanguages }),
    });
  }

  async getProject(
    workspaceSlug: string,
    projectId: string,
    stream?: string,
  ): Promise<ProjectInfo> {
    return this.fetchJSON(this.withStream(`${this.ep(workspaceSlug)}/${projectId}`, stream));
  }

  async updateProject(
    workspaceSlug: string,
    projectId: string,
    data: { name?: string; target_languages?: string[] },
  ): Promise<ProjectInfo> {
    return this.fetchJSON(`${this.ep(workspaceSlug)}/${projectId}`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(data),
    });
  }

  async deleteProject(workspaceSlug: string, projectId: string): Promise<void> {
    await this.fetchJSON(`${this.ep(workspaceSlug)}/${projectId}`, {
      method: "DELETE",
    });
  }

  async restoreProject(workspaceSlug: string, projectId: string): Promise<void> {
    await this.fetchJSON(`${this.ep(workspaceSlug)}/${projectId}/restore`, {
      method: "POST",
    });
  }

  async permanentlyDeleteProject(workspaceSlug: string, projectId: string): Promise<void> {
    await this.fetchJSON(`${this.ep(workspaceSlug)}/${projectId}/permanent`, {
      method: "DELETE",
    });
  }

  async listArchivedProjects(workspaceSlug: string): Promise<ArchivedProject[]> {
    return this.fetchJSON(
      `/api/v1/workspaces/${workspaceSlug}/archived/projects`,
    );
  }

  async restoreStream(
    workspaceSlug: string,
    projectId: string,
    streamName: string,
  ): Promise<void> {
    await this.fetchJSON(
      `/api/v1/workspaces/${workspaceSlug}/projects/${projectId}/streams/${streamName}/restore`,
      { method: "POST" },
    );
  }

  async uploadFiles(
    workspaceSlug: string,
    projectId: string,
    files: File[],
    stream?: string,
  ): Promise<ProjectInfo> {
    const formData = new FormData();
    for (const file of files) {
      formData.append("files", file);
    }
    const url = this.withStream(`${this.ep(workspaceSlug)}/${projectId}/files`, stream);
    const resp = await fetch(`${this.baseUrl}${url}`, {
      method: "POST",
      headers: this.token ? { Authorization: `Bearer ${this.token}` } : {},
      credentials: "same-origin",
      body: formData,
    });
    if (!resp.ok) {
      const body = await resp.text();
      throw new Error(`${resp.status}: ${body}`);
    }
    return resp.json();
  }

  async removeFile(
    workspaceSlug: string,
    projectId: string,
    fileName: string,
    stream?: string,
  ): Promise<ProjectInfo> {
    return this.fetchJSON(
      this.withStream(
        `${this.ep(workspaceSlug)}/${projectId}/file/${this.encodeFilePath(fileName)}`,
        stream,
      ),
      { method: "DELETE" },
    );
  }

  // ── Collections ─────────────────────────────────────────────────────────

  async listCollections(
    workspaceSlug: string,
    projectId: string,
    stream?: string,
  ): Promise<CollectionInfo[]> {
    return this.fetchJSON(
      this.withStream(`${this.ep(workspaceSlug)}/${projectId}/collections`, stream),
    );
  }

  async createCollection(
    workspaceSlug: string,
    projectId: string,
    req: CreateCollectionRequest,
  ): Promise<CollectionInfo> {
    return this.fetchJSON(`${this.ep(workspaceSlug)}/${projectId}/collections`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(req),
    });
  }

  async getCollection(
    workspaceSlug: string,
    projectId: string,
    collectionId: string,
  ): Promise<CollectionInfo> {
    return this.fetchJSON(`${this.ep(workspaceSlug)}/${projectId}/collections/${collectionId}`);
  }

  async updateCollection(
    workspaceSlug: string,
    projectId: string,
    collectionId: string,
    req: Partial<CreateCollectionRequest>,
  ): Promise<CollectionInfo> {
    return this.fetchJSON(
      `${this.ep(workspaceSlug)}/${projectId}/collections/${collectionId}`,
      {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(req),
      },
    );
  }

  async deleteCollection(
    workspaceSlug: string,
    projectId: string,
    collectionId: string,
  ): Promise<void> {
    await this.fetchJSON(
      `${this.ep(workspaceSlug)}/${projectId}/collections/${collectionId}`,
      { method: "DELETE" },
    );
  }

  async uploadToCollection(
    workspaceSlug: string,
    projectId: string,
    collectionId: string,
    files: File[],
    stream?: string,
  ): Promise<ProjectInfo> {
    const formData = new FormData();
    for (const file of files) {
      formData.append("files", file);
    }
    const url = this.withStream(
      `${this.ep(workspaceSlug)}/${projectId}/collections/${collectionId}/files`,
      stream,
    );
    const resp = await fetch(`${this.baseUrl}${url}`, {
      method: "POST",
      headers: this.token ? { Authorization: `Bearer ${this.token}` } : {},
      credentials: "same-origin",
      body: formData,
    });
    if (!resp.ok) {
      const body = await resp.text();
      throw new Error(`${resp.status}: ${body}`);
    }
    return resp.json();
  }

  // ── Editor ───────────────────────────────────────────────────────────────

  async getFileBlocks(
    workspaceSlug: string,
    projectId: string,
    fileName: string,
    stream?: string,
  ): Promise<BlockInfo[]> {
    return this.fetchJSON(
      this.withStream(
        `${this.ep(workspaceSlug)}/${projectId}/file-blocks/${this.encodeFilePath(fileName)}`,
        stream,
      ),
    );
  }

  async updateBlockTarget(workspaceSlug: string, req: UpdateBlockRequest): Promise<void> {
    await this.fetchJSON(`${this.ep(workspaceSlug)}/${req.project_id}/blocks/${req.block_id}`, {
      method: "PUT",
      body: JSON.stringify(req),
    });
  }

  async updateBlockTargetCoded(
    workspaceSlug: string,
    req: UpdateBlockTargetCodedRequest,
  ): Promise<void> {
    await this.fetchJSON(
      `${this.ep(workspaceSlug)}/${req.project_id}/blocks/${req.block_id}/coded`,
      { method: "PUT", body: JSON.stringify(req) },
    );
  }

  async pseudoTranslateFile(
    workspaceSlug: string,
    projectId: string,
    fileName: string,
    targetLocale: string,
    stream?: string,
  ): Promise<TranslationStats> {
    return this.fetchJSON(
      this.withStream(
        `${this.ep(workspaceSlug)}/${projectId}/file-pseudo/${this.encodeFilePath(fileName)}`,
        stream,
      ),
      { method: "POST", body: JSON.stringify({ target_locale: targetLocale }) },
    );
  }

  async aiTranslateFile(
    workspaceSlug: string,
    req: AITranslateFileRequest,
  ): Promise<TranslationStats> {
    return this.fetchJSON(
      `${this.ep(workspaceSlug)}/${req.project_id}/file-ai-translate/${encodeURIComponent(req.item_name)}`,
      { method: "POST", body: JSON.stringify(req) },
    );
  }

  async tmTranslateFile(
    workspaceSlug: string,
    projectId: string,
    fileName: string,
    targetLocale: string,
    stream?: string,
  ): Promise<TranslationStats> {
    return this.fetchJSON(
      this.withStream(
        `${this.ep(workspaceSlug)}/${projectId}/file-tm-translate/${this.encodeFilePath(fileName)}`,
        stream,
      ),
      { method: "POST", body: JSON.stringify({ target_locale: targetLocale }) },
    );
  }

  async getWordCount(
    workspaceSlug: string,
    projectId: string,
    fileName: string,
    stream?: string,
  ): Promise<WordCountResult> {
    return this.fetchJSON(
      this.withStream(
        `${this.ep(workspaceSlug)}/${projectId}/file-wordcount/${this.encodeFilePath(fileName)}`,
        stream,
      ),
    );
  }

  async exportTranslatedFile(
    workspaceSlug: string,
    projectId: string,
    fileName: string,
    targetLocale: string,
    stream?: string,
  ): Promise<Blob> {
    return this.fetchBlob(
      this.withStream(
        `${this.ep(workspaceSlug)}/${projectId}/file-export/${this.encodeFilePath(fileName)}`,
        stream,
      ),
      { method: "POST", body: JSON.stringify({ target_locale: targetLocale }) },
    );
  }

  async lookupTMForBlock(
    workspaceSlug: string,
    projectId: string,
    itemName: string,
    blockId: string,
    targetLocale: string,
    stream?: string,
  ): Promise<TMMatchInfo[]> {
    return this.fetchJSON(
      this.withStream(
        `${this.ep(workspaceSlug)}/${projectId}/blocks/${blockId}/tm-lookup?item=${encodeURIComponent(itemName)}&target_locale=${targetLocale}`,
        stream,
      ),
    );
  }

  async lookupTermsForBlock(
    workspaceSlug: string,
    projectId: string,
    itemName: string,
    blockId: string,
    targetLocale: string,
    stream?: string,
  ): Promise<BlockTermMatch[]> {
    return this.fetchJSON(
      this.withStream(
        `${this.ep(workspaceSlug)}/${projectId}/blocks/${blockId}/term-lookup?item=${encodeURIComponent(itemName)}&target_locale=${targetLocale}`,
        stream,
      ),
    );
  }

  // ── Block History ────────────────────────────────────────────────────────

  async getBlockHistory(
    workspaceSlug: string,
    projectId: string,
    blockId: string,
    locale: string,
    limit?: number,
    stream?: string,
  ): Promise<BlockHistoryEntry[]> {
    const params = new URLSearchParams({ locale });
    if (limit) params.set("limit", String(limit));
    return this.fetchJSON(
      this.withStream(
        `${this.ep(workspaceSlug)}/${projectId}/blocks/${blockId}/history?${params}`,
        stream,
      ),
    );
  }

  // ── Block Notes ──────────────────────────────────────────────────────────

  async addBlockNote(
    workspaceSlug: string,
    projectId: string,
    blockId: string,
    text: string,
  ): Promise<BlockNote> {
    return this.fetchJSON(`${this.ep(workspaceSlug)}/${projectId}/blocks/${blockId}/notes`, {
      method: "POST",
      body: JSON.stringify({ text }),
    });
  }

  async listBlockNotes(
    workspaceSlug: string,
    projectId: string,
    blockId: string,
  ): Promise<BlockNote[]> {
    return this.fetchJSON(`${this.ep(workspaceSlug)}/${projectId}/blocks/${blockId}/notes`);
  }

  async deleteBlockNote(workspaceSlug: string, projectId: string, noteId: string): Promise<void> {
    // Note: the route includes block ID in the path, but for deletion we use a
    // placeholder since the server only needs project ID and note ID.
    await this.fetchJSON(`${this.ep(workspaceSlug)}/${projectId}/blocks/_/notes/${noteId}`, {
      method: "DELETE",
    });
  }

  // ── QA ─────────────────────────────────────────────────────────────────

  async runQACheck(
    workspaceSlug: string,
    projectId: string,
    blockId: string,
    locale: string,
    stream?: string,
  ): Promise<QAIssue[]> {
    return this.fetchJSON(
      this.withStream(
        `${this.ep(workspaceSlug)}/${projectId}/blocks/${blockId}/qa-check?locale=${locale}`,
        stream,
      ),
      { method: "POST" },
    );
  }

  async runFileQACheck(
    workspaceSlug: string,
    projectId: string,
    fileName: string,
    locale: string,
    stream?: string,
  ): Promise<FileQAResult[]> {
    return this.fetchJSON(
      this.withStream(
        `${this.ep(workspaceSlug)}/${projectId}/file-qa-check/${this.encodeFilePath(fileName)}?locale=${locale}`,
        stream,
      ),
      { method: "POST" },
    );
  }

  // ── Preview ────────────────────────────────────────────────────────────

  async renderDocumentPreview(
    workspaceSlug: string,
    projectId: string,
    fileName: string,
    targetLocale: string,
    stream?: string,
  ): Promise<string> {
    return this.fetchText(
      this.withStream(
        `${this.ep(workspaceSlug)}/${projectId}/file-preview/${this.encodeFilePath(fileName)}?locale=${targetLocale}`,
        stream,
      ),
    );
  }

  async renderBlockHTML(
    workspaceSlug: string,
    projectId: string,
    blockId: string,
    targetLocale: string,
    stream?: string,
  ): Promise<string> {
    return this.fetchText(
      this.withStream(
        `${this.ep(workspaceSlug)}/${projectId}/blocks/${blockId}/html?locale=${targetLocale}`,
        stream,
      ),
    );
  }

  // ── Translation Memory ───────────────────────────────────────────────────

  private tmEp(ws: string) {
    return `/api/v1/workspaces/${ws}/tm`;
  }

  async getTMEntries(
    workspaceSlug: string,
    query: string,
    defaultSourceLanguage: string,
    targetLocale: string,
    offset: number,
    limit: number,
  ): Promise<TMSearchResult> {
    const params = new URLSearchParams({
      q: query,
      default_source_language: defaultSourceLanguage,
      target_locale: targetLocale,
      offset: String(offset),
      limit: String(limit),
    });
    return this.fetchJSON(`${this.tmEp(workspaceSlug)}?${params}`);
  }

  async getTMCount(workspaceSlug: string): Promise<number> {
    const resp: { count: number } = await this.fetchJSON(`${this.tmEp(workspaceSlug)}/count`);
    return resp.count;
  }

  async addTMEntry(
    workspaceSlug: string,
    source: string,
    target: string,
    defaultSourceLanguage: string,
    targetLocale: string,
  ): Promise<TMEntryInfo> {
    return this.fetchJSON(this.tmEp(workspaceSlug), {
      method: "POST",
      body: JSON.stringify({
        source,
        target,
        default_source_language: defaultSourceLanguage,
        target_locale: targetLocale,
      }),
    });
  }

  async updateTMEntry(workspaceSlug: string, req: TMUpdateRequest): Promise<void> {
    await this.fetchJSON(`${this.tmEp(workspaceSlug)}/${req.entry_id}`, {
      method: "PUT",
      body: JSON.stringify(req),
    });
  }

  async deleteTMEntry(workspaceSlug: string, entryId: string): Promise<void> {
    await this.fetchJSON(`${this.tmEp(workspaceSlug)}/${entryId}`, { method: "DELETE" });
  }

  // ── Terminology ──────────────────────────────────────────────────────────

  private termsEp(ws: string) {
    return `/api/v1/workspaces/${ws}/terms`;
  }

  async getTerms(
    workspaceSlug: string,
    query: string,
    defaultSourceLanguage: string,
    targetLocale: string,
    offset: number,
    limit: number,
  ): Promise<TermSearchResult> {
    const params = new URLSearchParams({
      q: query,
      default_source_language: defaultSourceLanguage,
      target_locale: targetLocale,
      offset: String(offset),
      limit: String(limit),
    });
    return this.fetchJSON(`${this.termsEp(workspaceSlug)}?${params}`);
  }

  async getTermCount(workspaceSlug: string): Promise<number> {
    const resp: { count: number } = await this.fetchJSON(`${this.termsEp(workspaceSlug)}/count`);
    return resp.count;
  }

  async addConcept(workspaceSlug: string, req: AddConceptRequest): Promise<ConceptInfo> {
    return this.fetchJSON(this.termsEp(workspaceSlug), {
      method: "POST",
      body: JSON.stringify(req),
    });
  }

  async updateConcept(workspaceSlug: string, req: UpdateConceptRequest): Promise<void> {
    await this.fetchJSON(`${this.termsEp(workspaceSlug)}/${req.concept_id}`, {
      method: "PUT",
      body: JSON.stringify(req),
    });
  }

  async deleteConcept(workspaceSlug: string, conceptId: string): Promise<void> {
    await this.fetchJSON(`${this.termsEp(workspaceSlug)}/${conceptId}`, { method: "DELETE" });
  }

  async importTermsCSV(
    workspaceSlug: string,
    csvContent: string,
    defaultSourceLanguage: string,
    targetLocale: string,
    domain: string,
    hasHeader: boolean,
  ): Promise<number> {
    const resp: { imported: number } = await this.fetchJSON(
      `${this.termsEp(workspaceSlug)}/import/csv`,
      {
        method: "POST",
        body: JSON.stringify({
          csv_content: csvContent,
          default_source_language: defaultSourceLanguage,
          target_locale: targetLocale,
          domain,
          has_header: hasHeader,
        }),
      },
    );
    return resp.imported;
  }

  async importTermsJSON(workspaceSlug: string, jsonContent: string): Promise<number> {
    const resp: { imported: number } = await this.fetchJSON(
      `${this.termsEp(workspaceSlug)}/import/json`,
      {
        method: "POST",
        body: JSON.stringify({ json_content: jsonContent }),
      },
    );
    return resp.imported;
  }

  async exportTermsJSON(workspaceSlug: string, name: string): Promise<string> {
    return this.fetchJSON(
      `${this.termsEp(workspaceSlug)}/export/json?name=${encodeURIComponent(name)}`,
    );
  }

  // ── Providers ────────────────────────────────────────────────────────────

  async listProviderConfigs(workspaceSlug: string): Promise<ProviderConfig[]> {
    return this.fetchJSON(`/api/v1/workspaces/${workspaceSlug}/providers`);
  }

  async saveProviderConfig(
    workspaceSlug: string,
    cfg: ProviderConfigWithKey,
  ): Promise<ProviderConfig> {
    return this.fetchJSON(`/api/v1/workspaces/${workspaceSlug}/providers`, {
      method: "POST",
      body: JSON.stringify(cfg),
    });
  }

  async deleteProviderConfig(workspaceSlug: string, id: string): Promise<void> {
    await this.fetchJSON(`/api/v1/workspaces/${workspaceSlug}/providers/${id}`, {
      method: "DELETE",
    });
  }

  async testProviderConfig(workspaceSlug: string, cfg: ProviderConfigWithKey): Promise<void> {
    await this.fetchJSON(`/api/v1/workspaces/${workspaceSlug}/providers/test`, {
      method: "POST",
      body: JSON.stringify(cfg),
    });
  }

  // ── Automations ────────────────────────────────────────────────────────

  private automationsEp(ws: string, projectId: string) {
    return `/api/v1/workspaces/${ws}/projects/${encodeURIComponent(projectId)}/automations`;
  }

  async listAutomationRules(workspaceSlug: string, projectId: string): Promise<AutomationRule[]> {
    return this.fetchJSON(this.automationsEp(workspaceSlug, projectId));
  }

  async createAutomationRule(
    workspaceSlug: string,
    projectId: string,
    data: SaveAutomationRuleRequest,
  ): Promise<AutomationRule> {
    return this.fetchJSON(this.automationsEp(workspaceSlug, projectId), {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  async updateAutomationRule(
    workspaceSlug: string,
    projectId: string,
    ruleId: string,
    data: SaveAutomationRuleRequest,
  ): Promise<AutomationRule> {
    return this.fetchJSON(
      `${this.automationsEp(workspaceSlug, projectId)}/${encodeURIComponent(ruleId)}`,
      {
        method: "PUT",
        body: JSON.stringify(data),
      },
    );
  }

  async deleteAutomationRule(
    workspaceSlug: string,
    projectId: string,
    ruleId: string,
  ): Promise<void> {
    await this.fetchJSON(
      `${this.automationsEp(workspaceSlug, projectId)}/${encodeURIComponent(ruleId)}`,
      {
        method: "DELETE",
      },
    );
  }

  async toggleAutomationRule(
    workspaceSlug: string,
    projectId: string,
    ruleId: string,
  ): Promise<AutomationRule> {
    return this.fetchJSON(
      `${this.automationsEp(workspaceSlug, projectId)}/${encodeURIComponent(ruleId)}/toggle`,
      {
        method: "PATCH",
      },
    );
  }

  async listAutomationEvents(workspaceSlug: string, projectId: string): Promise<AutomationEvent[]> {
    return this.fetchJSON(`${this.automationsEp(workspaceSlug, projectId)}/events`);
  }

  async listAutomationHistory(
    workspaceSlug: string,
    projectId: string,
  ): Promise<AutomationHistoryEntry[]> {
    return this.fetchJSON(`${this.automationsEp(workspaceSlug, projectId)}/history`);
  }

  // ── Notifications ──────────────────────────────────────────────────────

  async listNotifications(
    workspaceSlug: string,
    limit?: number,
    unreadOnly?: boolean,
  ): Promise<{ notifications: NotificationInfo[]; unread_count: number }> {
    const params = new URLSearchParams();
    if (limit) params.set("limit", String(limit));
    if (unreadOnly) params.set("unread", "true");
    return this.fetchJSON(`/api/v1/workspaces/${workspaceSlug}/notifications?${params}`);
  }

  async markNotificationRead(workspaceSlug: string, id: string): Promise<void> {
    await this.fetchJSON(`/api/v1/workspaces/${workspaceSlug}/notifications/${id}/read`, {
      method: "PUT",
    });
  }

  async markAllNotificationsRead(workspaceSlug: string): Promise<void> {
    await this.fetchJSON(`/api/v1/workspaces/${workspaceSlug}/notifications/read-all`, {
      method: "PUT",
    });
  }

  async deleteNotification(workspaceSlug: string, id: string): Promise<void> {
    await this.fetchJSON(`/api/v1/workspaces/${workspaceSlug}/notifications/${id}`, {
      method: "DELETE",
    });
  }

  // ── Entity Annotations ──────────────────────────────────────────────────

  async createEntity(
    workspaceSlug: string,
    projectId: string,
    itemName: string,
    blockId: string,
    entity: Partial<EntityInfo>,
  ): Promise<EntityInfo> {
    return this.fetchJSON(`${this.ep(workspaceSlug)}/${projectId}/blocks/${blockId}/entities`, {
      method: "POST",
      body: JSON.stringify({ item_name: itemName, ...entity }),
    });
  }

  async updateEntity(
    workspaceSlug: string,
    projectId: string,
    itemName: string,
    blockId: string,
    entityKey: string,
    entity: Partial<EntityInfo>,
  ): Promise<EntityInfo> {
    return this.fetchJSON(
      `${this.ep(workspaceSlug)}/${projectId}/blocks/${blockId}/entities/${encodeURIComponent(entityKey)}`,
      { method: "PUT", body: JSON.stringify({ item_name: itemName, ...entity }) },
    );
  }

  async deleteEntity(
    workspaceSlug: string,
    projectId: string,
    itemName: string,
    blockId: string,
    entityKey: string,
  ): Promise<void> {
    await this.fetchJSON(
      `${this.ep(workspaceSlug)}/${projectId}/blocks/${blockId}/entities/${encodeURIComponent(entityKey)}?item_name=${encodeURIComponent(itemName)}`,
      { method: "DELETE" },
    );
  }

  async promoteEntity(
    workspaceSlug: string,
    projectId: string,
    itemName: string,
    blockId: string,
    entityKey: string,
  ): Promise<void> {
    await this.fetchJSON(
      `${this.ep(workspaceSlug)}/${projectId}/blocks/${blockId}/entities/${encodeURIComponent(entityKey)}/promote`,
      { method: "POST", body: JSON.stringify({ item_name: itemName }) },
    );
  }

  // ── Brand Voice ──────────────────────────────────────────────────────────

  private brandEp(ws: string) {
    return `/api/v1/workspaces/${ws}/brand-profiles`;
  }

  async listBrandProfiles(workspaceSlug: string): Promise<VoiceProfile[]> {
    return this.fetchJSON(this.brandEp(workspaceSlug));
  }

  async getBrandProfile(workspaceSlug: string, profileId: string): Promise<VoiceProfile> {
    return this.fetchJSON(`${this.brandEp(workspaceSlug)}/${encodeURIComponent(profileId)}`);
  }

  async createBrandProfile(
    workspaceSlug: string,
    data: CreateVoiceProfileRequest,
  ): Promise<VoiceProfile> {
    return this.fetchJSON(this.brandEp(workspaceSlug), {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  async updateBrandProfile(
    workspaceSlug: string,
    data: UpdateVoiceProfileRequest,
  ): Promise<VoiceProfile> {
    return this.fetchJSON(`${this.brandEp(workspaceSlug)}/${encodeURIComponent(data.id)}`, {
      method: "PUT",
      body: JSON.stringify(data),
    });
  }

  async deleteBrandProfile(workspaceSlug: string, profileId: string): Promise<void> {
    await this.fetchJSON(`${this.brandEp(workspaceSlug)}/${encodeURIComponent(profileId)}`, {
      method: "DELETE",
    });
  }

  async getBrandScores(workspaceSlug: string, projectId: string): Promise<StoredScore[]> {
    return this.fetchJSON(
      `/api/v1/workspaces/${workspaceSlug}/brand/scores?project_id=${encodeURIComponent(projectId)}`,
    );
  }

  async getBrandTrends(workspaceSlug: string, projectId: string): Promise<ScoreTrend[]> {
    return this.fetchJSON(
      `/api/v1/workspaces/${workspaceSlug}/brand/trends?project_id=${encodeURIComponent(projectId)}`,
    );
  }

  // ── Audit Log ───────────────────────────────────────────────────────────

  async listWorkspaceAuditLog(
    workspaceSlug: string,
    query?: AuditQuery,
  ): Promise<AuditEntry[]> {
    const params = new URLSearchParams();
    if (query?.project) params.set("project", query.project);
    if (query?.type) params.set("type", query.type);
    if (query?.actor) params.set("actor", query.actor);
    if (query?.search) params.set("search", query.search);
    if (query?.limit) params.set("limit", String(query.limit));
    if (query?.offset) params.set("offset", String(query.offset));
    const qs = params.toString();
    return this.fetchJSON(
      `/api/v1/workspaces/${workspaceSlug}/audit-log${qs ? `?${qs}` : ""}`,
    );
  }

  // ── Utility ──────────────────────────────────────────────────────────────

  async getKnownLocales(): Promise<LocaleInfo[]> {
    return this.fetchJSON("/api/v1/locales");
  }

  async listFormats(): Promise<FormatInfo[]> {
    return this.fetchJSON("/api/v1/formats");
  }

  async listTools(): Promise<ToolInfo[]> {
    return this.fetchJSON("/api/v1/tools");
  }
}
