import type { ApiAdapter, ComponentSchema } from "@neokapi/ui";
import { ALL_PERMISSIONS, governedRefusalError } from "@neokapi/ui";
import type {
  CallerPermissions,
  ConvergenceRun,
  ConvergenceEstimate,
  ConvergenceRunScope,
  LoopRollup,
  User,
  Workspace,
  Membership,
  ProjectInfo,
  UploadFilesResult,
  ConfigResponse,
  PublicPlatformConfig,
  ModelRecommendationsResponse,
  BlockInfo,
  PendingReviewOptions,
  PendingReviewPage,
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
  CheckIssue,
  FileCheckResult,
  AutomationRule,
  AutomationEvent,
  SaveAutomationRuleRequest,
  NotificationInfo,
  EntityInfo,
  StreamInfo,
  StreamDiffResult,
  StreamMergeResult,
  CollectionInfo,
  ConnectorInfo,
  ConnectorSyncStatus,
  ConnectorContentItem,
  PostHogConnectorConfig,
  PostHogDemandResponse,
  AuditEntry,
  AuditChainVerification,
  SoDMode,
  Group,
  GroupRoleBinding,
  DenyRule,
  DenyRuleInput,
  ArchivedProject,
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
  ModelUsageResponse,
  TranslationDashboardStats,
  TranslationDashboardItemOpts,
  ActivityPage,
  TaskInfo,
  TaskPage,
  CreateTaskRequest,
  NotificationPreference,
  RoleTemplate,
  ProjectMembership,
  StreamTag,
  StreamTagKind,
  CreateStreamTagRequest,
  AutomationRun,
  AutomationStep,
  AutomationLogEntry,
  DigestSettingsDTO,
  BravoConversation,
  BravoMessage,
  BravoConfig,
  BravoToolInfo,
  BravoUsageSummary,
  BravoSSEHandler,
  BillingOverview,
  BillingPlan,
  BillingPlansResponse,
  BillingUsageBreakdown,
  CreditLedgerEntry,
  ListConceptsParams,
  ConceptStory,
  ConceptRelation,
  ConceptUsage,
  ConceptProjects,
  ConceptProjectsQuery,
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
  MergeResult,
  Pilot,
  StartPilotRequest,
  ReviewRung,
  ApprovePassingResult,
  VoiceCorrectionRequest,
  VoiceCorrectionResult,
  ContextScanRequest,
  ContextScanUploadResult,
  ContextScanJob,
  ContextScanCheckResult,
  PendingRecipeChange,
  ContextProfilesResponse,
  ChannelAliasJudgement,
  ChannelAliasProposal,
  ChannelAliasProposalsResponse,
  ChannelProposalStatus,
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
  ContextScanApproveResult,
  ConceptStatusCounts,
  LocaleCoverageReport,
  ChangeSetCounts,
} from "@neokapi/ui";

import { codedToRuns } from "./codedToRuns";

/**
 * Presence-collaboration session info surfaced by the Go backend so the webview
 * can open the Yjs awareness WebSocket directly (params.token). Mirrors the
 * backend's CollabSession struct and the @neokapi/ui useCollaboration options.
 */
export interface CollabSessionInfo {
  serverUrl: string;
  authToken: string;
  workspace: string;
  user: { userId: string; name: string; avatarUrl?: string };
}

import { Backend } from "./backend";
import { optionalBinding } from "./optionalBinding";

/**
 * ApiAdapter implementation for the Bowrain desktop app.
 * Wraps Wails v3 generated bindings to satisfy the shared ApiAdapter interface.
 * Workspace parameters are ignored (single-user desktop context).
 */
export class WailsApiAdapter implements ApiAdapter {
  // --- Server config ---
  async getConfig(): Promise<ConfigResponse> {
    const v = await Backend.GetVersion();
    return {
      mode: "standalone",
      version: v.version,
      commit: v.commit,
      build_date: v.build_date,
      // The desktop app has no brand-scan job system (the adapter's scan
      // methods throw), so the hosted-scan entry points stay hidden.
      features: { context_scan: false },
    };
  }

  // --- Measured steerability (model recommendation sweeps) ---
  // Sweeps are a multi-tenant platform QC surface (platform provider, ctrl
  // gate); the desktop runs against a local runtime with no sweep worker, so
  // the panel renders its disabled state.
  async getModelRecommendations(): Promise<ModelRecommendationsResponse> {
    return { enabled: false, locales: [] };
  }

  async refreshModelRecommendations(): Promise<never> {
    throw new Error("model sweeps are not available in the desktop app");
  }

  async getPublicPlatformConfig(): Promise<PublicPlatformConfig> {
    // The desktop app is single-tenant against a local runtime: there is no
    // platform-managed model gate, so signups are irrelevant, no maintenance
    // window applies, and workspace-level model choice is off (the desktop uses
    // its own provider configuration).
    return {
      signups_open: true,
      maintenance: { enabled: false },
      ai_customer_choice: false,
    };
  }

  // --- Auth (not applicable in desktop) ---
  async getCurrentUser(): Promise<User | null> {
    return {
      id: "local",
      email: "local@bowrain",
      name: "Local User",
      avatar_url: "",
      onboarded_at: new Date(0).toISOString(),
    };
  }

  // --- Account management (not applicable in desktop) ---
  async getOnboardingStatus() {
    return { needs_onboarding: false, email: "local@bowrain" };
  }
  async completeOnboarding(): Promise<Workspace> {
    throw new Error("onboarding is not available in the desktop app");
  }
  async checkSlug() {
    return { available: true };
  }
  async requestEmailChange(): Promise<never> {
    throw new Error("email change is not available in the desktop app");
  }
  async confirmEmailChange(): Promise<never> {
    throw new Error("email change is not available in the desktop app");
  }
  // Passkey management is a web account-surface feature; the desktop app is a
  // server working copy, not an account-management tool.
  async getAccountSecurity(): Promise<never> {
    throw new Error("passkey management is not available in the desktop app");
  }
  async listPasskeys(): Promise<never> {
    throw new Error("passkey management is not available in the desktop app");
  }
  async passkeyRegisterStart(): Promise<never> {
    throw new Error("passkey management is not available in the desktop app");
  }
  async passkeyRegisterFinish(): Promise<never> {
    throw new Error("passkey management is not available in the desktop app");
  }
  async deletePasskey(): Promise<never> {
    throw new Error("passkey management is not available in the desktop app");
  }
  async adminListSlugReservations() {
    return [];
  }
  async adminReleaseSlugReservation(): Promise<void> {
    throw new Error("admin actions are not available in the desktop app");
  }

  // --- Workspaces (single workspace in desktop) ---
  async listWorkspaces(): Promise<Workspace[]> {
    return [
      {
        id: "local",
        name: "Personal",
        slug: "personal",
        description: "",
        logo_url: "",
        type: "personal" as const,
        role: "owner",
      },
    ];
  }
  async createWorkspace(): Promise<Workspace> {
    throw new Error("Workspaces not supported in desktop mode");
  }
  async getWorkspace(): Promise<Workspace> {
    return {
      id: "local",
      name: "Personal",
      slug: "personal",
      description: "",
      logo_url: "",
      type: "personal" as const,
      role: "owner",
    };
  }
  async updateWorkspace(): Promise<Workspace> {
    throw new Error("Workspaces not supported in desktop mode");
  }
  async deleteWorkspace(): Promise<void> {
    throw new Error("Workspaces not supported in desktop mode");
  }

  // --- Members (proxied to the server's REST governance endpoints) ---
  async listMembers(workspaceSlug: string): Promise<Membership[]> {
    return Backend.ListMembers(workspaceSlug) as Promise<Membership[]>;
  }
  async addMember(workspaceSlug: string, userId: string, role: string): Promise<void> {
    return Backend.AddMember(workspaceSlug, userId, role);
  }
  async updateMemberRole(workspaceSlug: string, userId: string, role: string): Promise<void> {
    return Backend.UpdateMemberRole(workspaceSlug, userId, role);
  }
  async removeMember(workspaceSlug: string, userId: string): Promise<void> {
    return Backend.RemoveMember(workspaceSlug, userId);
  }

  // --- Invites (proxied to the server's REST governance endpoints) ---
  async listInvites(workspaceSlug: string): Promise<Invite[]> {
    return Backend.ListInvites(workspaceSlug) as Promise<Invite[]>;
  }
  async createInvite(
    workspaceSlug: string,
    email: string,
    role: string,
    maxUses: number,
  ): Promise<Invite> {
    return Backend.CreateInvite(workspaceSlug, email, role, maxUses) as Promise<Invite>;
  }
  async deleteInvite(workspaceSlug: string, inviteId: string): Promise<void> {
    return Backend.DeleteInvite(workspaceSlug, inviteId);
  }
  async acceptInvite(): Promise<AcceptInviteResponse> {
    throw new Error("Not supported in desktop mode");
  }
  async claimProject(): Promise<ClaimProjectResponse> {
    throw new Error("Not supported in desktop mode");
  }

