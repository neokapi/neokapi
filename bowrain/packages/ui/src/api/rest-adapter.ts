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
  FlowDefinitionInfo,
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
  AutomationRun,
  AutomationStep,
  AutomationLogEntry,
  SaveAutomationRuleRequest,
  NotificationInfo,
  EntityInfo,
  StreamInfo,
  StreamDiffResult,
  StreamMergeResult,
  CreateStreamRequest,
  StreamTag,
  StreamTagKind,
  CreateStreamTagRequest,
  CollectionInfo,
  CreateCollectionRequest,
  AuditEntry,
  AuditQuery,
  AuditChainVerification,
  BlockWorkflowStatus,
  SoDMode,
  Group,
  GroupRoleBinding,
  DenyRule,
  DenyRuleInput,
  RestorePointOptions,
  ArchivedProject,
  TranslationDashboardStats,
  ActivityInfo,
  TaskInfo,
  CreateTaskRequest,
  NotificationPreference,
  BravoConversation,
  BravoMessage,
  BravoConfig,
  BravoToolInfo,
  BravoUsageSummary,
  BravoSSEHandler,
  BravoSSEMessageStart,
  BravoSSEContentDelta,
  BravoSSEToolCallStart,
  BravoSSEToolCallEnd,
  BravoSSENeedsApproval,
  BravoSSEMessageEnd,
  BravoSSEError,
  BravoSSEStepUp,
  BillingOverview,
  BillingUsageBreakdown,
  ModelUsageResponse,
  CreditLedgerEntry,
  RoleTemplate,
  ProjectMembership,
  DigestSettingsDTO,
  OnboardingStatus,
  SlugCheckResponse,
  EmailChangeRequestResponse,
  EmailChangeConfirmResponse,
  SlugReservation,
} from "../types/api";
import type {
  VoiceProfile,
  StoredScore,
  ScoreTrend,
  CreateVoiceProfileRequest,
  UpdateVoiceProfileRequest,
  CandidateRule,
  BlastRadius,
  DriftResult,
} from "../brand/types";
import type {
  ListConceptsParams,
  ConceptStory,
  ConceptRelation,
  ConceptUsage,
  AddConceptRelationRequest,
  RelationScope,
  Observation,
  AddObservationRequest,
  Comment,
  AddCommentRequest,
  Market,
  MarketRequest,
  ChangeSet,
  ChangeSetDetail,
  ChangeSetStatus,
  ChangeSetOp,
  AddChangeSetOpRequest,
  CreateChangeSetRequest,
  UpdateChangeSetRequest,
  ReviewRequest,
  ChangeSetImpact,
  MergeResult,
  Pilot,
  StartPilotRequest,
} from "../types/brand-graph";

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
      headers: {
        ...this.headers(),
        ...(init?.headers as Record<string, string>),
      },
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
          headers: {
            ...this.headers(),
            ...(init?.headers as Record<string, string>),
          },
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
      // Block forever — the page is redirecting to login. Throwing would cause
      // React Query to flash error UI before the navigation completes.
      return new Promise<T>(() => {});
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
      headers: {
        ...this.headers(),
        ...(init?.headers as Record<string, string>),
      },
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
      headers: {
        ...this.headers(),
        ...(init?.headers as Record<string, string>),
      },
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
          headers: {
            ...this.headers(),
            ...(init?.headers as Record<string, string>),
          },
          credentials: "same-origin",
        });
        if (!retry.ok) {
          const body = await retry.text();
          throw new Error(`${retry.status}: ${body}`);
        }
        return retry.text();
      }
      this.onSessionExpired?.();
      return new Promise<string>(() => {});
    }
    if (!resp.ok) {
      const body = await resp.text();
      throw new Error(`${resp.status}: ${body}`);
    }
    return resp.text();
  }

  /** Returns the ref path segment (defaults to "main"). */
  private ref(stream?: string): string {
    return encodeURIComponent(stream || "main");
  }

  /** Project prefix: /api/v1/{ws}/{projectId} */
  private projectEp(ws: string, projectId: string): string {
    return `/api/v1/${ws}/${encodeURIComponent(projectId)}`;
  }

  // ── Config ──────────────────────────────────────────────────────────────

  async getConfig(): Promise<ConfigResponse> {
    return this.fetchJSON("/api/v1/info");
  }

  // ── Auth ─────────────────────────────────────────────────────────────────

  async getCurrentUser(): Promise<User | null> {
    // Bypass the standard fetchJSON 401-handling path. fetchJSON treats 401
    // as "session expired" and redirects via onSessionExpired, returning a
    // never-resolving promise to suppress error flicker during navigation.
    // That's wrong for getCurrentUser — this method IS the check for
    // "do we have a session?", and an unauthenticated 401 is the expected
    // negative answer, not a session-expiry. Returning null lets callers
    // (ClaimPage, JoinPage) render their unauthenticated UI instead of
    // hanging on the loading spinner forever.
    try {
      const resp = await fetch(`${this.baseUrl}/api/v1/auth/me`, {
        headers: this.headers(),
        credentials: "same-origin",
      });
      if (!resp.ok) return null;
      return (await resp.json()) as User;
    } catch {
      return null;
    }
  }

  // ── Account management ──────────────────────────────────────────────────
  // Onboarding picks the personal-workspace handle on first sign-in. Email
  // change is Bowrain-managed: a verification link is sent to the new
  // address; confirmation writes through to Keycloak via its admin API and
  // updates the local user record.

  async getOnboardingStatus(): Promise<OnboardingStatus> {
    return this.fetchJSON("/api/v1/auth/me/onboarding");
  }

  async completeOnboarding(slug: string, displayName?: string): Promise<Workspace> {
    return this.fetchJSON("/api/v1/auth/me/onboarding", {
      method: "POST",
      body: JSON.stringify({ slug, display_name: displayName ?? "" }),
    });
  }

  async checkSlug(slug: string): Promise<SlugCheckResponse> {
    return this.fetchJSON(`/api/v1/auth/check-slug?slug=${encodeURIComponent(slug)}`);
  }

  async requestEmailChange(newEmail: string): Promise<EmailChangeRequestResponse> {
    return this.fetchJSON("/api/v1/auth/me/email", {
      method: "POST",
      body: JSON.stringify({ new_email: newEmail }),
    });
  }

  async confirmEmailChange(token: string): Promise<EmailChangeConfirmResponse> {
    return this.fetchJSON("/api/v1/auth/email/confirm", {
      method: "POST",
      body: JSON.stringify({ token }),
    });
  }

  async adminListSlugReservations(): Promise<SlugReservation[]> {
    return this.fetchJSON("/api/admin/slug-reservations");
  }

  async adminReleaseSlugReservation(slug: string): Promise<void> {
    await this.fetchJSON("/api/admin/slug-reservations/release", {
      method: "POST",
      body: JSON.stringify({ slug }),
    });
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
    return this.fetchJSON(`/api/v1/${slug}`);
  }

  async updateWorkspace(slug: string, data: Partial<Workspace>): Promise<Workspace> {
    return this.fetchJSON(`/api/v1/${slug}`, {
      method: "PATCH",
      body: JSON.stringify(data),
    });
  }

  async deleteWorkspace(slug: string): Promise<void> {
    await this.fetchJSON(`/api/v1/${slug}`, { method: "DELETE" });
  }

  // ── Members ──────────────────────────────────────────────────────────────

  async listMembers(workspaceSlug: string): Promise<Membership[]> {
    return this.fetchJSON(`/api/v1/${workspaceSlug}/members`);
  }

  async addMember(workspaceSlug: string, userId: string, role: string): Promise<void> {
    await this.fetchJSON(`/api/v1/${workspaceSlug}/members`, {
      method: "POST",
      body: JSON.stringify({ user_id: userId, role }),
    });
  }

  async updateMemberRole(workspaceSlug: string, userId: string, role: string): Promise<void> {
    await this.fetchJSON(`/api/v1/${workspaceSlug}/members/${userId}/role`, {
      method: "PUT",
      body: JSON.stringify({ role }),
    });
  }

  async removeMember(workspaceSlug: string, userId: string): Promise<void> {
    await this.fetchJSON(`/api/v1/${workspaceSlug}/members/${userId}`, {
      method: "DELETE",
    });
  }

  // ── Invites ──────────────────────────────────────────────────────────────

  async listInvites(workspaceSlug: string): Promise<Invite[]> {
    return this.fetchJSON(`/api/v1/${workspaceSlug}/invites`);
  }

  async createInvite(
    workspaceSlug: string,
    email: string,
    role: string,
    maxUses: number,
  ): Promise<Invite> {
    return this.fetchJSON(`/api/v1/${workspaceSlug}/invites`, {
      method: "POST",
      body: JSON.stringify({ email, role, max_uses: maxUses }),
    });
  }

  async deleteInvite(workspaceSlug: string, inviteId: string): Promise<void> {
    await this.fetchJSON(`/api/v1/${workspaceSlug}/invites/${inviteId}`, {
      method: "DELETE",
    });
  }

  // ── Role Templates ───────────────────────────────────────────────────

  async listRoleTemplates(workspaceSlug: string): Promise<RoleTemplate[]> {
    return this.fetchJSON(`/api/v1/${workspaceSlug}/roles`);
  }

  async createRoleTemplate(
    workspaceSlug: string,
    data: {
      name: string;
      display_name: string;
      description: string;
      permissions: string[];
      position?: number;
    },
  ): Promise<RoleTemplate> {
    return this.fetchJSON(`/api/v1/${workspaceSlug}/roles`, {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  async updateRoleTemplate(
    workspaceSlug: string,
    roleId: string,
    data: {
      name?: string;
      display_name?: string;
      description?: string;
      permissions?: string[];
      position?: number;
    },
  ): Promise<RoleTemplate> {
    return this.fetchJSON(`/api/v1/${workspaceSlug}/roles/${roleId}`, {
      method: "PUT",
      body: JSON.stringify(data),
    });
  }

  async deleteRoleTemplate(workspaceSlug: string, roleId: string): Promise<void> {
    await this.fetchJSON(`/api/v1/${workspaceSlug}/roles/${roleId}`, {
      method: "DELETE",
    });
  }

  // ── Project Members ─────────────────────────────────────────────────

  async listProjectMembers(workspaceSlug: string, projectId: string): Promise<ProjectMembership[]> {
    return this.fetchJSON(`${this.projectEp(workspaceSlug, projectId)}/members`);
  }

  async addProjectMember(
    workspaceSlug: string,
    projectId: string,
    data: {
      user_id: string;
      role_id: string;
      languages?: string[];
    },
  ): Promise<ProjectMembership> {
    return this.fetchJSON(`${this.projectEp(workspaceSlug, projectId)}/members`, {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  async updateProjectMember(
    workspaceSlug: string,
    projectId: string,
    userId: string,
    data: {
      role_id: string;
      languages?: string[];
    },
  ): Promise<ProjectMembership> {
    return this.fetchJSON(`${this.projectEp(workspaceSlug, projectId)}/members/${userId}`, {
      method: "PUT",
      body: JSON.stringify(data),
    });
  }

  async removeProjectMember(
    workspaceSlug: string,
    projectId: string,
    userId: string,
  ): Promise<void> {
    await this.fetchJSON(`${this.projectEp(workspaceSlug, projectId)}/members/${userId}`, {
      method: "DELETE",
    });
  }

  // ── API Tokens ─────────────────────────────────────────────────────────

  async listApiTokens(workspaceSlug: string): Promise<ApiToken[]> {
    return this.fetchJSON(`/api/v1/${workspaceSlug}/tokens`);
  }

  async createApiToken(
    workspaceSlug: string,
    name: string,
    expireDays: number,
    scopes?: string[],
  ): Promise<CreateApiTokenResponse> {
    const body: Record<string, unknown> = { name, expire_days: expireDays };
    if (scopes && scopes.length > 0) {
      body.scopes = scopes;
    }
    return this.fetchJSON(`/api/v1/${workspaceSlug}/tokens`, {
      method: "POST",
      body: JSON.stringify(body),
    });
  }

  async deleteApiToken(workspaceSlug: string, tokenId: string): Promise<void> {
    await this.fetchJSON(`/api/v1/${workspaceSlug}/tokens/${tokenId}`, {
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
    return `${this.projectEp(ws, projectId)}/streams`;
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

  async lockStream(
    workspaceSlug: string,
    projectId: string,
    streamName: string,
  ): Promise<StreamInfo> {
    return this.fetchJSON(
      `${this.streamEp(workspaceSlug, projectId)}/${encodeURIComponent(streamName)}/lock`,
      { method: "POST" },
    );
  }

  async unlockStream(
    workspaceSlug: string,
    projectId: string,
    streamName: string,
  ): Promise<StreamInfo> {
    return this.fetchJSON(
      `${this.streamEp(workspaceSlug, projectId)}/${encodeURIComponent(streamName)}/unlock`,
      { method: "POST" },
    );
  }

  // ── Stream Tags ────────────────────────────────────────────────────────────

  async listStreamTags(
    workspaceSlug: string,
    projectId: string,
    streamName: string,
  ): Promise<StreamTag[]> {
    return this.fetchJSON(
      `${this.streamEp(workspaceSlug, projectId)}/${encodeURIComponent(streamName)}/tags`,
    );
  }

  async createStreamTag(
    workspaceSlug: string,
    projectId: string,
    streamName: string,
    req: CreateStreamTagRequest,
  ): Promise<StreamTag> {
    return this.fetchJSON(
      `${this.streamEp(workspaceSlug, projectId)}/${encodeURIComponent(streamName)}/tags`,
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(req),
      },
    );
  }

  async getStreamTag(
    workspaceSlug: string,
    projectId: string,
    streamName: string,
    tagName: string,
  ): Promise<StreamTag> {
    return this.fetchJSON(
      `${this.streamEp(workspaceSlug, projectId)}/${encodeURIComponent(streamName)}/tags/${encodeURIComponent(tagName)}`,
    );
  }

  async deleteStreamTag(
    workspaceSlug: string,
    projectId: string,
    streamName: string,
    tagName: string,
  ): Promise<void> {
    await this.fetchJSON(
      `${this.streamEp(workspaceSlug, projectId)}/${encodeURIComponent(streamName)}/tags/${encodeURIComponent(tagName)}`,
      { method: "DELETE" },
    );
  }

  async listProjectTags(
    workspaceSlug: string,
    projectId: string,
    kind?: StreamTagKind,
  ): Promise<StreamTag[]> {
    const params = kind ? `?kind=${encodeURIComponent(kind)}` : "";
    return this.fetchJSON(`${this.projectEp(workspaceSlug, projectId)}/tags${params}`);
  }

  // ── Projects ─────────────────────────────────────────────────────────────

  async listProjects(workspaceSlug: string): Promise<ProjectInfo[]> {
    return this.fetchJSON(`/api/v1/${workspaceSlug}/projects`);
  }

  async createProject(
    workspaceSlug: string,
    name: string,
    defaultSourceLanguage: string,
    targetLanguages: string[],
  ): Promise<ProjectInfo> {
    return this.fetchJSON(`/api/v1/${workspaceSlug}/projects`, {
      method: "POST",
      body: JSON.stringify({
        name,
        default_source_language: defaultSourceLanguage,
        target_languages: targetLanguages,
      }),
    });
  }

  async getProject(
    workspaceSlug: string,
    projectId: string,
    _stream?: string,
  ): Promise<ProjectInfo> {
    return this.fetchJSON(this.projectEp(workspaceSlug, projectId));
  }

  async updateProject(
    workspaceSlug: string,
    projectId: string,
    data: {
      name?: string;
      target_languages?: string[];
      dashboard_visibility?: string;
      properties?: Record<string, string>;
    },
  ): Promise<ProjectInfo> {
    return this.fetchJSON(this.projectEp(workspaceSlug, projectId), {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(data),
    });
  }

  async deleteProject(workspaceSlug: string, projectId: string): Promise<void> {
    await this.fetchJSON(this.projectEp(workspaceSlug, projectId), {
      method: "DELETE",
    });
  }

  async restoreProject(workspaceSlug: string, projectId: string): Promise<void> {
    await this.fetchJSON(`${this.projectEp(workspaceSlug, projectId)}/restore`, {
      method: "POST",
    });
  }

  async permanentlyDeleteProject(workspaceSlug: string, projectId: string): Promise<void> {
    await this.fetchJSON(`${this.projectEp(workspaceSlug, projectId)}/permanent`, {
      method: "DELETE",
    });
  }

  async listArchivedProjects(workspaceSlug: string): Promise<ArchivedProject[]> {
    return this.fetchJSON(`/api/v1/${workspaceSlug}/archived-projects`);
  }

  async restoreStream(workspaceSlug: string, projectId: string, streamName: string): Promise<void> {
    await this.fetchJSON(
      `${this.streamEp(workspaceSlug, projectId)}/${encodeURIComponent(streamName)}/restore`,
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
    const url = `${this.projectEp(workspaceSlug, projectId)}/items/${this.ref(stream)}`;
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
      `${this.projectEp(workspaceSlug, projectId)}/items/${this.ref(stream)}?item=${encodeURIComponent(fileName)}`,
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
      `${this.projectEp(workspaceSlug, projectId)}/collections/${this.ref(stream)}`,
    );
  }

  async createCollection(
    workspaceSlug: string,
    projectId: string,
    req: CreateCollectionRequest,
  ): Promise<CollectionInfo> {
    return this.fetchJSON(`${this.projectEp(workspaceSlug, projectId)}/collections`, {
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
    return this.fetchJSON(
      `${this.projectEp(workspaceSlug, projectId)}/collections/${collectionId}`,
    );
  }

  async updateCollection(
    workspaceSlug: string,
    projectId: string,
    collectionId: string,
    req: Partial<CreateCollectionRequest>,
  ): Promise<CollectionInfo> {
    return this.fetchJSON(
      `${this.projectEp(workspaceSlug, projectId)}/collections/${collectionId}`,
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
      `${this.projectEp(workspaceSlug, projectId)}/collections/${collectionId}`,
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
    const url = `${this.projectEp(workspaceSlug, projectId)}/collections/${collectionId}/files/${this.ref(stream)}`;
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
      `${this.projectEp(workspaceSlug, projectId)}/blocks/${this.ref(stream)}?item=${encodeURIComponent(fileName)}`,
    );
  }

  async updateBlockTarget(workspaceSlug: string, req: UpdateBlockRequest): Promise<void> {
    await this.fetchJSON(
      `${this.projectEp(workspaceSlug, req.project_id)}/blocks/${this.ref(req.stream)}/${req.block_id}`,
      {
        method: "PUT",
        body: JSON.stringify(req),
      },
    );
  }

  async updateBlockTargetCoded(
    workspaceSlug: string,
    req: UpdateBlockTargetCodedRequest,
  ): Promise<void> {
    await this.fetchJSON(
      `${this.projectEp(workspaceSlug, req.project_id)}/blocks/${this.ref(req.stream)}/${req.block_id}/coded`,
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
      `${this.projectEp(workspaceSlug, projectId)}/actions/${this.ref(stream)}/pseudo-translate`,
      {
        method: "POST",
        body: JSON.stringify({ item: fileName, target_locale: targetLocale }),
      },
    );
  }

  async aiTranslateFile(
    workspaceSlug: string,
    req: AITranslateFileRequest,
  ): Promise<TranslationStats> {
    return this.fetchJSON(
      `${this.projectEp(workspaceSlug, req.project_id)}/actions/${this.ref()}/ai-translate`,
      { method: "POST", body: JSON.stringify({ ...req, item: req.item_name }) },
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
      `${this.projectEp(workspaceSlug, projectId)}/actions/${this.ref(stream)}/tm-translate`,
      {
        method: "POST",
        body: JSON.stringify({ item: fileName, target_locale: targetLocale }),
      },
    );
  }

  async getWordCount(
    workspaceSlug: string,
    projectId: string,
    fileName: string,
    stream?: string,
  ): Promise<WordCountResult> {
    return this.fetchJSON(
      `${this.projectEp(workspaceSlug, projectId)}/word-count/${this.ref(stream)}?item=${encodeURIComponent(fileName)}`,
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
      `${this.projectEp(workspaceSlug, projectId)}/actions/${this.ref(stream)}/export`,
      {
        method: "POST",
        body: JSON.stringify({ item: fileName, target_locale: targetLocale }),
      },
    );
  }

  async getTranslationDashboard(
    workspaceSlug: string,
    projectId: string,
    stream?: string,
  ): Promise<TranslationDashboardStats> {
    return this.fetchJSON(
      `${this.projectEp(workspaceSlug, projectId)}/dashboard/${this.ref(stream)}`,
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
      `${this.projectEp(workspaceSlug, projectId)}/blocks/${this.ref(stream)}/${blockId}/tm-matches?item=${encodeURIComponent(itemName)}&target_locale=${targetLocale}`,
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
      `${this.projectEp(workspaceSlug, projectId)}/blocks/${this.ref(stream)}/${blockId}/term-matches?item=${encodeURIComponent(itemName)}&target_locale=${targetLocale}`,
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
      `${this.projectEp(workspaceSlug, projectId)}/blocks/${this.ref(stream)}/${blockId}/history?${params}`,
    );
  }

  // ── Rollback / restore (#778) ────────────────────────────────────────────

  async rollbackBlock(
    workspaceSlug: string,
    projectId: string,
    blockId: string,
    toSeq: number,
    locale: string,
    stream?: string,
  ): Promise<void> {
    await this.fetchJSON(
      `${this.projectEp(workspaceSlug, projectId)}/blocks/${this.ref(stream)}/${blockId}/rollback`,
      { method: "POST", body: JSON.stringify({ locale, to_seq: toSeq }) },
    );
  }

  async revertBatch(
    workspaceSlug: string,
    projectId: string,
    correlationId: string,
    stream?: string,
  ): Promise<{ reverted: number }> {
    return this.fetchJSON(`${this.projectEp(workspaceSlug, projectId)}/revert`, {
      method: "POST",
      body: JSON.stringify({ correlation_id: correlationId, stream }),
    });
  }

  async restoreToPoint(
    workspaceSlug: string,
    projectId: string,
    opts: RestorePointOptions,
  ): Promise<{ restored: number }> {
    return this.fetchJSON(`${this.projectEp(workspaceSlug, projectId)}/restore`, {
      method: "POST",
      body: JSON.stringify(opts),
    });
  }

  async setBlockStatus(
    workspaceSlug: string,
    projectId: string,
    blockId: string,
    status: BlockWorkflowStatus,
    reason?: string,
  ): Promise<void> {
    await this.fetchJSON(
      `${this.projectEp(workspaceSlug, projectId)}/blocks/${this.ref()}/${blockId}/status`,
      { method: "PUT", body: JSON.stringify({ status, reason }) },
    );
  }

  // ── Governance (#778) ─────────────────────────────────────────────────────

  async listGroups(workspaceSlug: string): Promise<Group[]> {
    return this.fetchJSON(`/api/v1/${workspaceSlug}/groups`);
  }
  async createGroup(workspaceSlug: string, name: string, description?: string): Promise<Group> {
    return this.fetchJSON(`/api/v1/${workspaceSlug}/groups`, {
      method: "POST",
      body: JSON.stringify({ name, description }),
    });
  }
  async deleteGroup(workspaceSlug: string, groupId: string): Promise<void> {
    await this.fetchJSON(`/api/v1/${workspaceSlug}/groups/${groupId}`, {
      method: "DELETE",
    });
  }
  async listGroupMembers(workspaceSlug: string, groupId: string): Promise<string[]> {
    return this.fetchJSON(`/api/v1/${workspaceSlug}/groups/${groupId}/members`);
  }
  async addGroupMember(workspaceSlug: string, groupId: string, userId: string): Promise<void> {
    await this.fetchJSON(`/api/v1/${workspaceSlug}/groups/${groupId}/members`, {
      method: "POST",
      body: JSON.stringify({ user_id: userId }),
    });
  }
  async removeGroupMember(workspaceSlug: string, groupId: string, userId: string): Promise<void> {
    await this.fetchJSON(`/api/v1/${workspaceSlug}/groups/${groupId}/members/${userId}`, {
      method: "DELETE",
    });
  }
  async listGroupBindings(workspaceSlug: string, groupId: string): Promise<GroupRoleBinding[]> {
    return this.fetchJSON(`/api/v1/${workspaceSlug}/groups/${groupId}/bindings`);
  }
  async addGroupBinding(
    workspaceSlug: string,
    groupId: string,
    projectId: string,
    roleId: string,
    languages?: string[],
  ): Promise<GroupRoleBinding> {
    return this.fetchJSON(`/api/v1/${workspaceSlug}/groups/${groupId}/bindings`, {
      method: "POST",
      body: JSON.stringify({
        project_id: projectId,
        role_id: roleId,
        languages,
      }),
    });
  }
  async removeGroupBinding(
    workspaceSlug: string,
    groupId: string,
    bindingId: string,
  ): Promise<void> {
    await this.fetchJSON(`/api/v1/${workspaceSlug}/groups/${groupId}/bindings/${bindingId}`, {
      method: "DELETE",
    });
  }
  async listDenyRules(workspaceSlug: string): Promise<DenyRule[]> {
    return this.fetchJSON(`/api/v1/${workspaceSlug}/deny-rules`);
  }
  async createDenyRule(workspaceSlug: string, rule: DenyRuleInput): Promise<DenyRule> {
    return this.fetchJSON(`/api/v1/${workspaceSlug}/deny-rules`, {
      method: "POST",
      body: JSON.stringify(rule),
    });
  }
  async deleteDenyRule(workspaceSlug: string, ruleId: string): Promise<void> {
    await this.fetchJSON(`/api/v1/${workspaceSlug}/deny-rules/${ruleId}`, {
      method: "DELETE",
    });
  }
  async getSoDMode(workspaceSlug: string): Promise<{ mode: SoDMode }> {
    return this.fetchJSON(`/api/v1/${workspaceSlug}/sod`);
  }
  async setSoDMode(workspaceSlug: string, mode: SoDMode): Promise<void> {
    await this.fetchJSON(`/api/v1/${workspaceSlug}/sod`, {
      method: "PUT",
      body: JSON.stringify({ mode }),
    });
  }
  async listRoleOverrides(workspaceSlug: string): Promise<Record<string, string[]>> {
    return this.fetchJSON(`/api/v1/${workspaceSlug}/role-overrides`);
  }
  async setRoleOverride(workspaceSlug: string, role: string, permissions: string[]): Promise<void> {
    await this.fetchJSON(`/api/v1/${workspaceSlug}/role-overrides/${role}`, {
      method: "PUT",
      body: JSON.stringify({ permissions }),
    });
  }
  async demoteBrandRule(workspaceSlug: string, profileId: string, term: string): Promise<void> {
    await this.fetchJSON(`/api/v1/${workspaceSlug}/brand-profiles/${profileId}/demote-rule`, {
      method: "POST",
      body: JSON.stringify({ term }),
    });
  }

  // ── Block Notes ──────────────────────────────────────────────────────────

  async addBlockNote(
    workspaceSlug: string,
    projectId: string,
    blockId: string,
    text: string,
  ): Promise<BlockNote> {
    return this.fetchJSON(
      `${this.projectEp(workspaceSlug, projectId)}/blocks/${this.ref()}/${blockId}/notes`,
      {
        method: "POST",
        body: JSON.stringify({ text }),
      },
    );
  }

  async listBlockNotes(
    workspaceSlug: string,
    projectId: string,
    blockId: string,
  ): Promise<BlockNote[]> {
    return this.fetchJSON(
      `${this.projectEp(workspaceSlug, projectId)}/blocks/${this.ref()}/${blockId}/notes`,
    );
  }

  async deleteBlockNote(workspaceSlug: string, projectId: string, noteId: string): Promise<void> {
    // Note: the route includes block ID in the path, but for deletion we use a
    // placeholder since the server only needs project ID and note ID.
    await this.fetchJSON(
      `${this.projectEp(workspaceSlug, projectId)}/blocks/${this.ref()}/_/notes/${noteId}`,
      { method: "DELETE" },
    );
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
      `${this.projectEp(workspaceSlug, projectId)}/actions/${this.ref(stream)}/qa-check-block`,
      {
        method: "POST",
        body: JSON.stringify({ block_id: blockId, locale }),
      },
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
      `${this.projectEp(workspaceSlug, projectId)}/actions/${this.ref(stream)}/qa-check`,
      {
        method: "POST",
        body: JSON.stringify({ item: fileName, locale }),
      },
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
      `${this.projectEp(workspaceSlug, projectId)}/preview/${this.ref(stream)}?item=${encodeURIComponent(fileName)}&locale=${targetLocale}`,
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
      `${this.projectEp(workspaceSlug, projectId)}/blocks/${this.ref(stream)}/${blockId}/html?locale=${targetLocale}`,
    );
  }

  // ── Translation Memory ───────────────────────────────────────────────────

  private tmEp(ws: string) {
    return `/api/v1/${ws}/translation-memory`;
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
    await this.fetchJSON(`${this.tmEp(workspaceSlug)}/${entryId}`, {
      method: "DELETE",
    });
  }

  // ── Terminology ──────────────────────────────────────────────────────────

  private termsEp(ws: string) {
    return `/api/v1/${ws}/concepts`;
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
    await this.fetchJSON(`${this.termsEp(workspaceSlug)}/${conceptId}`, {
      method: "DELETE",
    });
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
    return this.fetchJSON(`/api/v1/${workspaceSlug}/providers`);
  }

  async saveProviderConfig(
    workspaceSlug: string,
    cfg: ProviderConfigWithKey,
  ): Promise<ProviderConfig> {
    return this.fetchJSON(`/api/v1/${workspaceSlug}/providers`, {
      method: "POST",
      body: JSON.stringify(cfg),
    });
  }

  async deleteProviderConfig(workspaceSlug: string, id: string): Promise<void> {
    await this.fetchJSON(`/api/v1/${workspaceSlug}/providers/${id}`, {
      method: "DELETE",
    });
  }

  async testProviderConfig(workspaceSlug: string, cfg: ProviderConfigWithKey): Promise<void> {
    await this.fetchJSON(`/api/v1/${workspaceSlug}/providers/test`, {
      method: "POST",
      body: JSON.stringify(cfg),
    });
  }

  // ── Automations ────────────────────────────────────────────────────────

  private automationsEp(ws: string, projectId: string) {
    return `${this.projectEp(ws, projectId)}/automations`;
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

  // ── Automation Runs (Bowrain AD-013) ──────────────────────────────────────────

  async listAutomationRuns(
    workspaceSlug: string,
    projectId: string,
    status?: string,
    limit?: number,
  ): Promise<AutomationRun[]> {
    let qs = `limit=${limit ?? 20}`;
    if (status) qs += `&status=${status}`;
    const resp: { runs: AutomationRun[] } = await this.fetchJSON(
      `${this.automationsEp(workspaceSlug, projectId)}/runs?${qs}`,
    );
    return resp.runs;
  }

  async getAutomationRun(
    workspaceSlug: string,
    projectId: string,
    runId: string,
  ): Promise<{ run: AutomationRun; steps: AutomationStep[] }> {
    return this.fetchJSON(`${this.automationsEp(workspaceSlug, projectId)}/runs/${runId}`);
  }

  async listStepLogs(
    workspaceSlug: string,
    projectId: string,
    runId: string,
    stepId: string,
    limit?: number,
  ): Promise<AutomationLogEntry[]> {
    const resp: { logs: AutomationLogEntry[] } = await this.fetchJSON(
      `${this.automationsEp(workspaceSlug, projectId)}/runs/${runId}/steps/${stepId}/logs?limit=${limit ?? 100}`,
    );
    return resp.logs;
  }

  async cancelAutomationRun(
    workspaceSlug: string,
    projectId: string,
    runId: string,
  ): Promise<void> {
    await this.fetchJSON(`${this.automationsEp(workspaceSlug, projectId)}/runs/${runId}/cancel`, {
      method: "POST",
    });
  }

  // ── Flow definitions (Bowrain AD-013) ──────────────────────────────────

  private flowsEp(ws: string, projectId: string) {
    return `${this.projectEp(ws, projectId)}/flows`;
  }

  async listFlowDefinitions(
    workspaceSlug: string,
    projectId: string,
  ): Promise<FlowDefinitionInfo[]> {
    return this.fetchJSON(this.flowsEp(workspaceSlug, projectId));
  }

  async getFlowDefinition(
    workspaceSlug: string,
    projectId: string,
    flowId: string,
  ): Promise<FlowDefinitionInfo> {
    return this.fetchJSON(
      `${this.flowsEp(workspaceSlug, projectId)}/${encodeURIComponent(flowId)}`,
    );
  }

  async createFlowDefinition(
    workspaceSlug: string,
    projectId: string,
    def: FlowDefinitionInfo,
  ): Promise<FlowDefinitionInfo> {
    return this.fetchJSON(this.flowsEp(workspaceSlug, projectId), {
      method: "POST",
      body: JSON.stringify(def),
    });
  }

  async updateFlowDefinition(
    workspaceSlug: string,
    projectId: string,
    flowId: string,
    def: FlowDefinitionInfo,
  ): Promise<FlowDefinitionInfo> {
    return this.fetchJSON(
      `${this.flowsEp(workspaceSlug, projectId)}/${encodeURIComponent(flowId)}`,
      {
        method: "PUT",
        body: JSON.stringify(def),
      },
    );
  }

  async deleteFlowDefinition(
    workspaceSlug: string,
    projectId: string,
    flowId: string,
  ): Promise<void> {
    await this.fetchJSON(
      `${this.flowsEp(workspaceSlug, projectId)}/${encodeURIComponent(flowId)}`,
      {
        method: "DELETE",
      },
    );
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
    return this.fetchJSON(`/api/v1/${workspaceSlug}/notifications?${params}`);
  }

  async markNotificationRead(workspaceSlug: string, id: string): Promise<void> {
    await this.fetchJSON(`/api/v1/${workspaceSlug}/notifications/${id}/read`, {
      method: "PUT",
    });
  }

  async markAllNotificationsRead(workspaceSlug: string): Promise<void> {
    await this.fetchJSON(`/api/v1/${workspaceSlug}/notifications/read-all`, {
      method: "PUT",
    });
  }

  async deleteNotification(workspaceSlug: string, id: string): Promise<void> {
    await this.fetchJSON(`/api/v1/${workspaceSlug}/notifications/${id}`, {
      method: "DELETE",
    });
  }

  // ── Digest Settings ──────────────────────────────────────────────────────

  async getDigestSettings(workspaceSlug: string): Promise<DigestSettingsDTO> {
    return this.fetchJSON<DigestSettingsDTO>(`/api/v1/${workspaceSlug}/digest-settings`);
  }

  async updateDigestSettings(
    workspaceSlug: string,
    settings: DigestSettingsDTO,
  ): Promise<DigestSettingsDTO> {
    return this.fetchJSON<DigestSettingsDTO>(`/api/v1/${workspaceSlug}/digest-settings`, {
      method: "PUT",
      body: JSON.stringify(settings),
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
    return this.fetchJSON(
      `${this.projectEp(workspaceSlug, projectId)}/blocks/${this.ref()}/${blockId}/entities`,
      {
        method: "POST",
        body: JSON.stringify({ item_name: itemName, ...entity }),
      },
    );
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
      `${this.projectEp(workspaceSlug, projectId)}/blocks/${this.ref()}/${blockId}/entities/${encodeURIComponent(entityKey)}`,
      {
        method: "PUT",
        body: JSON.stringify({ item_name: itemName, ...entity }),
      },
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
      `${this.projectEp(workspaceSlug, projectId)}/blocks/${this.ref()}/${blockId}/entities/${encodeURIComponent(entityKey)}?item_name=${encodeURIComponent(itemName)}`,
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
      `${this.projectEp(workspaceSlug, projectId)}/blocks/${this.ref()}/${blockId}/entities/${encodeURIComponent(entityKey)}/promote`,
      { method: "POST", body: JSON.stringify({ item_name: itemName }) },
    );
  }

  // ── Brand Voice ──────────────────────────────────────────────────────────

  private brandEp(ws: string) {
    return `/api/v1/${ws}/brand-profiles`;
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
      `${this.projectEp(workspaceSlug, projectId)}/brand-voice/${this.ref()}/scores`,
    );
  }

  async getBrandTrends(workspaceSlug: string, projectId: string): Promise<ScoreTrend[]> {
    return this.fetchJSON(
      `${this.projectEp(workspaceSlug, projectId)}/brand-voice/${this.ref()}/trends`,
    );
  }

  // ── Correction-learning loop (AD-019) ──────────────────────────────────────

  async listBrandCandidates(
    workspaceSlug: string,
    profileId: string,
    opts?: { minCount?: number; all?: boolean },
  ): Promise<CandidateRule[]> {
    const q = new URLSearchParams();
    if (opts?.minCount) q.set("min_count", String(opts.minCount));
    if (opts?.all) q.set("all", "true");
    const qs = q.toString();
    return this.fetchJSON(
      `${this.brandEp(workspaceSlug)}/${encodeURIComponent(profileId)}/candidates${qs ? `?${qs}` : ""}`,
    );
  }

  async promoteBrandRule(
    workspaceSlug: string,
    profileId: string,
    rule: { term: string; replacement?: string; correction_count?: number },
  ): Promise<{ promoted: boolean }> {
    return this.fetchJSON(
      `${this.brandEp(workspaceSlug)}/${encodeURIComponent(profileId)}/promote-rule`,
      { method: "POST", body: JSON.stringify(rule) },
    );
  }

  async rejectBrandRule(
    workspaceSlug: string,
    profileId: string,
    rule: { term: string; replacement?: string },
  ): Promise<void> {
    await this.fetchJSON(
      `${this.brandEp(workspaceSlug)}/${encodeURIComponent(profileId)}/reject-rule`,
      { method: "POST", body: JSON.stringify(rule) },
    );
  }

  async evaluateBrandRule(
    workspaceSlug: string,
    profileId: string,
    req: {
      term: string;
      replacement?: string;
      project_id: string;
      stream?: string;
    },
  ): Promise<BlastRadius> {
    return this.fetchJSON(
      `${this.brandEp(workspaceSlug)}/${encodeURIComponent(profileId)}/evaluate-rule`,
      { method: "POST", body: JSON.stringify(req) },
    );
  }

  async getBrandDrift(
    workspaceSlug: string,
    projectId: string,
    opts?: { recentDays?: number; minScore?: number; dropPoints?: number },
  ): Promise<DriftResult> {
    const q = new URLSearchParams();
    if (opts?.recentDays) q.set("recent_days", String(opts.recentDays));
    if (opts?.minScore) q.set("min_score", String(opts.minScore));
    if (opts?.dropPoints) q.set("drop_points", String(opts.dropPoints));
    const qs = q.toString();
    return this.fetchJSON(
      `${this.projectEp(workspaceSlug, projectId)}/brand-voice/${this.ref()}/drift${qs ? `?${qs}` : ""}`,
    );
  }

  async listStarterPacks(): Promise<{ name: string; description: string }[]> {
    return this.fetchJSON("/api/v1/brand-voice/starter-packs");
  }

  async createProfileFromStarter(
    workspaceSlug: string,
    pack: string,
    name?: string,
  ): Promise<VoiceProfile> {
    return this.fetchJSON(this.brandEp(workspaceSlug) + "/from-starter", {
      method: "POST",
      body: JSON.stringify({ pack, ...(name ? { name } : {}) }),
    });
  }

  // ── Activities (Bowrain AD-014) ─────────────────────────────────────────────────

  async listActivities(
    workspaceSlug: string,
    query?: {
      project_id?: string;
      stream?: string;
      actor_id?: string;
      type?: string;
      cursor?: string;
      limit?: number;
    },
  ): Promise<{
    activities: ActivityInfo[];
    next_cursor: string;
    new_count?: number;
  }> {
    const params = new URLSearchParams();
    if (query?.project_id) params.set("project_id", query.project_id);
    if (query?.stream) params.set("stream", query.stream);
    if (query?.actor_id) params.set("actor_id", query.actor_id);
    if (query?.type) params.set("type", query.type);
    if (query?.cursor) params.set("cursor", query.cursor);
    if (query?.limit) params.set("limit", String(query.limit));
    const qs = params.toString();
    return this.fetchJSON(`/api/v1/${workspaceSlug}/activities${qs ? `?${qs}` : ""}`);
  }

  async markActivitiesSeen(workspaceSlug: string): Promise<void> {
    await this.fetchJSON(`/api/v1/${workspaceSlug}/activities/seen`, {
      method: "POST",
    });
  }

  // ── Tasks (Bowrain AD-014) ────────────────────────────────────────────────────

  private tasksEp(ws: string) {
    return `/api/v1/${ws}/tasks`;
  }

  async listTasks(
    workspaceSlug: string,
    query?: {
      project_id?: string;
      assignee_id?: string;
      status?: string;
      type?: string;
      priority?: string;
      cursor?: string;
      limit?: number;
    },
  ): Promise<{ tasks: TaskInfo[]; next_cursor: string }> {
    const params = new URLSearchParams();
    if (query?.project_id) params.set("project_id", query.project_id);
    if (query?.assignee_id) params.set("assignee_id", query.assignee_id);
    if (query?.status) params.set("status", query.status);
    if (query?.type) params.set("type", query.type);
    if (query?.priority) params.set("priority", query.priority);
    if (query?.cursor) params.set("cursor", query.cursor);
    if (query?.limit) params.set("limit", String(query.limit));
    const qs = params.toString();
    return this.fetchJSON(`${this.tasksEp(workspaceSlug)}${qs ? `?${qs}` : ""}`);
  }

  async createTask(workspaceSlug: string, task: CreateTaskRequest): Promise<TaskInfo> {
    return this.fetchJSON(this.tasksEp(workspaceSlug), {
      method: "POST",
      body: JSON.stringify(task),
    });
  }

  async getTask(workspaceSlug: string, taskId: string): Promise<TaskInfo> {
    return this.fetchJSON(`${this.tasksEp(workspaceSlug)}/${encodeURIComponent(taskId)}`);
  }

  async updateTask(
    workspaceSlug: string,
    taskId: string,
    updates: Partial<CreateTaskRequest>,
  ): Promise<TaskInfo> {
    return this.fetchJSON(`${this.tasksEp(workspaceSlug)}/${encodeURIComponent(taskId)}`, {
      method: "PATCH",
      body: JSON.stringify(updates),
    });
  }

  async deleteTask(workspaceSlug: string, taskId: string): Promise<void> {
    await this.fetchJSON(`${this.tasksEp(workspaceSlug)}/${encodeURIComponent(taskId)}`, {
      method: "DELETE",
    });
  }

  async assignTask(workspaceSlug: string, taskId: string, assigneeId: string): Promise<void> {
    await this.fetchJSON(`${this.tasksEp(workspaceSlug)}/${encodeURIComponent(taskId)}/assign`, {
      method: "POST",
      body: JSON.stringify({ assignee_id: assigneeId }),
    });
  }

  async completeTask(workspaceSlug: string, taskId: string): Promise<void> {
    await this.fetchJSON(`${this.tasksEp(workspaceSlug)}/${encodeURIComponent(taskId)}/complete`, {
      method: "POST",
    });
  }

  async cancelTask(workspaceSlug: string, taskId: string): Promise<void> {
    await this.fetchJSON(`${this.tasksEp(workspaceSlug)}/${encodeURIComponent(taskId)}/cancel`, {
      method: "POST",
    });
  }

  async listMyTasks(
    workspaceSlug: string,
    query?: { status?: string; cursor?: string; limit?: number },
  ): Promise<{ tasks: TaskInfo[]; next_cursor: string }> {
    const params = new URLSearchParams();
    if (query?.status) params.set("status", query.status);
    if (query?.cursor) params.set("cursor", query.cursor);
    if (query?.limit) params.set("limit", String(query.limit));
    const qs = params.toString();
    return this.fetchJSON(`/api/v1/${workspaceSlug}/my/tasks${qs ? `?${qs}` : ""}`);
  }

  // ── Notification Preferences (Bowrain AD-014) ─────────────────────────────────

  async getNotificationPreferences(
    workspaceSlug: string,
  ): Promise<{ preferences: NotificationPreference[] }> {
    return this.fetchJSON(`/api/v1/${workspaceSlug}/notification-preferences`);
  }

  async updateNotificationPreferences(
    workspaceSlug: string,
    preferences: NotificationPreference[],
  ): Promise<void> {
    await this.fetchJSON(`/api/v1/${workspaceSlug}/notification-preferences`, {
      method: "PUT",
      body: JSON.stringify({ preferences }),
    });
  }

  // ── Audit Log ───────────────────────────────────────────────────────────

  async listWorkspaceAuditLog(workspaceSlug: string, query?: AuditQuery): Promise<AuditEntry[]> {
    const params = new URLSearchParams();
    if (query?.project) params.set("project", query.project);
    if (query?.type) params.set("type", query.type);
    if (query?.actor) params.set("actor", query.actor);
    if (query?.resource_type) params.set("resource_type", query.resource_type);
    if (query?.effect) params.set("effect", query.effect);
    if (query?.search) params.set("search", query.search);
    if (query?.limit) params.set("limit", String(query.limit));
    if (query?.offset) params.set("offset", String(query.offset));
    const qs = params.toString();
    return this.fetchJSON(`/api/v1/${workspaceSlug}/audit-log${qs ? `?${qs}` : ""}`);
  }

  async verifyWorkspaceAuditChain(workspaceSlug: string): Promise<AuditChainVerification> {
    return this.fetchJSON(`/api/v1/${workspaceSlug}/audit-log/verify`);
  }

  // ── @bravo Agent (Bowrain AD-016) ────────────────────────────────────────────────

  private bravoEp(ws: string) {
    return `/api/v1/${ws}/bravo`;
  }

  async bravoCreateConversation(
    workspaceSlug: string,
    projectId?: string,
    title?: string,
  ): Promise<BravoConversation> {
    return this.fetchJSON(`${this.bravoEp(workspaceSlug)}/conversations`, {
      method: "POST",
      body: JSON.stringify({ project_id: projectId, title }),
    });
  }

  async bravoListConversations(
    workspaceSlug: string,
    limit?: number,
    offset?: number,
  ): Promise<{ conversations: BravoConversation[]; total: number }> {
    const params = new URLSearchParams();
    if (limit) params.set("limit", String(limit));
    if (offset) params.set("offset", String(offset));
    const qs = params.toString();
    return this.fetchJSON(`${this.bravoEp(workspaceSlug)}/conversations${qs ? `?${qs}` : ""}`);
  }

  async bravoGetConversation(
    workspaceSlug: string,
    conversationId: string,
  ): Promise<{ conversation: BravoConversation; messages: BravoMessage[] }> {
    return this.fetchJSON(
      `${this.bravoEp(workspaceSlug)}/conversations/${encodeURIComponent(conversationId)}`,
    );
  }

  async bravoDeleteConversation(workspaceSlug: string, conversationId: string): Promise<void> {
    await this.fetchJSON(
      `${this.bravoEp(workspaceSlug)}/conversations/${encodeURIComponent(conversationId)}`,
      { method: "DELETE" },
    );
  }

  async bravoSendMessage(
    workspaceSlug: string,
    conversationId: string,
    content: string,
  ): Promise<{ user_message: BravoMessage; assistant_message: BravoMessage }> {
    return this.fetchJSON(
      `${this.bravoEp(workspaceSlug)}/conversations/${encodeURIComponent(conversationId)}/messages`,
      {
        method: "POST",
        body: JSON.stringify({ content }),
      },
    );
  }

  async bravoListMessages(
    workspaceSlug: string,
    conversationId: string,
    limit?: number,
    offset?: number,
  ): Promise<{ messages: BravoMessage[] }> {
    const params = new URLSearchParams();
    if (limit) params.set("limit", String(limit));
    if (offset) params.set("offset", String(offset));
    const qs = params.toString();
    return this.fetchJSON(
      `${this.bravoEp(workspaceSlug)}/conversations/${encodeURIComponent(conversationId)}/messages${qs ? `?${qs}` : ""}`,
    );
  }

  async bravoApproveToolCall(
    workspaceSlug: string,
    conversationId: string,
    toolCallId: string,
  ): Promise<void> {
    await this.fetchJSON(
      `${this.bravoEp(workspaceSlug)}/conversations/${encodeURIComponent(conversationId)}/tool-calls/${encodeURIComponent(toolCallId)}/approve`,
      { method: "POST" },
    );
  }

  async bravoDenyToolCall(
    workspaceSlug: string,
    conversationId: string,
    toolCallId: string,
  ): Promise<void> {
    await this.fetchJSON(
      `${this.bravoEp(workspaceSlug)}/conversations/${encodeURIComponent(conversationId)}/tool-calls/${encodeURIComponent(toolCallId)}/deny`,
      { method: "POST" },
    );
  }

  async bravoCancelConversation(workspaceSlug: string, conversationId: string): Promise<void> {
    await this.fetchJSON(
      `${this.bravoEp(workspaceSlug)}/conversations/${encodeURIComponent(conversationId)}/cancel`,
      { method: "POST" },
    );
  }

  async bravoGetConfig(workspaceSlug: string): Promise<BravoConfig> {
    return this.fetchJSON(`${this.bravoEp(workspaceSlug)}/config`);
  }

  async bravoUpdateConfig(
    workspaceSlug: string,
    config: Partial<BravoConfig>,
  ): Promise<BravoConfig> {
    return this.fetchJSON(`${this.bravoEp(workspaceSlug)}/config`, {
      method: "PUT",
      body: JSON.stringify(config),
    });
  }

  async bravoListTools(workspaceSlug: string): Promise<{ tools: BravoToolInfo[] }> {
    return this.fetchJSON(`${this.bravoEp(workspaceSlug)}/tools`);
  }

  async bravoGetUsage(
    workspaceSlug: string,
    from?: string,
    to?: string,
  ): Promise<BravoUsageSummary> {
    const params = new URLSearchParams();
    if (from) params.set("from", from);
    if (to) params.set("to", to);
    const qs = params.toString();
    return this.fetchJSON(`${this.bravoEp(workspaceSlug)}/usage${qs ? `?${qs}` : ""}`);
  }

  async bravoUpdateMode(
    workspaceSlug: string,
    conversationId: string,
    mode: string,
  ): Promise<{ mode: string; permissions: string[] }> {
    return this.fetchJSON(
      `${this.bravoEp(workspaceSlug)}/conversations/${encodeURIComponent(conversationId)}/mode`,
      {
        method: "PATCH",
        body: JSON.stringify({ mode }),
      },
    );
  }

  bravoSendMessageSSE(
    workspaceSlug: string,
    conversationId: string,
    content: string,
    handler: BravoSSEHandler,
    mode?: string,
    context?: { projectId?: string; stream?: string; itemId?: string },
  ): AbortController {
    const controller = new AbortController();
    const url = `${this.baseUrl}${this.bravoEp(workspaceSlug)}/conversations/${encodeURIComponent(conversationId)}/messages`;

    const run = async () => {
      try {
        const resp = await fetch(url, {
          method: "POST",
          headers: {
            ...this.headers(),
            Accept: "text/event-stream",
          },
          credentials: "same-origin",
          body: JSON.stringify({ content, mode, context }),
          signal: controller.signal,
        });

        if (!resp.ok || !resp.body) {
          handler.onError?.({ error: `HTTP ${resp.status}` });
          return;
        }

        const reader = resp.body.getReader();
        const decoder = new TextDecoder();
        let buffer = "";
        let currentEvent = "";
        let receivedEnd = false;

        for (;;) {
          const { done, value } = await reader.read();
          if (done) break;

          buffer += decoder.decode(value, { stream: true });
          const lines = buffer.split("\n");
          buffer = lines.pop() ?? "";

          for (const line of lines) {
            if (line.startsWith("event: ")) {
              currentEvent = line.slice(7).trim();
            } else if (line.startsWith("data: ") && currentEvent) {
              const json = line.slice(6);
              try {
                const data = JSON.parse(json);
                switch (currentEvent) {
                  case "message_start":
                    handler.onMessageStart?.(data as BravoSSEMessageStart);
                    break;
                  case "content_delta":
                    handler.onContentDelta?.(data as BravoSSEContentDelta);
                    break;
                  case "tool_call_start":
                    handler.onToolCallStart?.(data as BravoSSEToolCallStart);
                    break;
                  case "tool_call_end":
                    handler.onToolCallEnd?.(data as BravoSSEToolCallEnd);
                    break;
                  case "needs_approval":
                    handler.onNeedsApproval?.(data as BravoSSENeedsApproval);
                    break;
                  case "message_end":
                    receivedEnd = true;
                    handler.onMessageEnd?.(data as BravoSSEMessageEnd);
                    break;
                  case "step_up":
                    handler.onStepUp?.(data as BravoSSEStepUp);
                    break;
                  case "error":
                    receivedEnd = true;
                    handler.onError?.(data as BravoSSEError);
                    break;
                }
              } catch {
                // Skip malformed JSON
              }
              currentEvent = "";
            } else if (line === "") {
              currentEvent = "";
            }
          }
        }

        // Stream closed without a terminal event — clean up.
        if (!receivedEnd) {
          handler.onError?.({ error: "stream ended unexpectedly" });
        }
      } catch (err) {
        if ((err as Error).name !== "AbortError") {
          handler.onError?.({ error: (err as Error).message });
        }
      }
    };

    void run();
    return controller;
  }

  // ── Billing (Bowrain AD-018) ────────────────────────────────────────────────────

  private billingEp(ws: string) {
    return `/api/v1/${ws}/billing`;
  }

  async billingGetOverview(workspaceSlug: string): Promise<BillingOverview> {
    return this.fetchJSON(this.billingEp(workspaceSlug));
  }

  async billingGetUsage(workspaceSlug: string): Promise<BillingUsageBreakdown> {
    return this.fetchJSON(`${this.billingEp(workspaceSlug)}/usage`);
  }

  async billingGetModelUsage(
    workspaceSlug: string,
    from?: string,
    to?: string,
  ): Promise<ModelUsageResponse> {
    const params = new URLSearchParams();
    if (from) params.set("from", from);
    if (to) params.set("to", to);
    const qs = params.toString();
    return this.fetchJSON(`${this.billingEp(workspaceSlug)}/model-usage${qs ? `?${qs}` : ""}`);
  }

  async billingCreateCheckout(
    workspaceSlug: string,
    priceId: string,
    successUrl: string,
    cancelUrl: string,
  ): Promise<{ url: string }> {
    return this.fetchJSON(`${this.billingEp(workspaceSlug)}/checkout`, {
      method: "POST",
      body: JSON.stringify({
        price_id: priceId,
        success_url: successUrl,
        cancel_url: cancelUrl,
      }),
    });
  }

  async billingCreatePortal(workspaceSlug: string, returnUrl: string): Promise<{ url: string }> {
    return this.fetchJSON(`${this.billingEp(workspaceSlug)}/portal`, {
      method: "POST",
      body: JSON.stringify({ return_url: returnUrl }),
    });
  }

  async billingGetLedger(
    workspaceSlug: string,
    from?: string,
    to?: string,
  ): Promise<CreditLedgerEntry[]> {
    const params = new URLSearchParams();
    if (from) params.set("from", from);
    if (to) params.set("to", to);
    const qs = params.toString();
    return this.fetchJSON(`${this.billingEp(workspaceSlug)}/ledger${qs ? `?${qs}` : ""}`);
  }

  // ── Brand knowledge graph — Concepts (AD-021) ─────────────────────────────
  // The concept routes share the /concepts base (termsEp) with the Terminology
  // block above; these add the graph/governance surface.

  private conceptEp(ws: string, conceptId: string) {
    return `${this.termsEp(ws)}/${encodeURIComponent(conceptId)}`;
  }

  private marketsEp(ws: string) {
    return `/api/v1/${ws}/markets`;
  }

  private changesetsEp(ws: string) {
    return `/api/v1/${ws}/changesets`;
  }

  private changesetEp(ws: string, id: string) {
    return `${this.changesetsEp(ws)}/${encodeURIComponent(id)}`;
  }

  async listConcepts(
    workspaceSlug: string,
    params?: ListConceptsParams,
  ): Promise<TermSearchResult> {
    const q = new URLSearchParams();
    if (params?.q) q.set("q", params.q);
    if (params?.status) q.set("status", params.status);
    if (params?.domain) q.set("domain", params.domain);
    if (params?.market) q.set("market", params.market);
    if (params?.locale) q.set("locale", params.locale);
    if (params?.source) q.set("source", params.source);
    if (params?.stream) q.set("stream", params.stream);
    if (params?.project_id) q.set("project_id", params.project_id);
    if (params?.offset !== undefined) q.set("offset", String(params.offset));
    if (params?.limit !== undefined) q.set("limit", String(params.limit));
    const qs = q.toString();
    return this.fetchJSON(`${this.termsEp(workspaceSlug)}${qs ? `?${qs}` : ""}`);
  }

  async getConcept(workspaceSlug: string, conceptId: string): Promise<ConceptInfo> {
    return this.fetchJSON(this.conceptEp(workspaceSlug, conceptId));
  }

  async createConcept(workspaceSlug: string, req: AddConceptRequest): Promise<ConceptInfo> {
    return this.fetchJSON(this.termsEp(workspaceSlug), {
      method: "POST",
      body: JSON.stringify(req),
    });
  }

  async getConceptStory(workspaceSlug: string, conceptId: string): Promise<ConceptStory> {
    return this.fetchJSON(`${this.conceptEp(workspaceSlug, conceptId)}/story`);
  }

  async listConceptRelations(
    workspaceSlug: string,
    conceptId: string,
    scope?: RelationScope,
  ): Promise<ConceptRelation[]> {
    const q = new URLSearchParams();
    if (scope?.as_of) q.set("as_of", scope.as_of);
    if (scope?.market) q.set("market", scope.market);
    const qs = q.toString();
    return this.fetchJSON(
      `${this.conceptEp(workspaceSlug, conceptId)}/relations${qs ? `?${qs}` : ""}`,
    );
  }

  async addConceptRelation(
    workspaceSlug: string,
    conceptId: string,
    req: AddConceptRelationRequest,
  ): Promise<ConceptRelation> {
    return this.fetchJSON(`${this.conceptEp(workspaceSlug, conceptId)}/relations`, {
      method: "POST",
      body: JSON.stringify(req),
    });
  }

  async deleteConceptRelation(
    workspaceSlug: string,
    conceptId: string,
    relationId: string,
  ): Promise<void> {
    await this.fetchJSON(
      `${this.conceptEp(workspaceSlug, conceptId)}/relations/${encodeURIComponent(relationId)}`,
      { method: "DELETE" },
    );
  }

  async getConceptBlastRadius(workspaceSlug: string, conceptId: string): Promise<ConceptUsage> {
    return this.fetchJSON(`${this.conceptEp(workspaceSlug, conceptId)}/blast-radius`);
  }

  async listObservations(workspaceSlug: string, conceptId: string): Promise<Observation[]> {
    return this.fetchJSON(`${this.conceptEp(workspaceSlug, conceptId)}/observations`);
  }

  async addObservation(
    workspaceSlug: string,
    conceptId: string,
    req: AddObservationRequest,
  ): Promise<Observation> {
    return this.fetchJSON(`${this.conceptEp(workspaceSlug, conceptId)}/observations`, {
      method: "POST",
      body: JSON.stringify(req),
    });
  }

  async deleteObservation(
    workspaceSlug: string,
    conceptId: string,
    observationId: string,
  ): Promise<void> {
    await this.fetchJSON(
      `${this.conceptEp(workspaceSlug, conceptId)}/observations/${encodeURIComponent(observationId)}`,
      { method: "DELETE" },
    );
  }

  async listConceptComments(workspaceSlug: string, conceptId: string): Promise<Comment[]> {
    return this.fetchJSON(`${this.conceptEp(workspaceSlug, conceptId)}/comments`);
  }

  async addConceptComment(
    workspaceSlug: string,
    conceptId: string,
    req: AddCommentRequest,
  ): Promise<Comment> {
    return this.fetchJSON(`${this.conceptEp(workspaceSlug, conceptId)}/comments`, {
      method: "POST",
      body: JSON.stringify(req),
    });
  }

  async resolveConceptComment(
    workspaceSlug: string,
    conceptId: string,
    commentId: string,
    resolved?: boolean,
  ): Promise<void> {
    await this.fetchJSON(
      `${this.conceptEp(workspaceSlug, conceptId)}/comments/${encodeURIComponent(commentId)}/resolve`,
      {
        method: "POST",
        body: JSON.stringify(resolved === undefined ? {} : { resolved }),
      },
    );
  }

  async deleteConceptComment(
    workspaceSlug: string,
    conceptId: string,
    commentId: string,
  ): Promise<void> {
    await this.fetchJSON(
      `${this.conceptEp(workspaceSlug, conceptId)}/comments/${encodeURIComponent(commentId)}`,
      { method: "DELETE" },
    );
  }

  // ── Brand knowledge graph — Markets (AD-021) ──────────────────────────────

  async listMarkets(workspaceSlug: string): Promise<Market[]> {
    return this.fetchJSON(this.marketsEp(workspaceSlug));
  }

  async createMarket(workspaceSlug: string, req: MarketRequest): Promise<Market> {
    return this.fetchJSON(this.marketsEp(workspaceSlug), {
      method: "POST",
      body: JSON.stringify(req),
    });
  }

  async updateMarket(workspaceSlug: string, marketId: string, req: MarketRequest): Promise<Market> {
    return this.fetchJSON(`${this.marketsEp(workspaceSlug)}/${encodeURIComponent(marketId)}`, {
      method: "PUT",
      body: JSON.stringify(req),
    });
  }

  async deleteMarket(workspaceSlug: string, marketId: string): Promise<void> {
    await this.fetchJSON(`${this.marketsEp(workspaceSlug)}/${encodeURIComponent(marketId)}`, {
      method: "DELETE",
    });
  }

  // ── Brand knowledge graph — Change-sets / experiments (AD-021) ────────────

  async listChangesets(workspaceSlug: string, status?: ChangeSetStatus): Promise<ChangeSet[]> {
    const qs = status ? `?status=${encodeURIComponent(status)}` : "";
    return this.fetchJSON(`${this.changesetsEp(workspaceSlug)}${qs}`);
  }

  async getChangeset(workspaceSlug: string, changesetId: string): Promise<ChangeSetDetail> {
    return this.fetchJSON(this.changesetEp(workspaceSlug, changesetId));
  }

  async createChangeset(workspaceSlug: string, req: CreateChangeSetRequest): Promise<ChangeSet> {
    return this.fetchJSON(this.changesetsEp(workspaceSlug), {
      method: "POST",
      body: JSON.stringify(req),
    });
  }

  async patchChangeset(
    workspaceSlug: string,
    changesetId: string,
    req: UpdateChangeSetRequest,
  ): Promise<ChangeSet> {
    return this.fetchJSON(this.changesetEp(workspaceSlug, changesetId), {
      method: "PATCH",
      body: JSON.stringify(req),
    });
  }

  async appendChangesetOp(
    workspaceSlug: string,
    changesetId: string,
    req: AddChangeSetOpRequest,
  ): Promise<ChangeSetOp> {
    return this.fetchJSON(`${this.changesetEp(workspaceSlug, changesetId)}/ops`, {
      method: "POST",
      body: JSON.stringify(req),
    });
  }

  async removeChangesetOp(workspaceSlug: string, changesetId: string, seq: number): Promise<void> {
    await this.fetchJSON(`${this.changesetEp(workspaceSlug, changesetId)}/ops/${seq}`, {
      method: "DELETE",
    });
  }

  async submitChangeset(workspaceSlug: string, changesetId: string): Promise<ChangeSet> {
    return this.fetchJSON(`${this.changesetEp(workspaceSlug, changesetId)}/submit`, {
      method: "POST",
    });
  }

  async approveChangeset(
    workspaceSlug: string,
    changesetId: string,
    req?: ReviewRequest,
  ): Promise<ChangeSet> {
    return this.fetchJSON(`${this.changesetEp(workspaceSlug, changesetId)}/approve`, {
      method: "POST",
      body: JSON.stringify(req ?? {}),
    });
  }

  async rejectChangeset(
    workspaceSlug: string,
    changesetId: string,
    req?: ReviewRequest,
  ): Promise<ChangeSet> {
    return this.fetchJSON(`${this.changesetEp(workspaceSlug, changesetId)}/reject`, {
      method: "POST",
      body: JSON.stringify(req ?? {}),
    });
  }

  async mergeChangeset(workspaceSlug: string, changesetId: string): Promise<MergeResult> {
    return this.fetchJSON(`${this.changesetEp(workspaceSlug, changesetId)}/merge`, {
      method: "POST",
    });
  }

  async abandonChangeset(workspaceSlug: string, changesetId: string): Promise<ChangeSet> {
    return this.fetchJSON(`${this.changesetEp(workspaceSlug, changesetId)}/abandon`, {
      method: "POST",
    });
  }

  async getChangesetBlastRadius(
    workspaceSlug: string,
    changesetId: string,
  ): Promise<ChangeSetImpact> {
    return this.fetchJSON(`${this.changesetEp(workspaceSlug, changesetId)}/blast-radius`);
  }

  async addPilot(
    workspaceSlug: string,
    changesetId: string,
    req: StartPilotRequest,
  ): Promise<Pilot> {
    return this.fetchJSON(`${this.changesetEp(workspaceSlug, changesetId)}/pilots`, {
      method: "POST",
      body: JSON.stringify(req),
    });
  }

  async removePilot(
    workspaceSlug: string,
    changesetId: string,
    projectId: string,
    stream: string,
  ): Promise<void> {
    await this.fetchJSON(
      `${this.changesetEp(workspaceSlug, changesetId)}/pilots/${encodeURIComponent(projectId)}/${encodeURIComponent(stream)}`,
      { method: "DELETE" },
    );
  }

  // ── Utility ──────────────────────────────────────────────────────────────

  async getKnownLocales(): Promise<LocaleInfo[]> {
    const info = await this.fetchJSON<{ locales: LocaleInfo[] }>("/api/v1/info");
    return info.locales;
  }

  async listFormats(): Promise<FormatInfo[]> {
    const info = await this.fetchJSON<{ formats: FormatInfo[] }>("/api/v1/info");
    return info.formats;
  }

  async listTools(): Promise<ToolInfo[]> {
    const info = await this.fetchJSON<{ tools: ToolInfo[] }>("/api/v1/info");
    return info.tools;
  }
}
