import { codedToRuns } from "@neokapi/ui-primitives";
import { normalizeServerBlocks, type ServerBlockInfo } from "../components/editor/blockRuns";
import { apiErrorFromResponse } from "../errors/ApiError";
import type { ApiAdapter } from "./adapter";
import type {
  User,
  Workspace,
  Membership,
  ProjectInfo,
  CallerPermissions,
  ProjectReadOptions,
  UploadFilesResult,
  ConfigResponse,
  PublicPlatformConfig,
  BlockInfo,
  UpdateBlockRequest,
  UpdateBlockTargetCodedRequest,
  AITranslateFileRequest,
  TranslationStats,
  WordCountResult,
  ProviderConfig,
  ProviderConfigWithKey,
  MemoryEntryInfo,
  MemorySearchResult,
  MemoryUpdateRequest,
  MemoryMatchInfo,
  ConceptInfo,
  TermSearchResult,
  AddConceptRequest,
  UpdateConceptRequest,
  BlockTermMatch,
  ReviewContext,
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
  ConnectorInfo,
  ConnectorSyncStatus,
  ConnectorContentItem,
  PostHogConnectorConfig,
  PostHogConnectorConfigRequest,
  PostHogDemandResponse,
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
  TranslationDashboardItemOpts,
  ActivityPage,
  ConvergenceRun,
  ConvergenceEstimate,
  LoopRollup,
  ConvergenceRunScope,
  TaskInfo,
  TaskPage,
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
  BillingPlan,
  BillingPlansResponse,
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
  AccountSecurity,
  PasskeyListResponse,
  PasskeyRegisterStartResponse,
  PasskeyRegisterFinishRequest,
  SlugReservation,
  ReviewDemotion,
  ApprovePassingRequest,
  ApprovePassingResult,
  SourceProposal,
  CreateSourceProposalRequest,
  DecideSourceProposalResult,
  ContextScanRequest,
  ContextScanUploadResult,
  ContextScanJob,
  ContextScanCheckResult,
  GitHubSetupState,
  ClaimInstallationResult,
  InstallationRepo,
  BindInstallationRepoRequest,
  BindInstallationRepoResult,
  RepoDetection,
  RepoDetectOptions,
  ModelRecommendationsResponse,
  PendingReviewOptions,
  TermCompliance,
  PendingReviewPage,
  BlockQueryOptions,
  BlockCounts,
  ItemInfo,
  BulkReviewBlocksRequest,
  BulkReviewBlocksResult,
  BulkApplyMemoryRequest,
  BulkApplyMemoryResult,
  BulkDeleteResult,
  ConnectorStatusBatch,
  AutomationHistoryPage,
  TaskQuery,
  TaskCounts,
  CreditLedgerQuery,
  CreditLedgerPage,
  ContextScanApproveRequest,
  ContextScanApproveResult,
  ApproveAxisRequest,
  PendingRecipeChange,
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
  VoiceRollup,
  VoiceRollupOptions,
  VoiceCorrectionRequest,
  VoiceCorrectionResult,
} from "../voice/types";
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
  TrialReport,
  ChangeSetCounts,
  ConceptStatusCounts,
  LocaleCoverageReport,
  MergeResult,
  Pilot,
  StartPilotRequest,
} from "../types/brand-graph";
import type { ContextProfilesResponse } from "../types/context-profiles";
import type {
  ChannelAliasJudgement,
  ChannelAliasProposal,
  ChannelAliasProposalsResponse,
  ChannelProposalStatus,
} from "../types/channel-proposals";
import type { ConceptProjects, ConceptProjectsQuery } from "../types/context-graph";

/**
 * Encode a value for use as a single URL path segment, preserving the colon.
 * Concept IDs may be `term:<hash>` — `:` is path-legal (RFC 3986 pchar) and the
 * server stores/matches IDs verbatim, so encodeURIComponent's `%3A` would 404.
 * Everything else (notably `/`) is still encoded.
 */
function encodeConceptSegment(id: string): string {
  return encodeURIComponent(id).replace(/%3A/gi, ":");
}

/**
 * The billing payloads exactly as the server sends them (Go JSON, snake_case).
 * They exist so the mapping into the UI's camelCase billing types is typed rather
 * than a cast: the previous code declared the server returned the UI shape, which
 * it never did, and TypeScript had no way to notice.
 */
interface BillingSubscriptionDTO {
  plan: BillingPlan;
  status: "active" | "past_due" | "canceled" | "trialing";
  seat_count: number;
  stripe_customer_id?: string;
  current_period_start?: string;
  current_period_end?: string;
  cancel_at?: string;
  trial_ends_at?: string;
}

interface CreditLedgerEntryDTO {
  id: number | string;
  amount: number;
  balance_after: number;
  operation: string;
  reference_id?: string;
  created_at: string;
}

/**
 * Connector sync status exactly as the Go server marshals it: `connector.SyncStatus`
 * has no JSON tags, so every field is PascalCase. Mapped into the UI's camelCase
 * ConnectorSyncStatus so consumers never touch the wire casing.
 */
/**
 * The wire shape of `core/venue/connector.SyncStatus`, which the per-connector and
 * batch status routes both answer with verbatim. The adapter normalises it into
 * the camelCase {@link ConnectorSyncStatus} the surfaces read.
 */
interface ConnectorSyncStatusDTO {
  connector_id: string;
  last_sync: string;
  item_count: number;
  file_count: number;
  word_count: number;
  pending_pull: number;
  pending_push: number;
  errors: string[] | null;
}

/** One DTO → the camelCase status, shared by the single and batch reads. */
function connectorSyncStatus(dto: ConnectorSyncStatusDTO): ConnectorSyncStatus {
  return {
    connectorId: dto.connector_id,
    lastSync: dto.last_sync,
    itemCount: dto.item_count,
    fileCount: dto.file_count,
    wordCount: dto.word_count,
    pendingPull: dto.pending_pull,
    pendingPush: dto.pending_push,
    errors: dto.errors ?? [],
  };
}

/**
 * RestApiAdapter talks to the neokapi REST server.
 * Used by the web apps (apps/web and apps/kapi-web).
 *
 * Supports two auth modes:
 * - Bearer token mode (desktop/CLI): token set via setToken(), sent as Authorization header.
 * - Cookie mode (web): no token set, browser sends HttpOnly cookies automatically.
 */
/**
 * A fetch-compatible transport. Defaults to the global `fetch`; the desktop
 * passes one backed by a Wails ProxyRequest binding so all server calls the
 * desktop doesn't implement with a dedicated binding go Go-side (keychain auth,
 * no webview-origin/CORS concerns) through this same adapter.
 */
export type ApiTransport = (input: string, init?: RequestInit) => Promise<Response>;

/**
 * Build the typed ApiError thrown on a non-OK response: the server's error
 * envelope parsed into `{ code, message, referenceId, status, details }`,
 * tolerating legacy shapes (no message/reference, plain-text bodies).
 *
 * The reference falls back to the `X-Request-ID` response header so it is
 * available even when the body is not our JSON envelope (gateway 502s, empty
 * bodies, text/blob endpoints).
 */
function httpError(resp: Response, body: string): Error {
  return apiErrorFromResponse(resp.status, body, resp.headers.get("X-Request-ID"));
}

export class RestApiAdapter implements ApiAdapter {
  private baseUrl: string;
  private token: string | null;
  private refreshToken: string | null;
  protected fetchImpl: ApiTransport;

  /** Called when tokens are refreshed so the consumer can persist them. */
  onTokenRefresh?: (token: string, refreshToken: string) => void;