  // --- Role Templates (not applicable in desktop) ---
  async listRoleTemplates(_ws: string): Promise<RoleTemplate[]> {
    return [];
  }
  async createRoleTemplate(
    _ws: string,
    _data: {
      name: string;
      display_name: string;
      description: string;
      permissions: string[];
      position?: number;
    },
  ): Promise<RoleTemplate> {
    throw new Error("not implemented in desktop app");
  }
  async updateRoleTemplate(
    _ws: string,
    _roleId: string,
    _data: {
      name?: string;
      display_name?: string;
      description?: string;
      permissions?: string[];
      position?: number;
    },
  ): Promise<RoleTemplate> {
    throw new Error("not implemented in desktop app");
  }
  async deleteRoleTemplate(_ws: string, _roleId: string): Promise<void> {
    throw new Error("not implemented in desktop app");
  }

  // --- Project Members (not applicable in desktop) ---
  async listProjectMembers(_ws: string, _projectId: string): Promise<ProjectMembership[]> {
    return [];
  }
  async addProjectMember(
    _ws: string,
    _projectId: string,
    _data: {
      user_id: string;
      role_id: string;
      languages?: string[];
    },
  ): Promise<ProjectMembership> {
    throw new Error("not implemented in desktop app");
  }
  async updateProjectMember(
    _ws: string,
    _projectId: string,
    _userId: string,
    _data: {
      role_id: string;
      languages?: string[];
    },
  ): Promise<ProjectMembership> {
    throw new Error("not implemented in desktop app");
  }
  async removeProjectMember(_ws: string, _projectId: string, _userId: string): Promise<void> {
    throw new Error("not implemented in desktop app");
  }

  // --- Streams (desktop: single stream mode) ---
  async listStreams(): Promise<StreamInfo[]> {
    return [];
  }
  async createStream(): Promise<StreamInfo> {
    throw new Error("Streams not yet supported in desktop mode");
  }
  async getStream(): Promise<StreamInfo> {
    throw new Error("Streams not yet supported in desktop mode");
  }
  async updateStream(): Promise<StreamInfo> {
    throw new Error("Streams not yet supported in desktop mode");
  }
  async deleteStream(): Promise<void> {
    throw new Error("Streams not yet supported in desktop mode");
  }
  async diffStream(): Promise<StreamDiffResult> {
    throw new Error("Streams not yet supported in desktop mode");
  }
  async mergeStream(): Promise<StreamMergeResult> {
    throw new Error("Streams not yet supported in desktop mode");
  }
  async lockStream(): Promise<StreamInfo> {
    throw new Error("not implemented in desktop app");
  }
  async unlockStream(): Promise<StreamInfo> {
    throw new Error("not implemented in desktop app");
  }

  // --- Stream Tags (not yet supported in desktop) ---
  async listStreamTags(_ws: string, _projectId: string, _streamName: string): Promise<StreamTag[]> {
    return [];
  }
  async createStreamTag(
    _ws: string,
    _projectId: string,
    _streamName: string,
    _req: CreateStreamTagRequest,
  ): Promise<StreamTag> {
    throw new Error("not implemented in desktop app");
  }
  async getStreamTag(
    _ws: string,
    _projectId: string,
    _streamName: string,
    _tagName: string,
  ): Promise<StreamTag> {
    throw new Error("not implemented in desktop app");
  }
  async deleteStreamTag(
    _ws: string,
    _projectId: string,
    _streamName: string,
    _tagName: string,
  ): Promise<void> {
    throw new Error("not implemented in desktop app");
  }
  async listProjectTags(
    _ws: string,
    _projectId: string,
    _kind?: StreamTagKind,
  ): Promise<StreamTag[]> {
    return [];
  }

  // --- API Tokens (not applicable in desktop) ---
  async listApiTokens(): Promise<ApiToken[]> {
    return [];
  }
  async createApiToken(): Promise<CreateApiTokenResponse> {
    throw new Error("Not supported in desktop mode");
  }
  async deleteApiToken(): Promise<void> {
    throw new Error("Not supported in desktop mode");
  }

  // --- Projects ---
  async listProjects(): Promise<ProjectInfo[]> {
    return Backend.ListProjects() as Promise<ProjectInfo[]>;
  }
  async createProject(
    _ws: string,
    name: string,
    sourceLocale: string,
    targetLocales: string[],
  ): Promise<ProjectInfo> {
    return Backend.CreateProject(name, sourceLocale, targetLocales) as Promise<ProjectInfo>;
  }
  async getProject(_ws: string, projectId: string): Promise<ProjectInfo> {
    return Backend.GetProject(projectId) as Promise<ProjectInfo>;
  }
  /**
   * The desktop app opens a project on its own machine, for the person sitting
   * at it. There is no membership to resolve and nothing to withhold, so the
   * caller holds every permission over every language.
   */
  async getCallerPermissions(): Promise<CallerPermissions> {
    return { permissions: [...ALL_PERMISSIONS], languages: [] };
  }
  async updateProject(): Promise<ProjectInfo> {
    throw new Error("Not implemented");
  }
  async deleteProject(_ws: string, projectId: string): Promise<void> {
    return Backend.CloseProject(projectId);
  }
  async uploadFiles(_ws: string, projectId: string, files: File[]): Promise<UploadFilesResult> {
    // In Wails v3, File objects from DnD have a .path property
    const paths = files.map((f) => (f as unknown as { path?: string }).path || f.name);
    const info = (await Backend.AddItems(projectId, paths)) as ProjectInfo;
    // AddItems does not report skipped files; return the shared result shape
    // with an empty skipped list so callers reading `.skipped` are type-correct.
    return { ...info, skipped: [] };
  }
  async removeFile(_ws: string, projectId: string, fileName: string): Promise<ProjectInfo> {
    return Backend.RemoveItem(projectId, fileName) as Promise<ProjectInfo>;
  }

  // --- Archive / Recycle Bin (not applicable in desktop) ---
  async restoreProject(): Promise<void> {}
  async permanentlyDeleteProject(): Promise<void> {}
  async listArchivedProjects(): Promise<ArchivedProject[]> {
    return [];
  }
  async restoreStream(): Promise<void> {}

  // --- Audit Log (not applicable in desktop) ---
  async listWorkspaceAuditLog(): Promise<AuditEntry[]> {
    return [];
  }

  async verifyWorkspaceAuditChain(): Promise<AuditChainVerification> {
    return { chain_key: "", rows: 0, valid: true };
  }

  // --- Collections (not yet implemented in desktop) ---
  async listCollections(): Promise<CollectionInfo[]> {
    return [];
  }
  async createCollection(): Promise<CollectionInfo> {
    throw new Error("Not implemented");
  }
  async getCollection(): Promise<CollectionInfo> {
    throw new Error("Not implemented");
  }
  async updateCollection(): Promise<CollectionInfo> {
    throw new Error("Not implemented");
  }
  async deleteCollection(): Promise<void> {
    throw new Error("Not implemented");
  }
  async uploadToCollection(): Promise<UploadFilesResult> {
    // Unused under the composite adapter (collection uploads proxy to REST); the
    // signature matches ApiAdapter so the class satisfies the interface.
    throw new Error("Not implemented");
  }

  // --- Integration connectors (Bowrain AD-011) ---
  // The desktop keeps connectors in-process (workspace-agnostic), so the
  // workspace argument is ignored, matching the rest of this adapter. These
  // wire straight to the real Wails connector bindings (local-first in the
  // composite adapter), which the shared connectors UI now drives.
  async listConnectors(): Promise<ConnectorInfo[]> {
    const list = ((await Backend.ListConnectors()) ?? []) as {
      id: string;
      name: string;
      category: string;
    }[];
    return list.map((c) => ({ id: c.id, name: c.name, category: c.category }));
  }

  async addConnector(
    _ws: string,
    type: string,
    config: Record<string, string>,
  ): Promise<ConnectorInfo> {
    const c = (await Backend.ConfigureConnector(type, config)) as {
      id: string;
      name: string;
      category: string;
    };
    return { id: c.id, name: c.name, category: c.category };
  }

  async githubSetupState(): Promise<never> {
    throw new Error("GitHub App setup is not available in the desktop app");
  }
  async claimInstallation(): Promise<never> {
    throw new Error("GitHub App setup is not available in the desktop app");
  }
  async listInstallationRepos(): Promise<never> {
    throw new Error("GitHub App setup is not available in the desktop app");
  }
  async detectInstallationRepo(): Promise<never> {
    throw new Error("GitHub App setup is not available in the desktop app");
  }
  async bindInstallationRepo(): Promise<never> {
    throw new Error("GitHub App setup is not available in the desktop app");
  }
  async removeConnector(_ws: string, connectorId: string): Promise<void> {
    await Backend.RemoveConnector(connectorId);
  }

  async getConnectorStatus(_ws: string, connectorId: string): Promise<ConnectorSyncStatus> {
    const s = (await Backend.GetConnectorStatus(connectorId)) as {
      connector_id: string;
      last_sync: string;
      item_count: number;
    };
    return {
      connectorId: s.connector_id,
      lastSync: s.last_sync ?? "",
      itemCount: s.item_count ?? 0,
      fileCount: 0,
      wordCount: 0,
      pendingPull: 0,
      pendingPush: 0,
      errors: [],
    };
  }

  // The desktop's connectors are local, so the batch is the per-connector read
  // run over the selection; an id the local surface cannot answer is unknown,
  // exactly as the server reports one.
  async getConnectorStatuses(ws: string, connectorIds: string[]): Promise<ConnectorStatusBatch> {
    const statuses: Record<string, ConnectorSyncStatus> = {};
    const unknown: string[] = [];
    for (const id of connectorIds) {
      try {
        statuses[id] = await this.getConnectorStatus(ws, id);
      } catch {
        unknown.push(id);
      }
    }
    return { statuses, unknown };
  }

  async fetchConnector(
    _ws: string,
    connectorId: string,
    projectId: string,
  ): Promise<{ items_fetched: number }> {
    const items = ((await Backend.FetchContent(connectorId, projectId)) ?? []) as unknown[];
    return { items_fetched: items.length };
  }

  async publishConnector(
    _ws: string,
    connectorId: string,
    projectId: string,
  ): Promise<{ status: string }> {
    await Backend.PublishContent(connectorId, projectId);
    return { status: "ok" };
  }

  async listConnectorContent(_ws: string, connectorId: string): Promise<ConnectorContentItem[]> {
    // The desktop binding returns its own ContentItemInfo shape
    // ({id, path, title, block_count}), not the server's verbatim ContentItem.
    // Map it into the PascalCase ConnectorContentItem: title → Name, and the
    // fields the binding doesn't carry (Format, Locale, Blocks, Metadata,
    // LastChanged) stay empty/null — matching a server listing where Blocks is
    // null too.
    const items = ((await Backend.ListContentItems(connectorId)) ?? []) as {
      id: string;
      path: string;
      title: string;
      block_count: number;
    }[];
    return items.map((it) => ({
      ID: it.id,
      Name: it.title,
      Path: it.path,
      Format: "",
      Locale: "",
      Blocks: null,
      Metadata: null,
      LastChanged: "",
    }));
  }

  // --- PostHog locale-demand connector (web-only surface) ---
  async getPostHogConnector(): Promise<PostHogConnectorConfig> {
    return { configured: false };
  }
  async savePostHogConnector(): Promise<PostHogConnectorConfig> {
    throw new Error("PostHog connector is not available in the desktop app");
  }
  async deletePostHogConnector(): Promise<void> {
    throw new Error("PostHog connector is not available in the desktop app");
  }
  async getPostHogDemand(): Promise<PostHogDemandResponse> {
    throw new Error("PostHog connector is not available in the desktop app");
  }

  // --- Editor ---
  async getFileBlocks(
    _ws: string,
    projectId: string,
    fileName: string,
    _stream?: string,
    opts?: BlockQueryOptions,
  ): Promise<BlockInfo[]> {
    return Backend.QueryItemBlocks(projectId, fileName, {
      locale: opts?.locale ?? "",
      status: opts?.status ?? "",
      q: opts?.q ?? "",
      translatable: opts?.translatable ?? null,
      limit: opts?.limit ?? 0,
      offset: opts?.offset ?? 0,
    }) as Promise<BlockInfo[]>;
  }

  async getBlockCounts(
    _ws: string,
    projectId: string,
    item?: string,
    locale?: string,
    _stream?: string,
    opts?: { q?: string; translatable?: boolean },
  ): Promise<BlockCounts> {
    return Backend.GetBlockCounts(projectId, item ?? "", {
      locale: locale ?? "",
      status: "",
      q: opts?.q ?? "",
      translatable: opts?.translatable ?? null,
      limit: 0,
      offset: 0,
    }) as Promise<BlockCounts>;
  }

  async getBlock(_ws: string, projectId: string, blockId: string): Promise<BlockInfo> {
    return Backend.GetBlock(projectId, blockId) as Promise<BlockInfo>;
  }

  async getItem(_ws: string, projectId: string, itemName: string): Promise<ItemInfo> {
    return Backend.GetItem(projectId, itemName) as Promise<ItemInfo>;
  }

  async bulkReviewBlocks(
    _ws: string,
    req: BulkReviewBlocksRequest,
  ): Promise<BulkReviewBlocksResult> {
    return Backend.BulkReviewBlocks(req.project_id, {
      block_ids: req.block_ids,
      target_locale: req.target_locale,
      approve: req.approve,
      status: req.status ?? "",
      comment: req.comment ?? "",
      item_name: req.item_name ?? "",
    }) as Promise<BulkReviewBlocksResult>;
  }

  async bulkApplyMemory(_ws: string, req: BulkApplyMemoryRequest): Promise<BulkApplyMemoryResult> {
    return Backend.BulkApplyMemory(req.project_id, {
      block_ids: req.block_ids,
      target_locale: req.target_locale,
      threshold: req.threshold ?? 0,
    }) as Promise<BulkApplyMemoryResult>;
  }
  // The desktop queue is the whole project's: `collectionId` is not threaded
  // because the Wails backend has no collection-scoped entry point into review
  // (the governed review session is a web surface). A caller that passes one
  // would be answered a wider queue than it asked for, so it is refused rather
  // than quietly ignored.
  async getPendingReview(
    _ws: string,
    projectId: string,
    opts?: PendingReviewOptions,
  ): Promise<PendingReviewPage> {
    if (opts?.collectionId !== undefined) {
      throw new Error("the desktop review queue cannot be scoped to a collection");
    }
    return Backend.GetPendingReview(
      projectId,
      opts?.locales ?? [],
      opts?.limit ?? 0,
      opts?.offset ?? 0,
    ) as Promise<PendingReviewPage>;
  }
  async updateBlockTarget(_ws: string, req: UpdateBlockRequest): Promise<void> {
    return Backend.UpdateBlockTarget(req);
  }
  async updateBlockTargetCoded(_ws: string, req: UpdateBlockTargetCodedRequest): Promise<void> {
    // The @neokapi/ui editor still authors coded text + spans; the Wails
    // backend now consumes RFC 0001 runs, so convert at the boundary.
    return Backend.UpdateBlockTargetRuns({
      project_id: req.project_id,
      item_name: req.item_name,
      block_id: req.block_id,
      target_locale: req.target_locale,
      runs: codedToRuns(req.coded_text, req.spans),
    });
  }
  async pseudoTranslateFile(
    _ws: string,
    projectId: string,
    fileName: string,
    targetLocale: string,
  ): Promise<TranslationStats> {
    return Backend.PseudoTranslateItem(
      projectId,
      fileName,
      targetLocale,
    ) as Promise<TranslationStats>;
  }
  async aiTranslateFile(_ws: string, _req: AITranslateFileRequest): Promise<TranslationStats> {
    throw new Error("AI translation is managed by the server pipeline");
  }
  async memoryTranslateFile(
    _ws: string,
    projectId: string,
    fileName: string,
    targetLocale: string,
  ): Promise<TranslationStats> {
    return Backend.MemoryTranslateItem(
      projectId,
      fileName,
      targetLocale,
    ) as Promise<TranslationStats>;
  }
  async getWordCount(_ws: string, projectId: string, fileName: string): Promise<WordCountResult> {
    return Backend.GetWordCount(projectId, fileName) as Promise<WordCountResult>;
  }
  async exportTranslatedFile(
    _ws: string,
    projectId: string,
    fileName: string,
    targetLocale: string,
  ): Promise<Blob> {
    // Desktop: export to file system and open in OS
    const path = await Backend.ExportTranslatedItem(projectId, fileName, targetLocale);
    await Backend.OpenFileInOS(path);
    return new Blob(); // Dummy blob; actual file was saved to disk
  }
  async getTranslationDashboard(
    _ws: string,
    projectId: string,
    stream?: string,
    _opts?: TranslationDashboardItemOpts,
  ): Promise<TranslationDashboardStats> {
    // The dashboard is a server-side capability (HandleGetTranslationDashboard);
    // it is not bound into the desktop backend and the desktop UI does not call
    // this method. Resolve it by name so the bundler doesn't reject the missing
    // binding, and fail loudly if it is ever invoked without one.
    const fn = optionalBinding<
      (projectId: string, stream: string) => Promise<TranslationDashboardStats>
    >(Backend, "GetTranslationDashboard");
    if (!fn) {
      throw new Error("GetTranslationDashboard is not available in the desktop backend");
    }
    return fn(projectId, stream ?? "");
  }
  async lookupMemoryForBlock(
    _ws: string,
    projectId: string,
    itemName: string,
    blockId: string,
    targetLocale: string,
  ): Promise<MemoryMatchInfo[]> {
    return Backend.LookupMemoryForBlock(projectId, itemName, blockId, targetLocale) as Promise<
      MemoryMatchInfo[]
    >;
  }
  async lookupTermsForBlock(
    _ws: string,
    projectId: string,
    itemName: string,
    blockId: string,
    targetLocale: string,
  ): Promise<BlockTermMatch[]> {
    return Backend.LookupTermsForBlock(projectId, itemName, blockId, targetLocale) as Promise<
      BlockTermMatch[]
    >;
  }