  /** Called when the session is invalid and the user must re-authenticate. */
  onSessionExpired?: () => void;

  private refreshPromise: Promise<boolean> | null = null;

  constructor(baseUrl: string = "", token: string | null = null, transport?: ApiTransport) {
    this.baseUrl = baseUrl;
    this.token = token;
    this.refreshToken = null;
    this.fetchImpl = transport ?? ((input, init) => fetch(input, init));
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
    } else {
      // Cookie (BFF) mode: send the CSRF header the server requires on
      // cookie-authenticated, state-changing requests. Any non-empty value
      // works — the guard is header *presence*, which a cross-origin page
      // cannot forge without a CORS preflight the server rejects. Omitted in
      // Bearer mode (native apps), where token auth is CSRF-exempt server-side
      // and a cross-origin custom header would force an unwanted preflight.
      h["X-Bowrain-Csrf"] = "1";
    }
    return h;
  }

  /**
   * Headers for a multipart/form-data request: the same auth and CSRF headers
   * as every other call, minus Content-Type — the browser must set that itself
   * so the multipart boundary is generated.
   *
   * The upload methods used to hand-roll `this.token ? { Authorization } : {}`,
   * which in cookie (BFF) mode produced no headers at all. When #1239 made the
   * CSRF header mandatory on cookie-authenticated state-changing requests,
   * every upload from the web app started coming back 403 "missing CSRF
   * header" — file upload, collection upload, and brand-scan sources alike.
   * Deriving from headers() keeps the auth contract in one place.
   */
  private uploadHeaders(): Record<string, string> {
    const h = this.headers();
    delete h["Content-Type"];
    return h;
  }

  /**
   * Attempt a session refresh through the same deduplicated path the 401
   * handling in fetchJSON uses, without carrying a request. Exposed for
   * consumers that cannot observe HTTP status codes — the workspace SSE
   * EventSource reconnect path — so they can restore the cookie/token session
   * before reconnecting instead of 401-looping forever.
   */
  async refreshSession(): Promise<boolean> {
    if (!this.refreshPromise) {
      this.refreshPromise = this.tryRefresh().finally(() => {
        this.refreshPromise = null;
      });
    }
    return this.refreshPromise;
  }

  /**
   * Attempt to refresh the access token using the stored refresh token or cookie.
   *
   * The headers come from `headers()` for the same reason the uploads' do. In
   * cookie (BFF) mode there is no in-memory refresh token, so the body is empty
   * and the server reads the HttpOnly refresh cookie — which makes this an
   * ambient-credential, state-changing request, and those must carry the CSRF
   * header. A hand-rolled `{ "Content-Type": … }` omitted it, so every browser
   * refresh came back 403 "missing CSRF header", `tryRefresh` reported failure,
   * and `onSessionExpired` bounced the user to the identity provider as soon as
   * the 15-minute access cookie lapsed (#1809). Deriving from `headers()` keeps
   * the auth contract in one place.
   */
  private async tryRefresh(): Promise<boolean> {
    try {
      const resp = await this.fetchImpl(`${this.baseUrl}/api/v1/auth/refresh`, {
        method: "POST",
        headers: this.headers(),
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
    const resp = await this.fetchImpl(`${this.baseUrl}${path}`, {
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
        const retry = await this.fetchImpl(`${this.baseUrl}${path}`, {
          ...init,
          headers: {
            ...this.headers(),
            ...(init?.headers as Record<string, string>),
          },
          credentials: "same-origin",
        });
        if (!retry.ok) {
          const body = await retry.text();
          throw httpError(retry, body);
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
      throw httpError(resp, body);
    }
    if (resp.status === 204) return undefined as T;
    return resp.json();
  }

  private async fetchBlob(path: string, init?: RequestInit): Promise<Blob> {
    const resp = await this.fetchImpl(`${this.baseUrl}${path}`, {
      ...init,
      headers: {
        ...this.headers(),
        ...(init?.headers as Record<string, string>),
      },
      credentials: "same-origin",
    });
    if (!resp.ok) {
      const body = await resp.text();
      throw httpError(resp, body);
    }
    return resp.blob();
  }

  private async fetchText(path: string, init?: RequestInit): Promise<string> {
    const resp = await this.fetchImpl(`${this.baseUrl}${path}`, {
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
        const retry = await this.fetchImpl(`${this.baseUrl}${path}`, {
          ...init,
          headers: {
            ...this.headers(),
            ...(init?.headers as Record<string, string>),
          },
          credentials: "same-origin",
        });
        if (!retry.ok) {
          const body = await retry.text();
          throw httpError(retry, body);
        }
        return retry.text();
      }
      this.onSessionExpired?.();
      return new Promise<string>(() => {});
    }
    if (!resp.ok) {
      const body = await resp.text();
      throw httpError(resp, body);
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

  async getPublicPlatformConfig(): Promise<PublicPlatformConfig> {
    return this.fetchJSON("/api/v1/config");
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
      const resp = await this.fetchImpl(`${this.baseUrl}/api/v1/auth/me`, {
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

  // ── Account security (passkeys) ─────────────────────────────────────────
  // The server relays only the WebAuthn ceremony (no identity-provider token
  // reaches the browser): the browser runs navigator.credentials.* on a
  // challenge and posts the result back. Managing credentials needs a
  // short-lived, self-service-scoped token the server obtains via an explicit
  // step-up; a 409 with error "elevation_required" means the caller should send
  // the user through /api/v1/account/security/elevate (see beginElevation).

  async getAccountSecurity(): Promise<AccountSecurity> {
    return this.fetchJSON("/api/v1/account/security");
  }

  async listPasskeys(): Promise<PasskeyListResponse> {
    return this.fetchJSON("/api/v1/account/passkeys");
  }

  async passkeyRegisterStart(): Promise<PasskeyRegisterStartResponse> {
    return this.fetchJSON("/api/v1/account/passkeys/register/start", {
      method: "POST",
    });
  }

  async passkeyRegisterFinish(req: PasskeyRegisterFinishRequest): Promise<void> {
    await this.fetchJSON("/api/v1/account/passkeys/register/finish", {
      method: "POST",
      body: JSON.stringify(req),
    });
  }

  async deletePasskey(id: string): Promise<void> {
    await this.fetchJSON(`/api/v1/account/passkeys/${encodeURIComponent(id)}`, {
      method: "DELETE",
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
      coordinates?: Record<string, string>;
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
      coordinates?: Record<string, string>;
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
    agentName?: string,
  ): Promise<CreateApiTokenResponse> {
    const body: Record<string, unknown> = { name, expire_days: expireDays };
    if (scopes && scopes.length > 0) {
      body.scopes = scopes;
    }
    if (agentName) {
      body.agent_name = agentName;
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
    data: { description?: string; visibility?: string; properties?: Record<string, string> },
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
    opts?: ProjectReadOptions,
  ): Promise<ProjectInfo> {
    // The default (no query) is the server's full detail shape. `view=summary`
    // drops the embedded item array, which the server would otherwise pay a
    // block read per item to build.
    const query = opts?.view === "summary" ? "?view=summary" : "";
    return this.fetchJSON(this.projectEp(workspaceSlug, projectId) + query);
  }

  async getCallerPermissions(workspaceSlug: string, projectId: string): Promise<CallerPermissions> {
    return this.fetchJSON(this.projectEp(workspaceSlug, projectId) + "/permissions");
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

  // Measured steerability (model recommendation sweeps).
  async getModelRecommendations(
    workspaceSlug: string,
    projectId: string,
  ): Promise<ModelRecommendationsResponse> {
    return this.fetchJSON(`${this.projectEp(workspaceSlug, projectId)}/model-recommendations`);
  }

  async refreshModelRecommendations(
    workspaceSlug: string,
    projectId: string,
  ): Promise<{ enqueued: number; locales: string[] }> {
    return this.fetchJSON(
      `${this.projectEp(workspaceSlug, projectId)}/model-recommendations/refresh`,
      { method: "POST" },
    );
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
  ): Promise<UploadFilesResult> {
    const formData = new FormData();
    for (const file of files) {
      formData.append("files", file);
    }
    const url = `${this.projectEp(workspaceSlug, projectId)}/items/${this.ref(stream)}`;
    const resp = await this.fetchImpl(`${this.baseUrl}${url}`, {
      method: "POST",
      headers: this.uploadHeaders(),
      credentials: "same-origin",
      body: formData,
    });
    if (!resp.ok) {
      const body = await resp.text();
      throw httpError(resp, body);
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

  // ── Integration connectors (Bowrain AD-011) ─────────────────────────────
  // /:ws/connectors — workspace-scoped CMS/design/marketing integrations.
  // A connector is added once per workspace; fetch/publish carry the project
  // in the body so one connector can serve any project in the workspace.

  async listConnectors(workspaceSlug: string): Promise<ConnectorInfo[]> {
    return (await this.fetchJSON<ConnectorInfo[]>(`/api/v1/${workspaceSlug}/connectors`)) ?? [];
  }

  async addConnector(
    workspaceSlug: string,
    type: string,
    config: Record<string, string>,
  ): Promise<ConnectorInfo> {
    return this.fetchJSON(`/api/v1/${workspaceSlug}/connectors`, {
      method: "POST",
      body: JSON.stringify({ type, config }),
    });
  }

  async githubSetupState(workspaceSlug: string): Promise<GitHubSetupState> {
    return this.fetchJSON(`/api/v1/${workspaceSlug}/github/setup-state`);
  }

  async claimInstallation(
    workspaceSlug: string,
    installationId: string,
    state: string,
  ): Promise<ClaimInstallationResult> {
    return this.fetchJSON(
      `/api/v1/${workspaceSlug}/github/installations/${encodeURIComponent(installationId)}/claim`,
      { method: "POST", body: JSON.stringify({ state }) },
    );
  }

  async listInstallationRepos(
    workspaceSlug: string,
    installationId: string,
  ): Promise<InstallationRepo[]> {
    return (
      (await this.fetchJSON<InstallationRepo[]>(
        `/api/v1/${workspaceSlug}/github/installations/${encodeURIComponent(installationId)}/repositories`,
      )) ?? []
    );
  }

  async detectInstallationRepo(
    workspaceSlug: string,
    installationId: string,
    repository: string,
    opts?: RepoDetectOptions,
  ): Promise<RepoDetection> {
    const slash = repository.indexOf("/");
    const owner = slash >= 0 ? repository.slice(0, slash) : repository;
    const name = slash >= 0 ? repository.slice(slash + 1) : "";
    const params = new URLSearchParams();
    if (opts?.scope) params.set("scope", opts.scope);
    if (opts?.patterns) params.set("patterns", opts.patterns);
    const query = params.size > 0 ? `?${params.toString()}` : "";
    return this.fetchJSON(
      `/api/v1/${workspaceSlug}/github/installations/${encodeURIComponent(installationId)}/repositories/${encodeURIComponent(owner)}/${encodeURIComponent(name)}/detect${query}`,
    );
  }

  async bindInstallationRepo(
    workspaceSlug: string,
    installationId: string,
    req: BindInstallationRepoRequest,
  ): Promise<BindInstallationRepoResult> {
    return this.fetchJSON(
      `/api/v1/${workspaceSlug}/github/installations/${encodeURIComponent(installationId)}/repositories`,
      { method: "POST", body: JSON.stringify(req) },
    );
  }

  async removeConnector(workspaceSlug: string, connectorId: string): Promise<void> {
    await this.fetchJSON(`/api/v1/${workspaceSlug}/connectors/${encodeURIComponent(connectorId)}`, {
      method: "DELETE",
    });
  }

  async getConnectorStatus(
    workspaceSlug: string,
    connectorId: string,
    opts?: { probe?: boolean },
  ): Promise<ConnectorSyncStatus> {
    // Default = the server's cheap stored read; probe=1 opts into the deep
    // live probe (a clone for git/forge) for explicit "Test"/manual paths.
    const dto = await this.fetchJSON<ConnectorSyncStatusDTO>(
      `/api/v1/${workspaceSlug}/connectors/${encodeURIComponent(connectorId)}/status${
        opts?.probe ? "?probe=1" : ""
      }`,
    );
    return connectorSyncStatus(dto);
  }

  async getConnectorStatuses(
    workspaceSlug: string,
    connectorIds: string[],
    opts?: { probe?: boolean },
  ): Promise<ConnectorStatusBatch> {
    if (connectorIds.length === 0) return { statuses: {}, unknown: [] };
    const q = new URLSearchParams({ ids: connectorIds.join(",") });
    if (opts?.probe) q.set("probe", "1");
    const raw = await this.fetchJSON<{
      statuses: Record<string, ConnectorSyncStatusDTO>;
      unknown: string[] | null;
    }>(`/api/v1/${workspaceSlug}/connectors/status?${q.toString()}`);
    const statuses: Record<string, ConnectorSyncStatus> = {};
    for (const [id, dto] of Object.entries(raw.statuses ?? {})) {
      statuses[id] = connectorSyncStatus(dto);
    }
    return { statuses, unknown: raw.unknown ?? [] };
  }

  async fetchConnector(
    workspaceSlug: string,
    connectorId: string,
    projectId: string,
    paths?: string[],
  ): Promise<{ items_fetched: number }> {
    return this.fetchJSON(
      `/api/v1/${workspaceSlug}/connectors/${encodeURIComponent(connectorId)}/fetch`,
      {
        method: "POST",
        body: JSON.stringify({ connector_id: connectorId, project_id: projectId, paths }),
      },
    );
  }

  async publishConnector(
    workspaceSlug: string,
    connectorId: string,
    projectId: string,
    message?: string,
  ): Promise<{ status: string }> {
    return this.fetchJSON(
      `/api/v1/${workspaceSlug}/connectors/${encodeURIComponent(connectorId)}/publish`,
      {
        method: "POST",
        body: JSON.stringify({ connector_id: connectorId, project_id: projectId, message }),
      },
    );
  }

  async listConnectorContent(
    workspaceSlug: string,
    connectorId: string,
    projectId?: string,
  ): Promise<ConnectorContentItem[]> {
    // The server marshals connector.ContentItem verbatim (PascalCase), so the
    // items already match ConnectorContentItem — no field mapping needed.
    const query = projectId ? `?project_id=${encodeURIComponent(projectId)}` : "";
    const res = await this.fetchJSON<{ items?: ConnectorContentItem[] | null }>(
      `/api/v1/${workspaceSlug}/connectors/${encodeURIComponent(connectorId)}/content${query}`,
    );
    return res?.items ?? [];
  }

  // ── PostHog locale-demand connector ─────────────────────────────────────
  // /:ws/:id/connectors/posthog — the personal API key is write-only; the
  // server stores it sealed and config reads return only a masked tail.

  async getPostHogConnector(
    workspaceSlug: string,
    projectId: string,
  ): Promise<PostHogConnectorConfig> {
    return this.fetchJSON(`${this.projectEp(workspaceSlug, projectId)}/connectors/posthog`);
  }

  async savePostHogConnector(
    workspaceSlug: string,
    projectId: string,
    req: PostHogConnectorConfigRequest,
  ): Promise<PostHogConnectorConfig> {
    return this.fetchJSON(`${this.projectEp(workspaceSlug, projectId)}/connectors/posthog`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(req),
    });
  }

  async deletePostHogConnector(workspaceSlug: string, projectId: string): Promise<void> {
    await this.fetchJSON(`${this.projectEp(workspaceSlug, projectId)}/connectors/posthog`, {
      method: "DELETE",
    });
  }

  async getPostHogDemand(
    workspaceSlug: string,
    projectId: string,
    range: string,
    refresh?: boolean,
  ): Promise<PostHogDemandResponse> {
    const params = new URLSearchParams({ range });
    if (refresh) params.set("refresh", "true");
    return this.fetchJSON(
      `${this.projectEp(workspaceSlug, projectId)}/connectors/posthog/demand?${params}`,
    );
  }

  async uploadToCollection(
    workspaceSlug: string,
    projectId: string,
    collectionId: string,
    files: File[],
    stream?: string,
  ): Promise<UploadFilesResult> {
    const formData = new FormData();
    for (const file of files) {
      formData.append("files", file);
    }
    // Server route (Bowrain AD-011): /:ws/:id/collections/:ref/:cid/items
    const url = `${this.projectEp(workspaceSlug, projectId)}/collections/${this.ref(stream)}/${encodeURIComponent(collectionId)}/items`;
    const resp = await this.fetchImpl(`${this.baseUrl}${url}`, {
      method: "POST",
      headers: this.uploadHeaders(),
      credentials: "same-origin",
      body: formData,
    });
    if (!resp.ok) {
      const body = await resp.text();
      throw httpError(resp, body);
    }
    return resp.json();
  }

  // ── Editor ───────────────────────────────────────────────────────────────

  async getFileBlocks(
    workspaceSlug: string,
    projectId: string,
    fileName: string,
    stream?: string,
    opts?: BlockQueryOptions,
  ): Promise<BlockInfo[]> {
    // Normalise the server's typed `source_runs`/`targets_runs` into the
    // coded-text + spans shape the render primitives consume, so inline codes
    // render faithfully (and identically) across the editor and review
    // surfaces. See components/editor/blockRuns.ts.
    const q = new URLSearchParams({ item: fileName });
    if (opts?.locale) q.set("locale", opts.locale);
    if (opts?.status) q.set("status", opts.status);
    if (opts?.q) q.set("q", opts.q);
    if (opts?.translatable !== undefined) q.set("translatable", String(opts.translatable));
    if (opts?.limit !== undefined) q.set("limit", String(opts.limit));
    if (opts?.offset !== undefined) q.set("offset", String(opts.offset));
    const raw = await this.fetchJSON<ServerBlockInfo[]>(
      `${this.projectEp(workspaceSlug, projectId)}/blocks/${this.ref(stream)}?${q.toString()}`,
    );
    return normalizeServerBlocks(raw);
  }

  async getBlockCounts(
    workspaceSlug: string,
    projectId: string,
    item?: string,
    locale?: string,
    stream?: string,
    opts?: { q?: string; translatable?: boolean },
  ): Promise<BlockCounts> {
    const q = new URLSearchParams();
    if (item) q.set("item", item);
    if (locale) q.set("locale", locale);
    if (opts?.q) q.set("q", opts.q);
    if (opts?.translatable !== undefined) q.set("translatable", String(opts.translatable));
    const qs = q.toString();
    return this.fetchJSON(
      `${this.projectEp(workspaceSlug, projectId)}/blocks/${this.ref(stream)}/counts${qs ? `?${qs}` : ""}`,
    );
  }

  async getBlock(
    workspaceSlug: string,
    projectId: string,
    blockId: string,
    stream?: string,
  ): Promise<BlockInfo> {
    const raw = await this.fetchJSON<ServerBlockInfo>(
      `${this.projectEp(workspaceSlug, projectId)}/blocks/${this.ref(stream)}/${encodeURIComponent(blockId)}`,
    );
    // The same run → coded-text normalisation the list route's elements get.
    return normalizeServerBlocks([raw])[0];
  }

  async getItem(
    workspaceSlug: string,
    projectId: string,
    itemName: string,
    stream?: string,
  ): Promise<ItemInfo> {
    // The item name is a query parameter, not a path segment: names carry
    // slashes, and /items/:ref is the list route.
    return this.fetchJSON(
      `${this.projectEp(workspaceSlug, projectId)}/items/${this.ref(stream)}/one?item=${encodeURIComponent(itemName)}`,
    );
  }

  async bulkReviewBlocks(
    workspaceSlug: string,
    req: BulkReviewBlocksRequest,
  ): Promise<BulkReviewBlocksResult> {
    const { project_id, stream, ...body } = req;
    return this.fetchJSON(
      `${this.projectEp(workspaceSlug, project_id)}/blocks/${this.ref(stream)}/bulk-review`,
      { method: "POST", body: JSON.stringify(body) },
    );
  }

  async bulkApplyMemory(
    workspaceSlug: string,
    req: BulkApplyMemoryRequest,
  ): Promise<BulkApplyMemoryResult> {
    const { project_id, stream, ...body } = req;
    return this.fetchJSON(
      `${this.projectEp(workspaceSlug, project_id)}/blocks/${this.ref(stream)}/bulk-apply-memory`,
      { method: "POST", body: JSON.stringify(body) },
    );
  }

  async getPendingReview(
    workspaceSlug: string,
    projectId: string,
    opts?: PendingReviewOptions,
  ): Promise<PendingReviewPage> {
    const q = new URLSearchParams();
    if (opts?.locales?.length) q.set("locales", opts.locales.join(","));
    if (opts?.limit) q.set("limit", String(opts.limit));
    if (opts?.offset) q.set("offset", String(opts.offset));
    // Presence of the key is the filter, so the ungrouped scope — the empty
    // collection id — sets an empty value rather than dropping the parameter.
    if (opts?.collectionId !== undefined) q.set("collection", opts.collectionId);
    const raw = await this.fetchJSON<{
      entries: {
        block_id: string;
        item_name: string;
        locale: string;
        block?: ServerBlockInfo;
        collection_id?: string;
        term_compliance?: TermCompliance;
        voice_score?: number;
        voice_bar?: number;
      }[];
      total: number;
      limit: number;
      offset: number;
    }>(
      `${this.projectEp(workspaceSlug, projectId)}/pending-review/${this.ref(opts?.stream)}?${q.toString()}`,
    );
    return {
      ...raw,
      entries: raw.entries.map((e) => ({
        ...e,
        collection_id: e.collection_id ?? "",
        block: e.block ? normalizeServerBlocks([e.block])[0] : undefined,
      })),
    };
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
    // The @neokapi/ui editor still authors coded text + spans; the server
    // consumes RFC 0001 runs (PUT .../runs — there is no /coded route), so
    // convert at the boundary, exactly like WailsApiAdapter does for the
    // desktop backend.
    await this.fetchJSON(
      `${this.projectEp(workspaceSlug, req.project_id)}/blocks/${this.ref(req.stream)}/${req.block_id}/runs`,
      {
        method: "PUT",
        body: JSON.stringify({
          target_locale: req.target_locale,
          runs: codedToRuns(req.coded_text, req.spans),
        }),
      },
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

  async memoryTranslateFile(
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
    opts?: TranslationDashboardItemOpts,
  ): Promise<TranslationDashboardStats> {
    const params = new URLSearchParams();
    if (opts?.itemCollection) params.set("collection", opts.itemCollection);
    if (opts?.itemUngrouped) params.set("ungrouped", "1");
    if (opts?.itemLimit !== undefined) params.set("limit", String(opts.itemLimit));
    if (opts?.itemOffset) params.set("offset", String(opts.itemOffset));
    if (opts?.itemSort) params.set("sort", opts.itemSort);
    if (opts?.itemDir) params.set("dir", opts.itemDir);
    const qs = params.size > 0 ? `?${params.toString()}` : "";
    return this.fetchJSON(
      `${this.projectEp(workspaceSlug, projectId)}/dashboard/${this.ref(stream)}${qs}`,
    );
  }

  async lookupMemoryForBlock(
    workspaceSlug: string,
    projectId: string,
    itemName: string,
    blockId: string,
    targetLocale: string,
    stream?: string,
  ): Promise<MemoryMatchInfo[]> {
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

  async getReviewContext(
    workspaceSlug: string,
    projectId: string,
    itemName: string,
    blockId: string,
    targetLocale: string,
    stream?: string,
  ): Promise<ReviewContext> {
    return this.fetchJSON(
      `${this.projectEp(workspaceSlug, projectId)}/blocks/${this.ref(stream)}/${blockId}/review-context?item=${encodeURIComponent(itemName)}&target_locale=${encodeURIComponent(targetLocale)}`,
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

  async reviewBlock(
    workspaceSlug: string,
    projectId: string,
    itemName: string,
    blockId: string,
    targetLocale: string,
    reviewed: boolean,
    stream?: string,
    demoteTo?: ReviewDemotion,
  ): Promise<void> {
    await this.fetchJSON(
      `${this.projectEp(workspaceSlug, projectId)}/blocks/${this.ref(stream)}/${blockId}/review`,
      {
        method: "PUT",
        body: JSON.stringify({
          item_name: itemName,
          target_locale: targetLocale,
          reviewed,
          // The demotion rung only applies to a clearing call; the server
          // rejects status alongside reviewed=true.
          status: !reviewed && demoteTo ? demoteTo : undefined,
        }),
      },
    );
  }

  async approvePassingReview(
    workspaceSlug: string,
    projectId: string,
    req: ApprovePassingRequest = {},
  ): Promise<ApprovePassingResult> {
    return this.fetchJSON(`${this.projectEp(workspaceSlug, projectId)}/review/approve-passing`, {
      method: "POST",
      body: JSON.stringify(req),
    });
  }

  async createSourceProposal(
    workspaceSlug: string,
    projectId: string,
    req: CreateSourceProposalRequest,
  ): Promise<SourceProposal> {
    // The source-proposal route carries no :ref segment, so the block's stream
    // rides as a query param (the server's streamParam reads ?stream=).
    const q = req.stream ? `?stream=${encodeURIComponent(req.stream)}` : "";
    return this.fetchJSON(`${this.projectEp(workspaceSlug, projectId)}/source-proposals${q}`, {
      method: "POST",
      body: JSON.stringify(req),
    });
  }

  async listSourceProposals(workspaceSlug: string, projectId: string): Promise<SourceProposal[]> {
    const res = await this.fetchJSON<{ proposals: SourceProposal[] }>(
      `${this.projectEp(workspaceSlug, projectId)}/source-proposals`,
    );
    return res.proposals ?? [];
  }

  async decideSourceProposal(
    workspaceSlug: string,
    projectId: string,
    proposalId: string,
    decision: "approve" | "reject",
    reason?: string,
  ): Promise<DecideSourceProposalResult> {
    return this.fetchJSON(
      `${this.projectEp(workspaceSlug, projectId)}/source-proposals/${encodeURIComponent(proposalId)}/decide`,
      { method: "POST", body: JSON.stringify({ decision, reason }) },
    );
  }

  async promoteEntityToConcept(
    workspaceSlug: string,
    projectId: string,
    itemName: string,
    blockId: string,
    entityKey: string,
    stream?: string,
  ): Promise<{ ok: boolean; concept: ConceptInfo }> {
    // The popover passes the overlay key ("entity:N"); the route takes the index.
    const idx = entityKey.replace(/^entity:/, "");
    return this.fetchJSON(
      `${this.projectEp(workspaceSlug, projectId)}/blocks/${this.ref(stream)}/${encodeURIComponent(blockId)}/entities/${encodeURIComponent(idx)}/promote-to-concept?item=${encodeURIComponent(itemName)}`,
      { method: "POST" },
    );
  }

  async recordVoiceCorrection(
    workspaceSlug: string,
    projectId: string,
    req: VoiceCorrectionRequest,
    stream?: string,
  ): Promise<VoiceCorrectionResult> {
    return this.fetchJSON(
      `${this.projectEp(workspaceSlug, projectId)}/voice/${this.ref(stream)}/corrections`,
      { method: "POST", body: JSON.stringify(req) },
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
  async demoteVoiceRule(workspaceSlug: string, profileId: string, term: string): Promise<void> {
    await this.fetchJSON(`/api/v1/${workspaceSlug}/voice-profiles/${profileId}/demote-rule`, {
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

  // ── Content Memory ───────────────────────────────────────────────────

  private memoryEp(ws: string) {
    return `/api/v1/${ws}/translation-memory`;
  }

  async getMemoryEntries(
    workspaceSlug: string,
    query: string,
    defaultSourceLanguage: string,
    targetLocale: string,
    offset: number,
    limit: number,
  ): Promise<MemorySearchResult> {
    const params = new URLSearchParams({
      q: query,
      default_source_language: defaultSourceLanguage,
      target_locale: targetLocale,
      offset: String(offset),
      limit: String(limit),
    });
    return this.fetchJSON(`${this.memoryEp(workspaceSlug)}?${params}`);
  }

  async getMemoryCount(workspaceSlug: string): Promise<number> {
    const resp: { count: number } = await this.fetchJSON(`${this.memoryEp(workspaceSlug)}/count`);
    return resp.count;
  }

  async addMemoryEntry(
    workspaceSlug: string,
    source: string,
    target: string,
    defaultSourceLanguage: string,
    targetLocale: string,
  ): Promise<MemoryEntryInfo> {
    return this.fetchJSON(this.memoryEp(workspaceSlug), {
      method: "POST",
      body: JSON.stringify({
        source,
        target,
        default_source_language: defaultSourceLanguage,
        target_locale: targetLocale,
      }),
    });
  }

  async updateMemoryEntry(workspaceSlug: string, req: MemoryUpdateRequest): Promise<void> {
    await this.fetchJSON(`${this.memoryEp(workspaceSlug)}/${req.entry_id}`, {
      method: "PUT",
      body: JSON.stringify(req),
    });
  }

  async deleteMemoryEntry(workspaceSlug: string, entryId: string): Promise<void> {
    await this.fetchJSON(`${this.memoryEp(workspaceSlug)}/${entryId}`, {
      method: "DELETE",
    });
  }

  async bulkDeleteMemoryEntries(
    workspaceSlug: string,
    entryIds: string[],
  ): Promise<BulkDeleteResult> {
    return this.fetchJSON(`${this.memoryEp(workspaceSlug)}/bulk-delete`, {
      method: "POST",
      body: JSON.stringify({ ids: entryIds }),
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

  async bulkDeleteConcepts(workspaceSlug: string, conceptIds: string[]): Promise<never> {
    // The batch is validated and then refused with the same 409 and change-set
    // hint the per-concept route returns.
    return this.fetchJSON(`${this.termsEp(workspaceSlug)}/bulk-delete`, {
      method: "POST",
      body: JSON.stringify({ ids: conceptIds }),
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
    opts?: { limit?: number; cursor?: string },
  ): Promise<AutomationHistoryPage> {
    const q = new URLSearchParams();
    if (opts?.limit !== undefined) q.set("limit", String(opts.limit));
    if (opts?.cursor) q.set("cursor", opts.cursor);
    const qs = q.toString();
    return this.fetchJSON(
      `${this.automationsEp(workspaceSlug, projectId)}/history${qs ? `?${qs}` : ""}`,
    );
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

  // ── Voice ──────────────────────────────────────────────────────────

  private voiceEp(ws: string) {
    return `/api/v1/${ws}/voice-profiles`;
  }

  async listVoiceProfiles(workspaceSlug: string): Promise<VoiceProfile[]> {
    return this.fetchJSON(this.voiceEp(workspaceSlug));
  }

  async listContextProfiles(workspaceSlug: string): Promise<ContextProfilesResponse> {
    return this.fetchJSON(`/api/v1/${workspaceSlug}/profiles`);
  }

  async listChannelProposals(
    workspaceSlug: string,
    status?: ChannelProposalStatus,
  ): Promise<ChannelAliasProposalsResponse> {
    const query = status ? `?status=${encodeURIComponent(status)}` : "";
    return this.fetchJSON(`/api/v1/${workspaceSlug}/context/channel-proposals${query}`);
  }

  async judgeChannelProposal(
    workspaceSlug: string,
    judgement: ChannelAliasJudgement,
  ): Promise<ChannelAliasProposal> {
    return this.fetchJSON(`/api/v1/${workspaceSlug}/context/channel-proposals/judge`, {
      method: "POST",
      body: JSON.stringify(judgement),
    });
  }

  async getVoiceProfile(workspaceSlug: string, profileId: string): Promise<VoiceProfile> {
    return this.fetchJSON(`${this.voiceEp(workspaceSlug)}/${encodeURIComponent(profileId)}`);
  }

  async createVoiceProfile(
    workspaceSlug: string,
    data: CreateVoiceProfileRequest,
  ): Promise<VoiceProfile> {
    return this.fetchJSON(this.voiceEp(workspaceSlug), {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  async updateVoiceProfile(
    workspaceSlug: string,
    data: UpdateVoiceProfileRequest,
  ): Promise<VoiceProfile> {
    return this.fetchJSON(`${this.voiceEp(workspaceSlug)}/${encodeURIComponent(data.id)}`, {
      method: "PUT",
      body: JSON.stringify(data),
    });
  }

  async deleteVoiceProfile(workspaceSlug: string, profileId: string): Promise<void> {
    await this.fetchJSON(`${this.voiceEp(workspaceSlug)}/${encodeURIComponent(profileId)}`, {
      method: "DELETE",
    });
  }

  async getVoiceScores(workspaceSlug: string, projectId: string): Promise<StoredScore[]> {
    return this.fetchJSON(`${this.projectEp(workspaceSlug, projectId)}/voice/${this.ref()}/scores`);
  }

  async getVoiceTrends(workspaceSlug: string, projectId: string): Promise<ScoreTrend[]> {
    return this.fetchJSON(`${this.projectEp(workspaceSlug, projectId)}/voice/${this.ref()}/trends`);
  }

  async getVoiceRollup(workspaceSlug: string, opts?: VoiceRollupOptions): Promise<VoiceRollup> {
    const q = new URLSearchParams();
    if (opts?.limit) q.set("limit", String(opts.limit));
    if (opts?.offset) q.set("offset", String(opts.offset));
    if (opts?.recentDays) q.set("recent_days", String(opts.recentDays));
    if (opts?.minScore) q.set("min_score", String(opts.minScore));
    if (opts?.dropPoints) q.set("drop_points", String(opts.dropPoints));
    const qs = q.toString();
    return this.fetchJSON(`/api/v1/${workspaceSlug}/voice/rollup${qs ? `?${qs}` : ""}`);
  }

  // ── Correction-learning loop (AD-019) ──────────────────────────────────────

  async listVoiceCandidates(
    workspaceSlug: string,
    profileId: string,
    opts?: { minCount?: number; all?: boolean },
  ): Promise<CandidateRule[]> {
    const q = new URLSearchParams();
    if (opts?.minCount) q.set("min_count", String(opts.minCount));
    if (opts?.all) q.set("all", "true");
    const qs = q.toString();
    return this.fetchJSON(
      `${this.voiceEp(workspaceSlug)}/${encodeURIComponent(profileId)}/candidates${qs ? `?${qs}` : ""}`,
    );
  }

  async promoteVoiceRule(
    workspaceSlug: string,
    profileId: string,
    rule: { term: string; replacement?: string; correction_count?: number },
  ): Promise<{ promoted: boolean }> {
    return this.fetchJSON(
      `${this.voiceEp(workspaceSlug)}/${encodeURIComponent(profileId)}/promote-rule`,
      { method: "POST", body: JSON.stringify(rule) },
    );
  }

  async rejectVoiceRule(
    workspaceSlug: string,
    profileId: string,
    rule: { term: string; replacement?: string },
  ): Promise<void> {
    await this.fetchJSON(
      `${this.voiceEp(workspaceSlug)}/${encodeURIComponent(profileId)}/reject-rule`,
      { method: "POST", body: JSON.stringify(rule) },
    );
  }

  async evaluateVoiceRule(
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
      `${this.voiceEp(workspaceSlug)}/${encodeURIComponent(profileId)}/evaluate-rule`,
      { method: "POST", body: JSON.stringify(req) },
    );
  }

  async getVoiceDrift(
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
      `${this.projectEp(workspaceSlug, projectId)}/voice/${this.ref()}/drift${qs ? `?${qs}` : ""}`,
    );
  }

  async createProfileFromStarter(
    workspaceSlug: string,
    pack: string,
    name?: string,
  ): Promise<VoiceProfile> {
    return this.fetchJSON(this.voiceEp(workspaceSlug) + "/from-starter", {
      method: "POST",
      body: JSON.stringify({ pack, ...(name ? { name } : {}) }),
    });
  }

  // ── Context scan (AI brand onboarding — epic 016) ─────────────────────────────

  private contextScanEp(ws: string) {
    return `/api/v1/${ws}/context-scans`;
  }

  async uploadContextScanSources(
    workspaceSlug: string,
    files: File[],
  ): Promise<ContextScanUploadResult> {
    const formData = new FormData();
    for (const file of files) {
      formData.append("files", file);
    }
    const resp = await this.fetchImpl(
      `${this.baseUrl}${this.contextScanEp(workspaceSlug)}/uploads`,
      {
        method: "POST",
        headers: this.uploadHeaders(),
        credentials: "same-origin",
        body: formData,
      },
    );
    if (!resp.ok) {
      const body = await resp.text();
      throw httpError(resp, body);
    }
    return resp.json();
  }

  async startContextScan(
    workspaceSlug: string,
    req: ContextScanRequest,
  ): Promise<{ job_id: string }> {
    return this.fetchJSON(this.contextScanEp(workspaceSlug), {
      method: "POST",
      body: JSON.stringify(req),
    });
  }

  async getContextScan(workspaceSlug: string, jobId: string): Promise<ContextScanJob> {
    return this.fetchJSON(`${this.contextScanEp(workspaceSlug)}/${encodeURIComponent(jobId)}`);
  }

  async approveContextScan(
    workspaceSlug: string,
    scanId: string,
    req: ContextScanApproveRequest,
  ): Promise<ContextScanApproveResult> {
    return this.fetchJSON(
      `${this.contextScanEp(workspaceSlug)}/${encodeURIComponent(scanId)}/approve`,
      { method: "POST", body: JSON.stringify(req) },
    );
  }

  async checkVoiceDraft(
    workspaceSlug: string,
    profile: VoiceProfile,
    text: string,
  ): Promise<ContextScanCheckResult> {
    return this.fetchJSON(`${this.contextScanEp(workspaceSlug)}/check-draft`, {
      method: "POST",
      body: JSON.stringify({ profile, text }),
    });
  }

  async approveAxis(
    workspaceSlug: string,
    projectId: string,
    req: ApproveAxisRequest,
  ): Promise<PendingRecipeChange> {
    return this.fetchJSON(
      `/api/v1/${workspaceSlug}/projects/${encodeURIComponent(projectId)}/axes`,
      { method: "POST", body: JSON.stringify(req) },
    );
  }

  // ── Activities (Bowrain AD-014) ─────────────────────────────────────────────────

  async listActivities(
    workspaceSlug: string,
    query?: {
      project_id?: string;
      stream?: string;
      actor_id?: string;
      type?: string;
      types?: string[];
      cursor?: string;
      limit?: number;
    },
  ): Promise<ActivityPage> {
    const params = new URLSearchParams();
    if (query?.project_id) params.set("project_id", query.project_id);
    if (query?.stream) params.set("stream", query.stream);
    if (query?.actor_id) params.set("actor_id", query.actor_id);
    if (query?.types?.length) params.set("types", query.types.join(","));
    else if (query?.type) params.set("type", query.type);
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

  // ── Convergence runs (server-side `kapi up`) — Bowrain AD-022 ────────────

  private convergenceRunsEp(ws: string, projectId: string) {
    return `${this.projectEp(ws, projectId)}/convergence/runs`;
  }

  async listConvergenceRuns(
    workspaceSlug: string,
    projectId: string,
    limit?: number,
  ): Promise<ConvergenceRun[]> {
    const qs = limit ? `?limit=${limit}` : "";
    return this.fetchJSON(`${this.convergenceRunsEp(workspaceSlug, projectId)}${qs}`);
  }

  async getConvergenceRun(
    workspaceSlug: string,
    projectId: string,
    runId: string,
  ): Promise<ConvergenceRun> {
    return this.fetchJSON(
      `${this.convergenceRunsEp(workspaceSlug, projectId)}/${encodeURIComponent(runId)}`,
    );
  }

  async startConvergenceRun(
    workspaceSlug: string,
    projectId: string,
    opts?: {
      trigger?: string;
      locales?: string[];
      scope?: ConvergenceRunScope;
      confirmed?: boolean;
    },
  ): Promise<ConvergenceRun | null> {
    // Scope "none" is transport-only: the server answers 204 and starts no run,
    // so fetchJSON resolves undefined — normalize to null for the caller.
    const run = await this.fetchJSON<ConvergenceRun | undefined>(
      this.convergenceRunsEp(workspaceSlug, projectId),
      {
        method: "POST",
        body: JSON.stringify({
          trigger: opts?.trigger ?? "manual",
          locales: opts?.locales,
          scope: opts?.scope,
          confirmed: opts?.confirmed,
        }),
      },
    );
    return run ?? null;
  }

  async cancelConvergenceRun(
    workspaceSlug: string,
    projectId: string,
    runId: string,
  ): Promise<void> {
    await this.fetchJSON(
      `${this.convergenceRunsEp(workspaceSlug, projectId)}/${encodeURIComponent(runId)}/cancel`,
      { method: "POST" },
    );
  }

  async estimateConvergence(
    workspaceSlug: string,
    projectId: string,
  ): Promise<ConvergenceEstimate> {
    return this.fetchJSON(`${this.projectEp(workspaceSlug, projectId)}/convergence/estimate`);
  }

  async getLoopRollup(workspaceSlug: string): Promise<LoopRollup> {
    return this.fetchJSON(`/api/v1/${workspaceSlug}/loop-rollup`);
  }

  // ── Tasks (Bowrain AD-014) ────────────────────────────────────────────────────

  private tasksEp(ws: string) {
    return `/api/v1/${ws}/tasks`;
  }

  /**
   * The task filters, as the server reads them: the plural forms are
   * comma-separated sets and win over their singular counterparts.
   */
  private taskParams(query?: TaskQuery, paged = true): URLSearchParams {
    const params = new URLSearchParams();
    if (query?.project_id) params.set("project_id", query.project_id);
    if (query?.assignee_id) params.set("assignee_id", query.assignee_id);
    if (query?.statuses?.length) params.set("statuses", query.statuses.join(","));
    else if (query?.status) params.set("status", query.status);
    if (query?.types?.length) params.set("types", query.types.join(","));
    else if (query?.type) params.set("type", query.type);
    if (query?.priority) params.set("priority", query.priority);
    if (paged) {
      if (query?.cursor) params.set("cursor", query.cursor);
      if (query?.limit) params.set("limit", String(query.limit));
    }
    return params;
  }

  async listTasks(workspaceSlug: string, query?: TaskQuery): Promise<TaskPage> {
    const qs = this.taskParams(query).toString();
    return this.fetchJSON(`${this.tasksEp(workspaceSlug)}${qs ? `?${qs}` : ""}`);
  }

  async getTaskCounts(workspaceSlug: string, query?: TaskQuery): Promise<TaskCounts> {
    const qs = this.taskParams(query, false).toString();
    return this.fetchJSON(`${this.tasksEp(workspaceSlug)}/counts${qs ? `?${qs}` : ""}`);
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
    query?: Omit<TaskQuery, "assignee_id">,
  ): Promise<TaskPage> {
    // "My tasks" is the tasks route filtered to the caller: the server folded
    // the former dedicated /my/tasks route into /tasks?assignee_id=me
    // (Bowrain AD-011). The old path fell through to the catch-all and 404'd
    // on fresh workspaces.
    return this.listTasks(workspaceSlug, { ...query, assignee_id: "me" });
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
        const resp = await this.fetchImpl(url, {
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

  // The server speaks Go/snake_case; the UI types are camelCase. The mapping
  // lives here, in the adapter, and nowhere else. (It was missing entirely: the
  // page read overview.credits and overview.stripeCustomerId from a payload that
  // never carried those keys, so the credit card and the Manage-Subscription
  // button silently never rendered.)
  async billingGetOverview(workspaceSlug: string): Promise<BillingOverview> {
    const raw = await this.fetchJSON<{
      plan: BillingPlan;
      status: BillingSubscriptionDTO["status"];
      credits_total: number;
      credits_used: number;
      credits_remaining: number;
      spendable_credits?: number;
      month_resets_at: string;
      subscription?: BillingSubscriptionDTO;
    }>(this.billingEp(workspaceSlug));

    return {
      subscription: {
        plan: raw.subscription?.plan ?? raw.plan,
        status: raw.subscription?.status ?? raw.status,
        // A workspace always has at least one seat. The synthesized free-plan
        // subscription the server returns for a workspace with no row carries
        // seat_count 0, which `??` would happily pass through as "0 seats".
        seatCount: Math.max(raw.subscription?.seat_count ?? 1, 1),
        currentPeriodStart: raw.subscription?.current_period_start,
        currentPeriodEnd: raw.subscription?.current_period_end,
        cancelAt: raw.subscription?.cancel_at,
        trialEndsAt: raw.subscription?.trial_ends_at,
      },
      credits: {
        creditsTotal: raw.credits_total,
        creditsUsed: raw.credits_used,
        creditsRemaining: raw.credits_remaining,
        resetsAt: raw.month_resets_at,
      },
      spendableCredits: raw.spendable_credits ?? raw.credits_remaining,
      stripeCustomerId: raw.subscription?.stripe_customer_id || undefined,
    };
  }

  async billingGetUsage(workspaceSlug: string): Promise<BillingUsageBreakdown> {
    const raw = await this.fetchJSON<{ usage_by_operation?: Record<string, number> }>(
      `${this.billingEp(workspaceSlug)}/usage`,
    );
    const byOp = raw.usage_by_operation ?? {};
    const total = Object.values(byOp).reduce((sum, n) => sum + n, 0);
    return {
      aiTranslation: byOp.ai_translation ?? 0,
      aiQualityCheck: byOp.ai_quality_check ?? 0,
      bravoMessages: byOp.bravo_message ?? 0,
      bravoContainer: byOp.bravo_container ?? 0,
      // The total is the sum of everything charged, not of the four named rows —
      // a new operation type (context_scan, say) must land in the total rather than
      // vanish from it.
      total,
    };
  }

  async billingGetPlans(workspaceSlug: string): Promise<BillingPlansResponse> {
    return this.fetchJSON(`${this.billingEp(workspaceSlug)}/plans`);
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
    plan: BillingPlan,
    successUrl: string,
    cancelUrl: string,
    seats?: number,
  ): Promise<{ url: string }> {
    const { checkout_url } = await this.fetchJSON<{ checkout_url: string }>(
      `${this.billingEp(workspaceSlug)}/checkout`,
      {
        method: "POST",
        body: JSON.stringify({
          plan,
          seats,
          success_url: successUrl,
          cancel_url: cancelUrl,
        }),
      },
    );
    return { url: checkout_url };
  }

  async billingBuyCredits(
    workspaceSlug: string,
    successUrl: string,
    cancelUrl: string,
  ): Promise<{ url: string }> {
    const { checkout_url } = await this.fetchJSON<{ checkout_url: string }>(
      `${this.billingEp(workspaceSlug)}/buy-credits`,
      {
        method: "POST",
        body: JSON.stringify({ success_url: successUrl, cancel_url: cancelUrl }),
      },
    );
    return { url: checkout_url };
  }

  async billingCreatePortal(workspaceSlug: string, returnUrl: string): Promise<{ url: string }> {
    const { portal_url } = await this.fetchJSON<{ portal_url: string }>(
      `${this.billingEp(workspaceSlug)}/portal`,
      {
        method: "POST",
        body: JSON.stringify({ return_url: returnUrl }),
      },
    );
    return { url: portal_url };
  }

  // The ledger is the `entries` of the usage response — there is no /ledger route
  // (the old call to one 404'd, which is why Credit Transactions was always
  // empty).
  async billingGetLedger(
    workspaceSlug: string,
    from?: string,
    to?: string,
    opts?: { limit?: number; offset?: number; operation?: string },
  ): Promise<CreditLedgerEntry[]> {
    const page = await this.billingGetLedgerPage(workspaceSlug, { from, to, ...opts });
    return page.entries;
  }

  async billingGetLedgerPage(
    workspaceSlug: string,
    query?: CreditLedgerQuery,
  ): Promise<CreditLedgerPage> {
    const params = new URLSearchParams();
    if (query?.from) params.set("from", query.from);
    if (query?.to) params.set("to", query.to);
    if (query?.limit !== undefined) params.set("limit", String(query.limit));
    if (query?.offset !== undefined) params.set("offset", String(query.offset));
    if (query?.operation) params.set("operation", query.operation);
    const qs = params.toString();
    const raw = await this.fetchJSON<{
      entries?: CreditLedgerEntryDTO[];
      total?: number;
      limit?: number;
      offset?: number;
      usage_by_operation?: Record<string, number>;
      net_by_operation?: Record<string, number>;
      from?: string;
      to?: string;
    }>(`${this.billingEp(workspaceSlug)}/usage${qs ? `?${qs}` : ""}`);
    return {
      entries: (raw.entries ?? []).map((e) => ({
        id: String(e.id),
        amount: e.amount,
        balanceAfter: e.balance_after,
        operation: e.operation,
        referenceId: e.reference_id,
        createdAt: e.created_at,
      })),
      total: raw.total ?? 0,
      limit: raw.limit ?? 0,
      offset: raw.offset ?? 0,
      usage_by_operation: raw.usage_by_operation ?? {},
      net_by_operation: raw.net_by_operation ?? {},
      from: raw.from ?? "",
      to: raw.to ?? "",
    };
  }

  // ── Brand knowledge graph — Concepts (AD-021) ─────────────────────────────
  // The concept routes share the /concepts base (termsEp) with the Terminology
  // block above; these add the graph/governance surface.

  private conceptEp(ws: string, conceptId: string) {
    // Concept IDs are path segments that may carry a colon (e.g. the
    // terms's `term:<hash>` IDs). `:` is path-legal (RFC 3986 pchar) and the
    // server stores/matches IDs raw, so we must NOT percent-encode it —
    // encodeURIComponent's `%3A` would 404. Encode everything else (notably `/`).
    return `${this.termsEp(ws)}/${encodeConceptSegment(conceptId)}`;
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
    if (params?.at) q.set("at", params.at);
    if (params?.sort) q.set("sort", params.sort);
    if (params?.offset !== undefined) q.set("offset", String(params.offset));
    if (params?.limit !== undefined) q.set("limit", String(params.limit));
    const qs = q.toString();
    return this.fetchJSON(`${this.termsEp(workspaceSlug)}${qs ? `?${qs}` : ""}`);
  }

  async getConceptStatusCounts(workspaceSlug: string): Promise<ConceptStatusCounts> {
    return this.fetchJSON(`${this.termsEp(workspaceSlug)}/status-counts`);
  }

  async getConceptLocaleCoverage(workspaceSlug: string): Promise<LocaleCoverageReport> {
    return this.fetchJSON(`${this.termsEp(workspaceSlug)}/locale-coverage`);
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

  async getConceptProjects(
    workspaceSlug: string,
    conceptId: string,
    query: ConceptProjectsQuery = {},
  ): Promise<ConceptProjects> {
    const params = new URLSearchParams();
    if (query.project) params.set("project", query.project);
    if (query.at) params.set("at", query.at);
    if (query.market) params.set("market", query.market);
    if (query.limit) params.set("limit", String(query.limit));
    const search = params.toString();
    return this.fetchJSON(
      `/api/v1/${workspaceSlug}/context/concepts/${encodeConceptSegment(conceptId)}/projects${
        search ? `?${search}` : ""
      }`,
    );
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

  async getChangesetCounts(workspaceSlug: string): Promise<ChangeSetCounts> {
    return this.fetchJSON(`${this.changesetsEp(workspaceSlug)}/counts`);
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

  async refreshChangesetBlastRadius(
    workspaceSlug: string,
    changesetId: string,
  ): Promise<ChangeSetImpact> {
    // `fresh=1` is the one value the server reads as "walk it now"; nothing is
    // persisted, so the stored summary is left as it was.
    return this.fetchJSON(`${this.changesetEp(workspaceSlug, changesetId)}/blast-radius?fresh=1`);
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

  async trialFindings(
    workspaceSlug: string,
    changesetId: string,
    projectId: string,
    stream: string,
  ): Promise<TrialReport> {
    return this.fetchJSON(
      `${this.changesetEp(workspaceSlug, changesetId)}/pilots/${encodeURIComponent(projectId)}/${encodeURIComponent(stream)}/findings`,
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