  /**
   * The context one unit is decided in. The desktop backend binds the memory
   * and term lookups but neither the decision ledger nor the voice scores, so
   * those layers arrive empty and the review surfaces draw their empty states.
   * They are absent here rather than fetched from the server, because a desktop
   * project is the local one.
   */
  async getReviewContext(
    _ws: string,
    projectId: string,
    itemName: string,
    blockId: string,
    targetLocale: string,
  ): Promise<ReviewContext> {
    const [memory, terms] = await Promise.all([
      this.lookupMemoryForBlock(_ws, projectId, itemName, blockId, targetLocale),
      this.lookupTermsForBlock(_ws, projectId, itemName, blockId, targetLocale),
    ]);
    const best = memory[0];
    return {
      block_id: blockId,
      item_name: itemName,
      locale: targetLocale,
      collection_id: "",
      terms,
      notes: [],
      point: { path: itemName, language: targetLocale, default: false, terms_total: 0 },
      neighbourhood: { key: blockId, window: 0 },
      history: best
        ? {
            match: {
              score: Math.round(best.score * 100),
              kind: best.match_type,
              source: best.source,
              target: best.target,
            },
          }
        : {},
      judgement: {},
      provenance: {},
    };
  }

  // --- Content Memory ---
  async getMemoryEntries(
    _ws: string,
    query: string,
    sourceLocale: string,
    targetLocale: string,
    offset: number,
    limit: number,
  ): Promise<MemorySearchResult> {
    // Bowrain content memory API takes projectID as first arg; pass empty string for workspace-level
    const result = (await Backend.GetMemoryEntries(
      "",
      query,
      sourceLocale,
      targetLocale,
      offset,
      limit,
    )) as {
      entries?: Array<Record<string, unknown>>;
      total_count: number;
    };
    // The Wails backend names the locale fields source_locale/target_locale;
    // the shared ApiAdapter contract (REST shape) uses source_language/
    // target_language. Map so shared consumers (MemoryBrowser adapter) see the
    // contract shape.
    return {
      entries: (result.entries ?? []).map((e) => ({
        ...e,
        source_language: e.source_locale,
        target_language: e.target_locale,
      })),
      total_count: result.total_count,
    } as MemorySearchResult;
  }
  async getMemoryCount(): Promise<number> {
    return Backend.GetMemoryCount("") as Promise<number>;
  }
  async addMemoryEntry(
    _ws: string,
    source: string,
    target: string,
    sourceLocale: string,
    targetLocale: string,
  ): Promise<MemoryEntryInfo> {
    return Backend.AddMemoryEntry(
      "",
      source,
      target,
      sourceLocale,
      targetLocale,
    ) as Promise<MemoryEntryInfo>;
  }
  async updateMemoryEntry(_ws: string, req: MemoryUpdateRequest): Promise<void> {
    return Backend.UpdateMemoryEntry(req);
  }
  async deleteMemoryEntry(_ws: string, entryId: string): Promise<void> {
    return Backend.DeleteMemoryEntry("", entryId);
  }
  // The desktop deletes memory entries one at a time, so the batch reports each
  // id's own outcome rather than failing the whole selection.
  async bulkDeleteMemoryEntries(ws: string, entryIds: string[]): Promise<BulkDeleteResult> {
    const results: BulkDeleteResult["results"] = [];
    for (const id of entryIds) {
      try {
        await this.deleteMemoryEntry(ws, id);
        results.push({ id, deleted: true });
      } catch (err) {
        results.push({ id, deleted: false, error: (err as Error).message });
      }
    }
    const deleted = results.filter((r) => r.deleted).length;
    return { results, deleted, failed: results.length - deleted };
  }

  // --- Terminology ---
  async getTerms(
    _ws: string,
    query: string,
    sourceLocale: string,
    targetLocale: string,
    offset: number,
    limit: number,
  ): Promise<TermSearchResult> {
    return Backend.GetTerms(
      "",
      query,
      sourceLocale,
      targetLocale,
      offset,
      limit,
    ) as Promise<TermSearchResult>;
  }
  async getTermCount(): Promise<number> {
    return Backend.GetTermCount("") as Promise<number>;
  }
  async addConcept(_ws: string, req: AddConceptRequest): Promise<ConceptInfo> {
    return Backend.AddConcept(req) as Promise<ConceptInfo>;
  }
  async updateConcept(_ws: string, req: UpdateConceptRequest): Promise<void> {
    return Backend.UpdateConcept(req);
  }
  // Deleting a concept is a governed transition, refused the same way the
  // server refuses it — a multi-select learns it once, not once per row.
  //
  // The refusal is the server's 409 envelope, not a bare Error: the resource
  // browser reads {detail, hint} off it (governedRefusal) to say what was
  // refused and what to do instead, and a plain Error left the desktop showing
  // a vaguer message than the web for the same action.
  async bulkDeleteConcepts(_ws: string, _conceptIds: string[]): Promise<never> {
    throw governedRefusalError("deleting concepts");
  }
  async deleteConcept(_ws: string, conceptId: string): Promise<void> {
    return Backend.DeleteConcept("", conceptId);
  }
  async importTermsCSV(
    _ws: string,
    csvContent: string,
    sourceLocale: string,
    targetLocale: string,
    domain: string,
    hasHeader: boolean,
  ): Promise<number> {
    return Backend.ImportTermsCSV(
      "",
      csvContent,
      sourceLocale,
      targetLocale,
      domain,
      hasHeader,
    ) as Promise<number>;
  }
  async importTermsJSON(_ws: string, jsonContent: string): Promise<number> {
    return Backend.ImportTermsJSON("", jsonContent) as Promise<number>;
  }
  async exportTermsJSON(_ws: string, name: string): Promise<string> {
    return Backend.ExportTermsJSON("", name) as Promise<string>;
  }

  // --- Providers ---
  async listProviderConfigs(): Promise<ProviderConfig[]> {
    return Backend.ListProviderConfigs() as Promise<ProviderConfig[]>;
  }
  async saveProviderConfig(_ws: string, cfg: ProviderConfigWithKey): Promise<ProviderConfig> {
    return Backend.SaveProviderConfig(cfg) as Promise<ProviderConfig>;
  }
  async deleteProviderConfig(_ws: string, id: string): Promise<void> {
    return Backend.DeleteProviderConfig(id);
  }
  async testProviderConfig(_ws: string, cfg: ProviderConfigWithKey): Promise<void> {
    return Backend.TestProviderConfig(cfg);
  }

  // --- Utility ---
  async getKnownLocales(): Promise<LocaleInfo[]> {
    return Backend.GetKnownLocales() as Promise<LocaleInfo[]>;
  }
  async listFormats(): Promise<FormatInfo[]> {
    return Backend.ListFormats() as Promise<FormatInfo[]>;
  }
  async listTools(): Promise<ToolInfo[]> {
    return Backend.ListTools() as Promise<ToolInfo[]>;
  }
  async getToolSchema(name: string): Promise<ComponentSchema | null> {
    // The backend proxies the server's schema route; a tool without a schema
    // comes back as null.
    return (await Backend.GetToolSchema(name)) as ComponentSchema | null;
  }

  // --- Block history (desktop: not yet backed by Wails bindings) ---
  async getBlockHistory(
    _ws: string,
    _projectId: string,
    _blockId: string,
    _locale: string,
    _limit?: number,
  ): Promise<BlockHistoryEntry[]> {
    return [];
  }

  // --- Rollback / restore + governance (#778): server-only, not in desktop ---
  async rollbackBlock(): Promise<void> {}
  async revertBatch(): Promise<{ reverted: number }> {
    return { reverted: 0 };
  }
  async restoreToPoint(): Promise<{ restored: number }> {
    return { restored: 0 };
  }
  async setBlockStatus(): Promise<void> {}
  async reviewBlock(
    _ws: string,
    projectId: string,
    itemName: string,
    blockId: string,
    targetLocale: string,
    reviewed: boolean,
    _stream?: string,
    rung?: ReviewRung,
  ): Promise<void> {
    // Delegates to the Go backend, which calls the server's review endpoint
    // and queues the operation for replay when offline (backend/offlineop.go).
    // Desktop mode is stream-unaware by design: the entire desktop editor
    // surface is pinned to the "main" stream (editorRef in
    // bowrain/editorclient/editor.go), so the adapter accepts and ignores the
    // stream parameter — same as rollbackBlock/getBlockHistory above. Each
    // direction has its own rungs, so only "signed-off" rides on an approval
    // and only "draft" on a clearing call.
    const status = reviewed
      ? rung === "signed-off"
        ? "signed-off"
        : ""
      : rung === "draft"
        ? "draft"
        : "";
    return Backend.ReviewBlock(projectId, itemName, blockId, targetLocale, reviewed, status);
  }
  async approvePassingReview(): Promise<ApprovePassingResult> {
    // Bulk approve-passing is a server-side governance operation; the desktop
    // working copy signs off block-by-block via reviewBlock instead.
    throw new Error("bulk approve-passing is not available in the desktop app");
  }
  async recordVoiceCorrection(
    _ws: string,
    _projectId: string,
    _req: VoiceCorrectionRequest,
  ): Promise<VoiceCorrectionResult> {
    throw new Error("recording brand corrections is not available in the desktop app");
  }
  async listGroups(): Promise<Group[]> {
    return [];
  }
  async createGroup(_ws: string, name: string): Promise<Group> {
    return { id: "", workspace_id: "", name, description: "", created_at: "" };
  }
  async deleteGroup(): Promise<void> {}
  async listGroupMembers(): Promise<string[]> {
    return [];
  }
  async addGroupMember(): Promise<void> {}
  async removeGroupMember(): Promise<void> {}
  async listGroupBindings(): Promise<GroupRoleBinding[]> {
    return [];
  }
  async addGroupBinding(
    _ws: string,
    groupId: string,
    projectId: string,
    roleId: string,
  ): Promise<GroupRoleBinding> {
    return {
      id: "",
      group_id: groupId,
      workspace_id: "",
      project_id: projectId,
      role_id: roleId,
      languages: [],
      created_at: "",
    };
  }
  async removeGroupBinding(): Promise<void> {}
  async listDenyRules(): Promise<DenyRule[]> {
    return [];
  }
  async createDenyRule(_ws: string, rule: DenyRuleInput): Promise<DenyRule> {
    return {
      id: "",
      workspace_id: "",
      subject_type: rule.subject_type,
      subject_id: rule.subject_id,
      project_id: rule.project_id ?? "",
      denied_perms: 0,
      reason: rule.reason ?? "",
      created_at: "",
    };
  }
  async deleteDenyRule(): Promise<void> {}
  async getSoDMode(): Promise<{ mode: SoDMode }> {
    return { mode: "warn" };
  }
  async setSoDMode(): Promise<void> {}
  async listRoleOverrides(): Promise<Record<string, string[]>> {
    return {};
  }
  async setRoleOverride(): Promise<void> {}
  async demoteVoiceRule(): Promise<void> {}

  // --- Block notes (desktop: not yet backed by Wails bindings) ---
  async addBlockNote(
    _ws: string,
    _projectId: string,
    _blockId: string,
    _text: string,
  ): Promise<BlockNote> {
    throw new Error("Block notes not yet supported in desktop mode");
  }
  async listBlockNotes(_ws: string, _projectId: string, _blockId: string): Promise<BlockNote[]> {
    return [];
  }
  async deleteBlockNote(_ws: string, _projectId: string, _noteId: string): Promise<void> {
    throw new Error("Block notes not yet supported in desktop mode");
  }

  // --- Checks (desktop: not yet backed by Wails bindings) ---
  async runCheck(
    _ws: string,
    _projectId: string,
    _blockId: string,
    _locale: string,
  ): Promise<CheckIssue[]> {
    return [];
  }
  async runFileCheck(
    _ws: string,
    _projectId: string,
    _fileName: string,
    _locale: string,
  ): Promise<FileCheckResult[]> {
    return [];
  }

  // --- Preview ---
  async renderDocumentPreview(
    _ws: string,
    projectId: string,
    fileName: string,
    targetLocale: string,
  ): Promise<string> {
    return Backend.RenderDocumentPreview(projectId, fileName, targetLocale);
  }
  async renderBlockHTML(
    _ws: string,
    projectId: string,
    blockId: string,
    targetLocale: string,
  ): Promise<string> {
    return Backend.RenderBlockHTML(projectId, "", blockId, targetLocale);
  }

  // --- Automations (desktop: not yet backed by Wails bindings) ---
  async listAutomationRules(_ws: string, _projectId: string): Promise<AutomationRule[]> {
    return [];
  }
  async createAutomationRule(
    _ws: string,
    _projectId: string,
    _data: SaveAutomationRuleRequest,
  ): Promise<AutomationRule> {
    throw new Error("Automations not yet supported in desktop mode");
  }
  async updateAutomationRule(
    _ws: string,
    _projectId: string,
    _ruleId: string,
    _data: SaveAutomationRuleRequest,
  ): Promise<AutomationRule> {
    throw new Error("Automations not yet supported in desktop mode");
  }
  async deleteAutomationRule(_ws: string, _projectId: string, _ruleId: string): Promise<void> {
    throw new Error("Automations not yet supported in desktop mode");
  }
  async toggleAutomationRule(
    _ws: string,
    _projectId: string,
    _ruleId: string,
  ): Promise<AutomationRule> {
    throw new Error("Automations not yet supported in desktop mode");
  }
  async listAutomationEvents(_ws: string, _projectId: string): Promise<AutomationEvent[]> {
    return [];
  }
  async listAutomationHistory(_ws: string, _projectId: string): Promise<AutomationHistoryPage> {
    return { entries: [] };
  }

  // --- Automation Runs (Bowrain AD-013, not yet supported in desktop) ---
  async listAutomationRuns(
    _ws: string,
    _projectId: string,
    _status?: string,
    _limit?: number,
  ): Promise<AutomationRun[]> {
    return [];
  }
  async getAutomationRun(
    _ws: string,
    _projectId: string,
    _runId: string,
  ): Promise<{ run: AutomationRun; steps: AutomationStep[] }> {
    throw new Error("not implemented in desktop app");
  }
  async listStepLogs(
    _ws: string,
    _projectId: string,
    _runId: string,
    _stepId: string,
    _limit?: number,
  ): Promise<AutomationLogEntry[]> {
    return [];
  }
  async cancelAutomationRun(_ws: string, _projectId: string, _runId: string): Promise<void> {
    throw new Error("not implemented in desktop app");
  }

  // --- Flow definitions (Bowrain AD-013, #766) ---
  // The desktop's flow Wails methods are project-scoped and proxy to the
  // server's flow-definition REST API. The workspace is implicit (the active
  // workspace tracked by the Go backend), so the ws arg is unused here.
  async listFlowDefinitions(_ws: string, projectId: string): Promise<FlowDefinitionInfo[]> {
    return Backend.ListFlowDefinitions(projectId) as Promise<FlowDefinitionInfo[]>;
  }
  async getFlowDefinition(
    _ws: string,
    projectId: string,
    flowId: string,
  ): Promise<FlowDefinitionInfo> {
    return Backend.GetFlowDefinition(projectId, flowId) as Promise<FlowDefinitionInfo>;
  }
  async createFlowDefinition(
    _ws: string,
    projectId: string,
    def: FlowDefinitionInfo,
  ): Promise<FlowDefinitionInfo> {
    return Backend.SaveFlowDefinition(projectId, {
      ...def,
      id: "",
    }) as Promise<FlowDefinitionInfo>;
  }
  async updateFlowDefinition(
    _ws: string,
    projectId: string,
    flowId: string,
    def: FlowDefinitionInfo,
  ): Promise<FlowDefinitionInfo> {
    return Backend.SaveFlowDefinition(projectId, {
      ...def,
      id: flowId,
    }) as Promise<FlowDefinitionInfo>;
  }
  async deleteFlowDefinition(_ws: string, projectId: string, flowId: string): Promise<void> {
    return Backend.DeleteFlowDefinition(projectId, flowId);
  }

  // --- Notifications (desktop: not yet backed by Wails bindings) ---
  async listNotifications(
    _ws: string,
    _limit?: number,
    _unreadOnly?: boolean,
  ): Promise<{ notifications: NotificationInfo[]; unread_count: number }> {
    return { notifications: [], unread_count: 0 };
  }
  async markNotificationRead(_ws: string, _id: string): Promise<void> {}
  async markAllNotificationsRead(_ws: string): Promise<void> {}
  async deleteNotification(_ws: string, _id: string): Promise<void> {}

  // --- Digest settings (not yet supported in desktop) ---
  async getDigestSettings(_ws: string): Promise<DigestSettingsDTO> {
    throw new Error("not implemented in desktop app");
  }
  async updateDigestSettings(
    _ws: string,
    _settings: DigestSettingsDTO,
  ): Promise<DigestSettingsDTO> {
    throw new Error("not implemented in desktop app");
  }

  // --- Entity annotations (desktop: not yet backed by Wails bindings) ---
  async createEntity(
    _ws: string,
    _projectId: string,
    _itemName: string,
    _blockId: string,
    _entity: Partial<EntityInfo>,
  ): Promise<EntityInfo> {
    throw new Error("Entity annotations not yet supported in desktop mode");
  }
  async updateEntity(
    _ws: string,
    _projectId: string,
    _itemName: string,
    _blockId: string,
    _entityKey: string,
    _entity: Partial<EntityInfo>,
  ): Promise<EntityInfo> {
    throw new Error("Entity annotations not yet supported in desktop mode");
  }
  async deleteEntity(
    _ws: string,
    _projectId: string,
    _itemName: string,
    _blockId: string,
    _entityKey: string,
  ): Promise<void> {
    throw new Error("Entity annotations not yet supported in desktop mode");
  }
  async promoteEntity(
    _ws: string,
    _projectId: string,
    _itemName: string,
    _blockId: string,
    _entityKey: string,
  ): Promise<void> {
    throw new Error("Entity annotations not yet supported in desktop mode");
  }

  // --- Voice (proxied to the server's REST governance endpoints) ---
  async createProfileFromStarter(
    _ws: string,
    _pack: string,
    _name?: string,
  ): Promise<VoiceProfile> {
    throw new Error("Creating brand profiles is not available in the desktop app");
  }

  async listVoiceProfiles(workspaceSlug: string): Promise<VoiceProfile[]> {
    return Backend.ListVoiceProfiles(workspaceSlug) as Promise<VoiceProfile[]>;
  }
  async listContextProfiles(workspaceSlug: string): Promise<ContextProfilesResponse> {
    return Backend.ListContextProfiles(workspaceSlug) as Promise<ContextProfilesResponse>;
  }
  async listChannelProposals(
    workspaceSlug: string,
    status?: ChannelProposalStatus,
  ): Promise<ChannelAliasProposalsResponse> {
    return Backend.ListChannelAliasProposals(
      workspaceSlug,
      status ?? "",
    ) as Promise<ChannelAliasProposalsResponse>;
  }
  async judgeChannelProposal(
    workspaceSlug: string,
    judgement: ChannelAliasJudgement,
  ): Promise<ChannelAliasProposal> {
    return Backend.JudgeChannelAliasProposal(workspaceSlug, {
      profile: judgement.profile ?? "",
      proposed_channel: judgement.proposed_channel,
      existing_channel: judgement.existing_channel,
      status: judgement.status,
    }) as Promise<ChannelAliasProposal>;
  }
  async getVoiceProfile(workspaceSlug: string, profileId: string): Promise<VoiceProfile> {
    return Backend.GetVoiceProfile(workspaceSlug, profileId) as Promise<VoiceProfile>;
  }
  async createVoiceProfile(_ws: string, _data: CreateVoiceProfileRequest): Promise<VoiceProfile> {
    // Authoring profiles is a web/MCP workflow; the desktop governance surface
    // is review (promote/reject/evaluate), not profile creation.
    throw new Error("Creating brand profiles is not available in the desktop app");
  }
  async updateVoiceProfile(_ws: string, _data: UpdateVoiceProfileRequest): Promise<VoiceProfile> {
    throw new Error("Editing brand profiles is not available in the desktop app");
  }
  async deleteVoiceProfile(_ws: string, _profileId: string): Promise<void> {
    throw new Error("Deleting brand profiles is not available in the desktop app");
  }
  async getVoiceScores(workspaceSlug: string, projectId: string): Promise<StoredScore[]> {
    return Backend.GetVoiceScores(workspaceSlug, projectId) as Promise<StoredScore[]>;
  }
  async getVoiceTrends(workspaceSlug: string, projectId: string): Promise<ScoreTrend[]> {
    return Backend.GetVoiceTrends(workspaceSlug, projectId) as Promise<ScoreTrend[]>;
  }
  async getVoiceRollup(workspaceSlug: string, opts?: VoiceRollupOptions): Promise<VoiceRollup> {
    // The rollup is the server's aggregation: the effective profile comes from
    // a resolution ladder only the server can walk. Offline the backend answers
    // the empty rollup marked offline rather than a locally composed board.
    const out = (await Backend.GetVoiceRollup(workspaceSlug, {
      limit: opts?.limit ?? 0,
      offset: opts?.offset ?? 0,
      recentDays: opts?.recentDays ?? 0,
      minScore: opts?.minScore ?? 0,
      dropPoints: opts?.dropPoints ?? 0,
      days: 0,
    })) as VoiceRollup;
    return { projects: out.projects ?? [], total: out.total, limit: out.limit, offset: out.offset };
  }
  // --- Correction-learning loop (AD-019, proxied to the server's REST endpoints) ---
  async listVoiceCandidates(
    workspaceSlug: string,
    profileId: string,
    opts?: { minCount?: number; all?: boolean },
  ): Promise<CandidateRule[]> {
    return Backend.GetSuggestedRules(
      workspaceSlug,
      profileId,
      opts?.minCount ?? 0,
      opts?.all ?? false,
    ) as Promise<CandidateRule[]>;
  }
  async promoteVoiceRule(
    workspaceSlug: string,
    profileId: string,
    rule: { term: string; replacement?: string; correction_count?: number },
  ): Promise<{ promoted: boolean }> {
    return Backend.PromoteRule(workspaceSlug, profileId, {
      term: rule.term,
      replacement: rule.replacement ?? "",
      correction_count: rule.correction_count ?? 0,
    }) as Promise<{ promoted: boolean }>;
  }
  async rejectVoiceRule(
    workspaceSlug: string,
    profileId: string,
    rule: { term: string; replacement?: string },
  ): Promise<void> {
    return Backend.RejectRule(workspaceSlug, profileId, {
      term: rule.term,
      replacement: rule.replacement ?? "",
      correction_count: 0,
    });
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
    return Backend.EvaluateRule(workspaceSlug, profileId, {
      term: req.term,
      replacement: req.replacement ?? "",
      project_id: req.project_id,
      stream: req.stream ?? "",
    }) as Promise<BlastRadius>;
  }
  async getVoiceDrift(
    workspaceSlug: string,
    projectId: string,
    opts?: { recentDays?: number; minScore?: number; dropPoints?: number },
  ): Promise<DriftResult> {
    return Backend.GetVoiceDrift(
      workspaceSlug,
      projectId,
      opts?.recentDays ?? 0,
      opts?.minScore ?? 0,
      opts?.dropPoints ?? 0,
    ) as Promise<DriftResult>;
  }

  // --- Context scan (epic 016) — a server-side worker job; the desktop app
  // does not expose the hosted scan (the local kapi Agent Skill lane covers
  // local onboarding) ---
  async uploadContextScanSources(_ws: string, _files: File[]): Promise<ContextScanUploadResult> {
    throw new Error("not implemented in desktop app");
  }

  async startContextScan(_ws: string, _req: ContextScanRequest): Promise<{ job_id: string }> {
    throw new Error("not implemented in desktop app");
  }

  async getContextScan(_ws: string, _jobId: string): Promise<ContextScanJob> {
    throw new Error("not implemented in desktop app");
  }

  async approveContextScan(): Promise<ContextScanApproveResult> {
    throw new Error("not implemented in desktop app");
  }

  async checkVoiceDraft(
    _ws: string,
    _profile: VoiceProfile,
    _text: string,
  ): Promise<ContextScanCheckResult> {
    throw new Error("not implemented in desktop app");
  }

  // Approving an axis proposes a recipe change for a pull to write. The desktop
  // app edits kapi.yaml directly, so it has no use for a change queued on a
  // server — the local edit is the change.
  async approveAxis(): Promise<PendingRecipeChange> {
    throw new Error("not implemented in desktop app");
  }

  // --- Activities (Bowrain AD-014, not yet supported in desktop) ---
  async listActivities(
    _ws: string,
    _query?: {
      project_id?: string;
      stream?: string;
      actor_id?: string;
      type?: string;
      types?: string[];
      cursor?: string;
      limit?: number;
    },
  ): Promise<ActivityPage> {
    return { activities: [] };
  }

  async markActivitiesSeen(_ws: string): Promise<void> {}

  // --- Convergence runs (AD-022; the desktop's Runs surface arrives with the
  // web-frontend consolidation — list reads as empty until then) ---
  async listConvergenceRuns(
    _ws: string,
    _projectId: string,
    _limit?: number,
  ): Promise<ConvergenceRun[]> {
    return [];
  }
  async getConvergenceRun(
    _ws: string,
    _projectId: string,
    _runId: string,
  ): Promise<ConvergenceRun> {
    throw new Error("Convergence runs are not yet surfaced in the desktop app");
  }
  async startConvergenceRun(
    _ws: string,
    _projectId: string,
    _opts?: {
      trigger?: string;
      locales?: string[];
      scope?: ConvergenceRunScope;
      confirmed?: boolean;
    },
  ): Promise<ConvergenceRun | null> {
    throw new Error("Convergence runs are not yet surfaced in the desktop app");
  }
  async cancelConvergenceRun(_ws: string, _projectId: string, _runId: string): Promise<void> {
    throw new Error("Convergence runs are not yet surfaced in the desktop app");
  }
  async estimateConvergence(_ws: string, _projectId: string): Promise<ConvergenceEstimate> {
    throw new Error("Convergence runs are not yet surfaced in the desktop app");
  }
  async getLoopRollup(_ws: string): Promise<LoopRollup> {
    // No convergence-run surface in the desktop app yet: an empty rollup
    // hides the workspace home's run/ship cards.
    return {};
  }

  // --- Tasks (Bowrain AD-014, not yet supported in desktop) ---
  async listTasks(_ws: string, _query?: TaskQuery): Promise<TaskPage> {
    return { tasks: [] };
  }
  async getTaskCounts(_ws: string, _query?: TaskQuery): Promise<TaskCounts> {
    return { by_status: { open: 0, in_progress: 0, completed: 0, cancelled: 0 }, total: 0 };
  }
  async createTask(_ws: string, _task: CreateTaskRequest): Promise<TaskInfo> {
    throw new Error("Tasks not yet supported in desktop mode");
  }
  async getTask(_ws: string, _taskId: string): Promise<TaskInfo> {
    throw new Error("Tasks not yet supported in desktop mode");
  }
  async updateTask(
    _ws: string,
    _taskId: string,
    _updates: Partial<CreateTaskRequest>,
  ): Promise<TaskInfo> {
    throw new Error("Tasks not yet supported in desktop mode");
  }
  async deleteTask(_ws: string, _taskId: string): Promise<void> {
    throw new Error("Tasks not yet supported in desktop mode");
  }
  async assignTask(_ws: string, _taskId: string, _assigneeId: string): Promise<void> {
    throw new Error("Tasks not yet supported in desktop mode");
  }
  async completeTask(_ws: string, _taskId: string): Promise<void> {
    throw new Error("Tasks not yet supported in desktop mode");
  }
  async cancelTask(_ws: string, _taskId: string): Promise<void> {
    throw new Error("Tasks not yet supported in desktop mode");
  }
  async listMyTasks(
    _ws: string,
    _query?: { status?: string; cursor?: string; limit?: number },
  ): Promise<TaskPage> {
    return { tasks: [] };
  }

  // --- Notification preferences (Bowrain AD-014, not yet supported in desktop) ---
  async getNotificationPreferences(
    _ws: string,
  ): Promise<{ preferences: NotificationPreference[] }> {
    return { preferences: [] };
  }
  async updateNotificationPreferences(
    _ws: string,
    _preferences: NotificationPreference[],
  ): Promise<void> {
    throw new Error("Notification preferences not yet supported in desktop mode");
  }

  // --- @bravo Agent (not yet supported in desktop) ---
  async bravoCreateConversation(
    _ws: string,
    _projectId?: string,
    _title?: string,
  ): Promise<BravoConversation> {
    throw new Error("not implemented in desktop app");
  }
  async bravoListConversations(
    _ws: string,
    _limit?: number,
    _offset?: number,
  ): Promise<{ conversations: BravoConversation[]; total: number }> {
    return { conversations: [], total: 0 };
  }
  async bravoGetConversation(
    _ws: string,
    _conversationId: string,
  ): Promise<{ conversation: BravoConversation; messages: BravoMessage[] }> {
    throw new Error("not implemented in desktop app");
  }
  async bravoDeleteConversation(_ws: string, _conversationId: string): Promise<void> {
    throw new Error("not implemented in desktop app");
  }
  async bravoSendMessage(
    _ws: string,
    _conversationId: string,
    _content: string,
  ): Promise<{ user_message: BravoMessage; assistant_message: BravoMessage }> {
    throw new Error("not implemented in desktop app");
  }
  async bravoListMessages(
    _ws: string,
    _conversationId: string,
    _limit?: number,
    _offset?: number,
  ): Promise<{ messages: BravoMessage[] }> {
    return { messages: [] };
  }
  async bravoApproveToolCall(
    _ws: string,
    _conversationId: string,
    _toolCallId: string,
  ): Promise<void> {
    throw new Error("not implemented in desktop app");
  }
  async bravoDenyToolCall(
    _ws: string,
    _conversationId: string,
    _toolCallId: string,
  ): Promise<void> {
    throw new Error("not implemented in desktop app");
  }
  async bravoCancelConversation(_ws: string, _conversationId: string): Promise<void> {
    throw new Error("not implemented in desktop app");
  }
  async bravoGetConfig(_ws: string): Promise<BravoConfig> {
    throw new Error("not implemented in desktop app");
  }
  async bravoUpdateConfig(_ws: string, _config: Partial<BravoConfig>): Promise<BravoConfig> {
    throw new Error("not implemented in desktop app");
  }
  async bravoListTools(_ws: string): Promise<{ tools: BravoToolInfo[] }> {
    return { tools: [] };
  }
  async bravoGetUsage(_ws: string, _from?: string, _to?: string): Promise<BravoUsageSummary> {
    throw new Error("not implemented in desktop app");
  }
  async bravoUpdateMode(
    _ws: string,
    _conversationId: string,
    _mode: string,
  ): Promise<{ mode: string; permissions: string[] }> {
    throw new Error("not implemented in desktop app");
  }
  bravoSendMessageSSE(
    _ws: string,
    _conversationId: string,
    _content: string,
    _handler: BravoSSEHandler,
    _mode?: string,
    _context?: { projectId?: string; stream?: string; itemId?: string },
  ): AbortController {
    throw new Error("not implemented in desktop app");
  }

  // --- Billing (not applicable in desktop) ---
  async billingGetOverview(_ws: string): Promise<BillingOverview> {
    throw new Error("not implemented in desktop app");
  }
  async billingGetUsage(_ws: string): Promise<BillingUsageBreakdown> {
    throw new Error("not implemented in desktop app");
  }
  async billingGetModelUsage(
    _ws: string,
    _from?: string,
    _to?: string,
  ): Promise<ModelUsageResponse> {
    throw new Error("not implemented in desktop app");
  }
  async billingGetPlans(_ws: string): Promise<BillingPlansResponse> {
    throw new Error("not implemented in desktop app");
  }
  async billingCreateCheckout(
    _ws: string,
    _plan: BillingPlan,
    _successUrl: string,
    _cancelUrl: string,
    _seats?: number,
  ): Promise<{ url: string }> {
    throw new Error("not implemented in desktop app");
  }
  async billingBuyCredits(
    _ws: string,
    _successUrl: string,
    _cancelUrl: string,
  ): Promise<{ url: string }> {
    throw new Error("not implemented in desktop app");
  }
  async billingCreatePortal(_ws: string, _returnUrl: string): Promise<{ url: string }> {
    throw new Error("not implemented in desktop app");
  }
  async billingGetLedger(
    _ws: string,
    _from?: string,
    _to?: string,
    _opts?: { limit?: number; offset?: number; operation?: string },
  ): Promise<CreditLedgerEntry[]> {
    return [];
  }
  async billingGetLedgerPage(_ws: string, query?: CreditLedgerQuery): Promise<CreditLedgerPage> {
    return {
      entries: [],
      total: 0,
      limit: query?.limit ?? 0,
      offset: query?.offset ?? 0,
      usage_by_operation: {},
      net_by_operation: {},
      from: query?.from ?? "",
      to: query?.to ?? "",
    };
  }

  // --- Brand knowledge graph (AD-021) ---
  // The desktop app is a working copy of the server (AD-021): these proxy the
  // workspace REST surface through the Go backend's knowledge.go (Bearer
  // keychain auth, govRequest), exactly like the Voice methods above. The
  // graph is never authored offline. The backend returns json.RawMessage, which
  // the bindings surface untyped, so cast at the boundary to the typed shapes
  // the ApiAdapter promises. Ordinary concept terminology edits keep using the
  // gRPC editor methods (addConcept/updateConcept/deleteConcept) above.
  async listConcepts(
    workspaceSlug: string,
    params?: ListConceptsParams,
  ): Promise<TermSearchResult> {
    return Backend.ListConcepts(workspaceSlug, params ?? {}) as Promise<TermSearchResult>;
  }
  async getConceptStatusCounts(workspaceSlug: string): Promise<ConceptStatusCounts> {
    return Backend.ConceptStatusCounts(workspaceSlug) as Promise<ConceptStatusCounts>;
  }
  async getConceptLocaleCoverage(workspaceSlug: string): Promise<LocaleCoverageReport> {
    return Backend.ConceptLocaleCoverage(workspaceSlug) as Promise<LocaleCoverageReport>;
  }
  async getConcept(workspaceSlug: string, conceptId: string): Promise<ConceptInfo> {
    return Backend.GetConcept(workspaceSlug, conceptId) as Promise<ConceptInfo>;
  }
  async createConcept(workspaceSlug: string, req: AddConceptRequest): Promise<ConceptInfo> {
    return Backend.CreateConcept(workspaceSlug, req) as Promise<ConceptInfo>;
  }
  async getConceptStory(workspaceSlug: string, conceptId: string): Promise<ConceptStory> {
    return Backend.GetConceptStory(workspaceSlug, conceptId) as Promise<ConceptStory>;
  }
  async listConceptRelations(
    workspaceSlug: string,
    conceptId: string,
    scope?: RelationScope,
  ): Promise<ConceptRelation[]> {
    return Backend.ListConceptRelations(
      workspaceSlug,
      conceptId,
      scope?.as_of ?? "",
      scope?.market ?? "",
    ) as Promise<ConceptRelation[]>;
  }
  async addConceptRelation(
    workspaceSlug: string,
    conceptId: string,
    req: AddConceptRelationRequest,
  ): Promise<ConceptRelation> {
    return Backend.AddConceptRelation(workspaceSlug, conceptId, req) as Promise<ConceptRelation>;
  }
  async deleteConceptRelation(
    workspaceSlug: string,
    conceptId: string,
    relationId: string,
  ): Promise<void> {
    return Backend.DeleteConceptRelation(workspaceSlug, conceptId, relationId);
  }
  async getConceptBlastRadius(workspaceSlug: string, conceptId: string): Promise<ConceptUsage> {
    return Backend.GetConceptBlastRadius(workspaceSlug, conceptId) as Promise<ConceptUsage>;
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
    return Backend.GetConceptProjects(
      workspaceSlug,
      conceptId,
      params.toString(),
    ) as Promise<ConceptProjects>;
  }
  async listObservations(workspaceSlug: string, conceptId: string): Promise<Observation[]> {
    return Backend.ListObservations(workspaceSlug, conceptId) as Promise<Observation[]>;
  }
  async addObservation(
    workspaceSlug: string,
    conceptId: string,
    req: AddObservationRequest,
  ): Promise<Observation> {
    return Backend.AddObservation(workspaceSlug, conceptId, req) as Promise<Observation>;
  }
  async deleteObservation(
    workspaceSlug: string,
    conceptId: string,
    observationId: string,
  ): Promise<void> {
    return Backend.DeleteObservation(workspaceSlug, conceptId, observationId);
  }
  async listConceptComments(workspaceSlug: string, conceptId: string): Promise<Comment[]> {
    return Backend.ListConceptComments(workspaceSlug, conceptId) as Promise<Comment[]>;
  }
  async addConceptComment(
    workspaceSlug: string,
    conceptId: string,
    req: AddCommentRequest,
  ): Promise<Comment> {
    return Backend.AddConceptComment(workspaceSlug, conceptId, req) as Promise<Comment>;
  }
  async resolveConceptComment(
    workspaceSlug: string,
    conceptId: string,
    commentId: string,
    resolved?: boolean,
  ): Promise<void> {
    // The Go proxy always sends an explicit bool; the resolve action defaults to
    // marking resolved when the caller doesn't pass an explicit toggle value.
    return Backend.ResolveConceptComment(workspaceSlug, conceptId, commentId, resolved ?? true);
  }
  async deleteConceptComment(
    workspaceSlug: string,
    conceptId: string,
    commentId: string,
  ): Promise<void> {
    return Backend.DeleteConceptComment(workspaceSlug, conceptId, commentId);
  }
  async listMarkets(workspaceSlug: string): Promise<Market[]> {
    return Backend.ListMarkets(workspaceSlug) as Promise<Market[]>;
  }
  async createMarket(workspaceSlug: string, req: MarketRequest): Promise<Market> {
    return Backend.CreateMarket(workspaceSlug, req) as Promise<Market>;
  }
  async updateMarket(workspaceSlug: string, marketId: string, req: MarketRequest): Promise<Market> {
    return Backend.UpdateMarket(workspaceSlug, marketId, req) as Promise<Market>;
  }
  async deleteMarket(workspaceSlug: string, marketId: string): Promise<void> {
    return Backend.DeleteMarket(workspaceSlug, marketId);
  }
  async listChangesets(workspaceSlug: string, status?: ChangeSetStatus): Promise<ChangeSet[]> {
    return Backend.ListChangesets(workspaceSlug, status ?? "") as Promise<ChangeSet[]>;
  }
  async getChangesetCounts(workspaceSlug: string): Promise<ChangeSetCounts> {
    return Backend.ChangesetCounts(workspaceSlug) as Promise<ChangeSetCounts>;
  }
  async getChangeset(workspaceSlug: string, changesetId: string): Promise<ChangeSetDetail> {
    return Backend.GetChangeset(workspaceSlug, changesetId) as Promise<ChangeSetDetail>;
  }
  async createChangeset(workspaceSlug: string, req: CreateChangeSetRequest): Promise<ChangeSet> {
    return Backend.CreateChangeset(workspaceSlug, req) as Promise<ChangeSet>;
  }
  async patchChangeset(
    workspaceSlug: string,
    changesetId: string,
    req: UpdateChangeSetRequest,
  ): Promise<ChangeSet> {
    return Backend.PatchChangeset(workspaceSlug, changesetId, req) as Promise<ChangeSet>;
  }
  async appendChangesetOp(
    workspaceSlug: string,
    changesetId: string,
    req: AddChangeSetOpRequest,
  ): Promise<ChangeSetOp> {
    return Backend.AppendChangesetOp(workspaceSlug, changesetId, req) as Promise<ChangeSetOp>;
  }
  async removeChangesetOp(workspaceSlug: string, changesetId: string, seq: number): Promise<void> {
    return Backend.RemoveChangesetOp(workspaceSlug, changesetId, seq);
  }
  async submitChangeset(workspaceSlug: string, changesetId: string): Promise<ChangeSet> {
    return Backend.SubmitChangeset(workspaceSlug, changesetId) as Promise<ChangeSet>;
  }
  async approveChangeset(
    workspaceSlug: string,
    changesetId: string,
    req?: ReviewRequest,
  ): Promise<ChangeSet> {
    return Backend.ApproveChangeset(workspaceSlug, changesetId, req ?? {}) as Promise<ChangeSet>;
  }
  async rejectChangeset(
    workspaceSlug: string,
    changesetId: string,
    req?: ReviewRequest,
  ): Promise<ChangeSet> {
    return Backend.RejectChangeset(workspaceSlug, changesetId, req ?? {}) as Promise<ChangeSet>;
  }
  async mergeChangeset(workspaceSlug: string, changesetId: string): Promise<MergeResult> {
    return Backend.MergeChangeset(workspaceSlug, changesetId) as Promise<MergeResult>;
  }
  async abandonChangeset(workspaceSlug: string, changesetId: string): Promise<ChangeSet> {
    return Backend.AbandonChangeset(workspaceSlug, changesetId) as Promise<ChangeSet>;
  }
  async getChangesetBlastRadius(
    workspaceSlug: string,
    changesetId: string,
  ): Promise<ChangeSetImpact> {
    return Backend.GetChangesetBlastRadius(workspaceSlug, changesetId) as Promise<ChangeSetImpact>;
  }
  async refreshChangesetBlastRadius(
    workspaceSlug: string,
    changesetId: string,
  ): Promise<ChangeSetImpact> {
    return Backend.RefreshChangesetBlastRadius(
      workspaceSlug,
      changesetId,
    ) as Promise<ChangeSetImpact>;
  }
  async addPilot(
    workspaceSlug: string,
    changesetId: string,
    req: StartPilotRequest,
  ): Promise<Pilot> {
    return Backend.AddPilot(workspaceSlug, changesetId, req) as Promise<Pilot>;
  }
  async removePilot(
    workspaceSlug: string,
    changesetId: string,
    projectId: string,
    stream: string,
  ): Promise<void> {
    return Backend.RemovePilot(workspaceSlug, changesetId, projectId, stream);
  }
  async trialFindings(
    workspaceSlug: string,
    changesetId: string,
    projectId: string,
    stream: string,
  ): Promise<TrialReport> {
    return Backend.TrialFindings(
      workspaceSlug,
      changesetId,
      projectId,
      stream,
    ) as Promise<TrialReport>;
  }

  // --- Desktop-specific helpers (not in ApiAdapter) ---
  async openFileInOS(path: string): Promise<void> {
    return Backend.OpenFileInOS(path);
  }

  /**
   * Returns the presence-collaboration session (server URL, keychain auth
   * token, workspace, current user) so the frontend can open the Yjs awareness
   * WebSocket exactly like the web translate view. Rejects when not connected
   * to a server — presence is a server feature.
   */
  async getCollabSession(): Promise<CollabSessionInfo> {
    return Backend.GetCollabSession() as Promise<CollabSessionInfo>;
  }

  // Back-to-source review (RV-F) — the source-proposal and entity-promotion
  // flow — is a governed web/server surface. The desktop working copy does not
  // host it, so these reject like the other server-only features above.
  async createSourceProposal(): Promise<never> {
    throw new Error("source proposals are not available in the desktop app");
  }

  async listSourceProposals(): Promise<never> {
    throw new Error("source proposals are not available in the desktop app");
  }

  async decideSourceProposal(): Promise<never> {
    throw new Error("source proposals are not available in the desktop app");
  }

  async promoteEntityToConcept(): Promise<never> {
    throw new Error("entity-to-concept promotion is not available in the desktop app");
  }
}
