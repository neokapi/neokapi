import type {
  User,
  Workspace,
  Membership,
  ProjectInfo,
  CallerPermissions,
  ProjectReadOptions,
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
  CheckIssue,
  FileCheckResult,
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
  BillingOverview,
  BillingPlan,
  BillingPlansResponse,
  BillingUsageBreakdown,
  CreditLedgerEntry,
  ModelUsageResponse,
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
  UploadFilesResult,
  ReviewRung,
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
 * ApiAdapter abstracts the communication layer so that the same
 * UI components work in both the Wails desktop app and the REST web app.
 */
export interface ApiAdapter {
  // Server config
  getConfig(): Promise<ConfigResponse>;
  /** Public platform config (signups gate, maintenance, model-choice set). */
  getPublicPlatformConfig(): Promise<PublicPlatformConfig>;

  // Auth
  getCurrentUser(): Promise<User | null>;
  /**
   * Attempt a session refresh through the normal auth-refresh path, without
   * carrying a request. Resolves true when the session is (again) valid.
   * Consumers that cannot see HTTP status codes — the SSE EventSource, which
   * silently reconnect-loops on 401 after session expiry — call this before
   * reconnecting. Optional: only cookie/token adapters (web REST) implement
   * it; the desktop's keychain-Bearer transport has no cookie session to
   * refresh, and callers must treat an absent member as "nothing to refresh".
   */
  refreshSession?(): Promise<boolean>;

  // Account management — onboarding (handle pick), email change, slug check.
  getOnboardingStatus(): Promise<OnboardingStatus>;
  completeOnboarding(slug: string, displayName?: string): Promise<Workspace>;
  checkSlug(slug: string): Promise<SlugCheckResponse>;
  requestEmailChange(newEmail: string): Promise<EmailChangeRequestResponse>;
  confirmEmailChange(token: string): Promise<EmailChangeConfirmResponse>;

  // Account security — self-service passkey (WebAuthn) management. The server
  // relays the ceremony (Cognito) or steers to an account console (Keycloak);
  // no identity-provider token ever reaches the browser.
  getAccountSecurity(): Promise<AccountSecurity>;
  listPasskeys(): Promise<PasskeyListResponse>;
  passkeyRegisterStart(): Promise<PasskeyRegisterStartResponse>;
  passkeyRegisterFinish(req: PasskeyRegisterFinishRequest): Promise<void>;
  deletePasskey(id: string): Promise<void>;

  // Admin: workspace slug-rename reservations.
  adminListSlugReservations(): Promise<SlugReservation[]>;
  adminReleaseSlugReservation(slug: string): Promise<void>;

  // Workspaces
  listWorkspaces(): Promise<Workspace[]>;
  createWorkspace(name: string, slug: string): Promise<Workspace>;
  getWorkspace(slug: string): Promise<Workspace>;
  updateWorkspace(slug: string, data: Partial<Workspace>): Promise<Workspace>;
  deleteWorkspace(slug: string): Promise<void>;

  // Members
  listMembers(workspaceSlug: string): Promise<Membership[]>;
  addMember(workspaceSlug: string, userId: string, role: string): Promise<void>;
  updateMemberRole(workspaceSlug: string, userId: string, role: string): Promise<void>;
  removeMember(workspaceSlug: string, userId: string): Promise<void>;

  // Invites
  listInvites(workspaceSlug: string): Promise<Invite[]>;
  createInvite(
    workspaceSlug: string,
    email: string,
    role: string,
    maxUses: number,
  ): Promise<Invite>;
  deleteInvite(workspaceSlug: string, inviteId: string): Promise<void>;
  acceptInvite(code: string): Promise<AcceptInviteResponse>;

  // Role Templates
  listRoleTemplates(workspaceSlug: string): Promise<RoleTemplate[]>;
  createRoleTemplate(
    workspaceSlug: string,
    data: {
      name: string;
      display_name: string;
      description: string;
      permissions: string[];
      position?: number;
    },
  ): Promise<RoleTemplate>;
  updateRoleTemplate(
    workspaceSlug: string,
    roleId: string,
    data: {
      name?: string;
      display_name?: string;
      description?: string;
      permissions?: string[];
      position?: number;
    },
  ): Promise<RoleTemplate>;
  deleteRoleTemplate(workspaceSlug: string, roleId: string): Promise<void>;

  // Project Members
  listProjectMembers(workspaceSlug: string, projectId: string): Promise<ProjectMembership[]>;
  addProjectMember(
    workspaceSlug: string,
    projectId: string,
    data: {
      user_id: string;
      role_id: string;
      languages?: string[];
      /** The region of the context space this member governs. Absent = all of it. */
      coordinates?: Record<string, string>;
    },
  ): Promise<ProjectMembership>;
  updateProjectMember(
    workspaceSlug: string,
    projectId: string,
    userId: string,
    data: {
      role_id: string;
      languages?: string[];
      coordinates?: Record<string, string>;
    },
  ): Promise<ProjectMembership>;
  removeProjectMember(workspaceSlug: string, projectId: string, userId: string): Promise<void>;

  // API Tokens
  listApiTokens(workspaceSlug: string): Promise<ApiToken[]>;
  createApiToken(
    workspaceSlug: string,
    name: string,
    expireDays: number,
    scopes?: string[],
    /**
     * Names the machine the token is for (a CI runner, an agent-driven kapi).
     * Work created under it is authored by "agent/<agentName>" rather than by
     * the person who minted it, which is what leaves them free to review it.
     */
    agentName?: string,
  ): Promise<CreateApiTokenResponse>;
  deleteApiToken(workspaceSlug: string, tokenId: string): Promise<void>;

  // Claim
  claimProject(claimToken: string): Promise<ClaimProjectResponse>;

  // Streams
  listStreams(workspaceSlug: string, projectId: string): Promise<StreamInfo[]>;
  createStream(
    workspaceSlug: string,
    projectId: string,
    req: CreateStreamRequest,
  ): Promise<StreamInfo>;
  getStream(workspaceSlug: string, projectId: string, streamName: string): Promise<StreamInfo>;
  updateStream(
    workspaceSlug: string,
    projectId: string,
    streamName: string,
    data: { description?: string; visibility?: string; properties?: Record<string, string> },
  ): Promise<StreamInfo>;
  deleteStream(workspaceSlug: string, projectId: string, streamName: string): Promise<void>;
  diffStream(
    workspaceSlug: string,
    projectId: string,
    streamName: string,
  ): Promise<StreamDiffResult>;
  mergeStream(
    workspaceSlug: string,
    projectId: string,
    streamName: string,
    dryRun?: boolean,
  ): Promise<StreamMergeResult>;
  lockStream(workspaceSlug: string, projectId: string, streamName: string): Promise<StreamInfo>;
  unlockStream(workspaceSlug: string, projectId: string, streamName: string): Promise<StreamInfo>;

  // Stream tags
  listStreamTags(
    workspaceSlug: string,
    projectId: string,
    streamName: string,
  ): Promise<StreamTag[]>;
  createStreamTag(
    workspaceSlug: string,
    projectId: string,
    streamName: string,
    req: CreateStreamTagRequest,
  ): Promise<StreamTag>;
  getStreamTag(
    workspaceSlug: string,
    projectId: string,
    streamName: string,
    tagName: string,
  ): Promise<StreamTag>;
  deleteStreamTag(
    workspaceSlug: string,
    projectId: string,
    streamName: string,
    tagName: string,
  ): Promise<void>;
  listProjectTags(
    workspaceSlug: string,
    projectId: string,
    kind?: StreamTagKind,
  ): Promise<StreamTag[]>;

  // Projects (workspace-scoped)
  listProjects(workspaceSlug: string): Promise<ProjectInfo[]>;
  createProject(
    workspaceSlug: string,
    name: string,
    defaultSourceLanguage: string,
    targetLanguages: string[],
  ): Promise<ProjectInfo>;
  getProject(
    workspaceSlug: string,
    projectId: string,
    stream?: string,
    opts?: ProjectReadOptions,
  ): Promise<ProjectInfo>;
  /**
   * What the caller may do on this project, as the server resolved it. A
   * surface offers only the actions the server will accept, so a translator
   * sees no Approve button rather than a 403 after clicking it.
   */
  getCallerPermissions(workspaceSlug: string, projectId: string): Promise<CallerPermissions>;
  updateProject(
    workspaceSlug: string,
    projectId: string,
    data: {
      name?: string;
      target_languages?: string[];
      dashboard_visibility?: string;
      properties?: Record<string, string>;
    },
  ): Promise<ProjectInfo>;
  deleteProject(workspaceSlug: string, projectId: string): Promise<void>;
  restoreProject(workspaceSlug: string, projectId: string): Promise<void>;
  permanentlyDeleteProject(workspaceSlug: string, projectId: string): Promise<void>;
  listArchivedProjects(workspaceSlug: string): Promise<ArchivedProject[]>;
  restoreStream(workspaceSlug: string, projectId: string, streamName: string): Promise<void>;
  uploadFiles(
    workspaceSlug: string,
    projectId: string,
    files: File[],
    stream?: string,
  ): Promise<UploadFilesResult>;
  removeFile(
    workspaceSlug: string,
    projectId: string,
    fileName: string,
    stream?: string,
  ): Promise<ProjectInfo>;

  // Measured steerability (model recommendation sweeps, project-scoped).
  // The GET carries the instance-wide model_sweeps.enabled gate so the
  // settings panel can disable Refresh with a reason; the refresh POST
  // enqueues one sweep per target locale (manage-brand permission, flag on).
  getModelRecommendations(
    workspaceSlug: string,
    projectId: string,
  ): Promise<ModelRecommendationsResponse>;
  refreshModelRecommendations(
    workspaceSlug: string,
    projectId: string,
  ): Promise<{ enqueued: number; locales: string[] }>;

  // Collections (project-scoped)
  listCollections(
    workspaceSlug: string,
    projectId: string,
    stream?: string,
  ): Promise<CollectionInfo[]>;
  createCollection(
    workspaceSlug: string,
    projectId: string,
    req: CreateCollectionRequest,
  ): Promise<CollectionInfo>;
  getCollection(
    workspaceSlug: string,
    projectId: string,
    collectionId: string,
  ): Promise<CollectionInfo>;
  updateCollection(
    workspaceSlug: string,
    projectId: string,
    collectionId: string,
    req: Partial<CreateCollectionRequest>,
  ): Promise<CollectionInfo>;
  deleteCollection(workspaceSlug: string, projectId: string, collectionId: string): Promise<void>;
  uploadToCollection(
    workspaceSlug: string,
    projectId: string,
    collectionId: string,
    files: File[],
    stream?: string,
  ): Promise<UploadFilesResult>;

  // Integration connectors (Bowrain AD-011). Workspace-scoped CMS/design/
  // marketing integrations (WordPress, Figma, HubSpot); a connector is added
  // once per workspace, and fetch/publish bind it to a specific project.
  // Config secrets are write-only — the list never echoes them back.
  listConnectors(workspaceSlug: string): Promise<ConnectorInfo[]>;
  addConnector(
    workspaceSlug: string,
    type: string,
    config: Record<string, string>,
  ): Promise<ConnectorInfo>;
  removeConnector(workspaceSlug: string, connectorId: string): Promise<void>;
  // GitHub App post-install setup: the repositories an installation covers
  // (annotated with existing bindings), and binding one to a project — which
  // creates an app-auth forge connector server-side.
  /**
   * Mint the signed state that ties an installation back to this workspace.
   * It goes to GitHub as the install URL's `state`; GitHub echoes it on the
   * setup redirect, and {@link claimInstallation} redeems it. Without it a
   * returning request carries only an installation id GitHub chose, which is
   * evidence of nothing.
   */
  githubSetupState(workspaceSlug: string): Promise<GitHubSetupState>;
  /**
   * Record that an installation belongs to this workspace, on the strength of
   * the signed state. Every other installation endpoint requires this to have
   * happened; an installation another workspace already holds answers 404,
   * exactly as an unknown one does.
   */
  claimInstallation(
    workspaceSlug: string,
    installationId: string,
    state: string,
  ): Promise<ClaimInstallationResult>;
  listInstallationRepos(workspaceSlug: string, installationId: string): Promise<InstallationRepo[]>;
  /**
   * Inspect one repository of an installation before binding: reads the repo
   * tree server-side (no clone) and reports monorepo markers, i18n/content
   * signals, a proposed `patterns` value, and the files it matches.
   */
  detectInstallationRepo(
    workspaceSlug: string,
    installationId: string,
    repository: string,
    opts?: RepoDetectOptions,
  ): Promise<RepoDetection>;
  bindInstallationRepo(
    workspaceSlug: string,
    installationId: string,
    req: BindInstallationRepoRequest,
  ): Promise<BindInstallationRepoResult>;
  /**
   * Read a connector's sync state. The default is the cheap stored read
   * (last sync + last recorded ingest error) — safe to poll. Pass
   * `{ probe: true }` for the deep live probe (item counts, pending
   * pull/push, remote reachability); for git/forge connectors that re-runs a
   * clone, so reserve it for explicit "Test"/manual surfaces, never polls.
   */
  getConnectorStatus(
    workspaceSlug: string,
    connectorId: string,
    opts?: { probe?: boolean },
  ): Promise<ConnectorSyncStatus>;
  /**
   * Read a whole panel's connector states in one call. `probe` applies to the
   * batch, with the same cost as the single read — reserve it for explicit
   * "Test" surfaces. At most 100 ids; a connector the server cannot answer for
   * lands in `unknown` rather than failing the call.
   */
  getConnectorStatuses(
    workspaceSlug: string,
    connectorIds: string[],
    opts?: { probe?: boolean },
  ): Promise<ConnectorStatusBatch>;
  /** Pull content from the connector into the given project (synchronous). */
  fetchConnector(
    workspaceSlug: string,
    connectorId: string,
    projectId: string,
    paths?: string[],
  ): Promise<{ items_fetched: number }>;
  /** Publish the given project's content out through the connector (synchronous). */
  publishConnector(
    workspaceSlug: string,
    connectorId: string,
    projectId: string,
    message?: string,
  ): Promise<{ status: string }>;
  /**
   * List the content a connector can see, for the read-only content browser.
   * `projectId` scopes the listing when a connector can serve multiple projects.
   */
  listConnectorContent(
    workspaceSlug: string,
    connectorId: string,
    projectId?: string,
  ): Promise<ConnectorContentItem[]>;

  // PostHog locale-demand connector (project-scoped, phase 0, read-only).
  // The personal API key is write-only: config reads return a masked tail.
  getPostHogConnector(workspaceSlug: string, projectId: string): Promise<PostHogConnectorConfig>;
  savePostHogConnector(
    workspaceSlug: string,
    projectId: string,
    req: PostHogConnectorConfigRequest,
  ): Promise<PostHogConnectorConfig>;
  deletePostHogConnector(workspaceSlug: string, projectId: string): Promise<void>;
  getPostHogDemand(
    workspaceSlug: string,
    projectId: string,
    range: string,
    refresh?: boolean,
  ): Promise<PostHogDemandResponse>;

  // Editor
  /**
   * One page of an item's blocks. `opts` narrows the page server-side: a
   * locale's status bucket, a case-insensitive substring over source and
   * target, translatability, and the page bounds.
   */
  getFileBlocks(
    workspaceSlug: string,
    projectId: string,
    fileName: string,
    stream?: string,
    opts?: BlockQueryOptions,
  ): Promise<BlockInfo[]>;
  /**
   * The totals and status histogram for a block query, answered in SQL
   * without shipping a block. The histogram is what the call reports, so it
   * takes every block filter except `status`.
   */
  getBlockCounts(
    workspaceSlug: string,
    projectId: string,
    item?: string,
    locale?: string,
    stream?: string,
    opts?: { q?: string; translatable?: boolean },
  ): Promise<BlockCounts>;
  /**
   * One block, in the same shape `getFileBlocks` returns its elements.
   *
   * A surface that has just written a target reads back what the server now
   * holds rather than reconstructing it: the demotion an edit triggers, and
   * anything else the write recomputed, are the server's to decide.
   */
  getBlock(
    workspaceSlug: string,
    projectId: string,
    blockId: string,
    stream?: string,
  ): Promise<BlockInfo>;
  /** One item's metadata and block tallies, without the project's item list. */
  getItem(
    workspaceSlug: string,
    projectId: string,
    itemName: string,
    stream?: string,
  ): Promise<ItemInfo>;
  /**
   * Apply one review decision across a selection of blocks in a single
   * request. A block that refuses is reported in its own result; the call
   * itself succeeds.
   */
  bulkReviewBlocks(
    workspaceSlug: string,
    req: BulkReviewBlocksRequest,
  ): Promise<BulkReviewBlocksResult>;
  /**
   * Write the best content-memory match above the threshold into each
   * selected block's target, in one request.
   */
  bulkApplyMemory(
    workspaceSlug: string,
    req: BulkApplyMemoryRequest,
  ): Promise<BulkApplyMemoryResult>;
  /**
   * One page of the server-side translation review queue: (block, locale)
   * pairs still awaiting a decision, each with its hydrated block.
   */
  getPendingReview(
    workspaceSlug: string,
    projectId: string,
    opts?: PendingReviewOptions,
  ): Promise<PendingReviewPage>;
  updateBlockTarget(workspaceSlug: string, req: UpdateBlockRequest): Promise<void>;
  updateBlockTargetCoded(workspaceSlug: string, req: UpdateBlockTargetCodedRequest): Promise<void>;
  pseudoTranslateFile(
    workspaceSlug: string,
    projectId: string,
    fileName: string,
    targetLocale: string,
    stream?: string,
  ): Promise<TranslationStats>;
  aiTranslateFile(workspaceSlug: string, req: AITranslateFileRequest): Promise<TranslationStats>;
  memoryTranslateFile(
    workspaceSlug: string,
    projectId: string,
    fileName: string,
    targetLocale: string,
    stream?: string,
  ): Promise<TranslationStats>;
  getWordCount(
    workspaceSlug: string,
    projectId: string,
    fileName: string,
    stream?: string,
  ): Promise<WordCountResult>;
  exportTranslatedFile(
    workspaceSlug: string,
    projectId: string,
    fileName: string,
    targetLocale: string,
    stream?: string,
  ): Promise<Blob>;
  getTranslationDashboard(
    workspaceSlug: string,
    projectId: string,
    stream?: string,
    opts?: TranslationDashboardItemOpts,
  ): Promise<TranslationDashboardStats>;
  lookupMemoryForBlock(
    workspaceSlug: string,
    projectId: string,
    itemName: string,
    blockId: string,
    targetLocale: string,
    stream?: string,
  ): Promise<MemoryMatchInfo[]>;
  lookupTermsForBlock(
    workspaceSlug: string,
    projectId: string,
    itemName: string,
    blockId: string,
    targetLocale: string,
    stream?: string,
  ): Promise<BlockTermMatch[]>;
  /**
   * The five layers of context one unit is decided in, gathered server-side.
   * Both review surfaces call it for the unit under decision.
   */
  getReviewContext(
    workspaceSlug: string,
    projectId: string,
    itemName: string,
    blockId: string,
    targetLocale: string,
    stream?: string,
  ): Promise<ReviewContext>;

  // Block notes
  addBlockNote(
    workspaceSlug: string,
    projectId: string,
    blockId: string,
    text: string,
  ): Promise<BlockNote>;
  listBlockNotes(workspaceSlug: string, projectId: string, blockId: string): Promise<BlockNote[]>;
  deleteBlockNote(workspaceSlug: string, projectId: string, noteId: string): Promise<void>;

  // Block history
  getBlockHistory(
    workspaceSlug: string,
    projectId: string,
    blockId: string,
    locale: string,
    limit?: number,
    stream?: string,
  ): Promise<BlockHistoryEntry[]>;

  // Rollback / restore (#778)
  rollbackBlock(
    workspaceSlug: string,
    projectId: string,
    blockId: string,
    toSeq: number,
    locale: string,
    stream?: string,
  ): Promise<void>;
  revertBatch(
    workspaceSlug: string,
    projectId: string,
    correlationId: string,
    stream?: string,
  ): Promise<{ reverted: number }>;
  restoreToPoint(
    workspaceSlug: string,
    projectId: string,
    opts: RestorePointOptions,
  ): Promise<{ restored: number }>;
  setBlockStatus(
    workspaceSlug: string,
    projectId: string,
    blockId: string,
    status: BlockWorkflowStatus,
    reason?: string,
  ): Promise<void>;
  /**
   * Mark a block's target for `targetLocale` as reviewed (or back down the
   * ladder when `reviewed` is false) — the per-locale `Target.Status` review
   * ladder, distinct from the governance workflow lifecycle above. `rung`
   * picks where the call lands, and each direction has its own two: with
   * `reviewed` true, omitted/"reviewed" for an approval and "signed-off" for a
   * sign-off; with `reviewed` false, omitted/"translated" for a plain
   * un-review and "draft" for a reviewer rejection (re-enters the work queue).
   * A rung belonging to the other direction is ignored rather than sent.
   */
  reviewBlock(
    workspaceSlug: string,
    projectId: string,
    itemName: string,
    blockId: string,
    targetLocale: string,
    reviewed: boolean,
    stream?: string,
    rung?: ReviewRung,
  ): Promise<void>;

  /**
   * Bulk "Approve all passing": promote to reviewed every pending block that
   * passes checks + the compliance bar, leaving flagged ones. When this empties
   * the project's review queue the server starts the completing convergence
   * run + delivery (`review_completed: true`). The solo-founder fast path.
   */
  approvePassingReview(
    workspaceSlug: string,
    projectId: string,
    req?: ApprovePassingRequest,
  ): Promise<ApprovePassingResult>;

  // Back-to-source review (RV-F): a reviewer proposes a source-text fix; a source
  // owner (PermEditSource) approves it — applying it and re-drafting every locale
  // — or rejects it.
  createSourceProposal(
    workspaceSlug: string,
    projectId: string,
    req: CreateSourceProposalRequest,
  ): Promise<SourceProposal>;
  listSourceProposals(workspaceSlug: string, projectId: string): Promise<SourceProposal[]>;
  decideSourceProposal(
    workspaceSlug: string,
    projectId: string,
    proposalId: string,
    decision: "approve" | "reject",
    reason?: string,
  ): Promise<DecideSourceProposalResult>;

  /**
   * Promote a marked source entity to a real terms concept (RV-F piece 3).
   * Distinct from the term-candidate `/promote`: this creates a concept, which
   * fires concept.created and flows into the governed terminology re-check.
   */
  promoteEntityToConcept(
    workspaceSlug: string,
    projectId: string,
    itemName: string,
    blockId: string,
    entityKey: string,
    stream?: string,
  ): Promise<{ ok: boolean; concept: ConceptInfo }>;

  // Governance (#778): groups, deny rules, separation-of-duties, role overrides
  listGroups(workspaceSlug: string): Promise<Group[]>;
  createGroup(workspaceSlug: string, name: string, description?: string): Promise<Group>;
  deleteGroup(workspaceSlug: string, groupId: string): Promise<void>;
  listGroupMembers(workspaceSlug: string, groupId: string): Promise<string[]>;
  addGroupMember(workspaceSlug: string, groupId: string, userId: string): Promise<void>;
  removeGroupMember(workspaceSlug: string, groupId: string, userId: string): Promise<void>;
  listGroupBindings(workspaceSlug: string, groupId: string): Promise<GroupRoleBinding[]>;
  addGroupBinding(
    workspaceSlug: string,
    groupId: string,
    projectId: string,
    roleId: string,
    languages?: string[],
  ): Promise<GroupRoleBinding>;
  removeGroupBinding(workspaceSlug: string, groupId: string, bindingId: string): Promise<void>;
  listDenyRules(workspaceSlug: string): Promise<DenyRule[]>;
  createDenyRule(workspaceSlug: string, rule: DenyRuleInput): Promise<DenyRule>;
  deleteDenyRule(workspaceSlug: string, ruleId: string): Promise<void>;
  getSoDMode(workspaceSlug: string): Promise<{ mode: SoDMode }>;
  setSoDMode(workspaceSlug: string, mode: SoDMode): Promise<void>;
  listRoleOverrides(workspaceSlug: string): Promise<Record<string, string[]>>;
  setRoleOverride(workspaceSlug: string, role: string, permissions: string[]): Promise<void>;
  demoteVoiceRule(workspaceSlug: string, profileId: string, term: string): Promise<void>;

  // Checks
  runCheck(
    workspaceSlug: string,
    projectId: string,
    blockId: string,
    locale: string,
    stream?: string,
  ): Promise<CheckIssue[]>;
  runFileCheck(
    workspaceSlug: string,
    projectId: string,
    fileName: string,
    locale: string,
    stream?: string,
  ): Promise<FileCheckResult[]>;

  // Preview
  renderDocumentPreview(
    workspaceSlug: string,
    projectId: string,
    fileName: string,
    targetLocale: string,
    stream?: string,
  ): Promise<string>;
  renderBlockHTML(
    workspaceSlug: string,
    projectId: string,
    blockId: string,
    targetLocale: string,
    stream?: string,
  ): Promise<string>;

  // Content Memory
  getMemoryEntries(
    workspaceSlug: string,
    query: string,
    sourceLocale: string,
    targetLocale: string,
    offset: number,
    limit: number,
  ): Promise<MemorySearchResult>;
  getMemoryCount(workspaceSlug: string): Promise<number>;
  addMemoryEntry(
    workspaceSlug: string,
    source: string,
    target: string,
    sourceLocale: string,
    targetLocale: string,
  ): Promise<MemoryEntryInfo>;
  updateMemoryEntry(workspaceSlug: string, req: MemoryUpdateRequest): Promise<void>;
  deleteMemoryEntry(workspaceSlug: string, entryId: string): Promise<void>;
  /**
   * Delete a selection of content-memory entries in one request. Each id gets
   * its own result, so a missing entry does not strand the rest of the
   * selection. At most 500 ids.
   */
  bulkDeleteMemoryEntries(workspaceSlug: string, entryIds: string[]): Promise<BulkDeleteResult>;

  // Terminology
  getTerms(
    workspaceSlug: string,
    query: string,
    sourceLocale: string,
    targetLocale: string,
    offset: number,
    limit: number,
  ): Promise<TermSearchResult>;
  getTermCount(workspaceSlug: string): Promise<number>;
  addConcept(workspaceSlug: string, req: AddConceptRequest): Promise<ConceptInfo>;
  updateConcept(workspaceSlug: string, req: UpdateConceptRequest): Promise<void>;
  deleteConcept(workspaceSlug: string, conceptId: string): Promise<void>;
  /**
   * Always refuses: deleting a concept is a governed transition, so a
   * multi-select learns once — rather than once per row — that the batch
   * belongs in a change-set. Rejects with an ApiError carrying status 409 and
   * the {@link GovernedRefusal} envelope; read it with `governedRefusal`.
   */
  bulkDeleteConcepts(workspaceSlug: string, conceptIds: string[]): Promise<never>;
  importTermsCSV(
    workspaceSlug: string,
    csvContent: string,
    sourceLocale: string,
    targetLocale: string,
    domain: string,
    hasHeader: boolean,
  ): Promise<number>;
  importTermsJSON(workspaceSlug: string, jsonContent: string): Promise<number>;
  exportTermsJSON(workspaceSlug: string, name: string): Promise<string>;

  // Providers
  listProviderConfigs(workspaceSlug: string): Promise<ProviderConfig[]>;
  saveProviderConfig(workspaceSlug: string, cfg: ProviderConfigWithKey): Promise<ProviderConfig>;
  deleteProviderConfig(workspaceSlug: string, id: string): Promise<void>;
  testProviderConfig(workspaceSlug: string, cfg: ProviderConfigWithKey): Promise<void>;

  // Automations
  listAutomationRules(workspaceSlug: string, projectId: string): Promise<AutomationRule[]>;
  createAutomationRule(
    workspaceSlug: string,
    projectId: string,
    data: SaveAutomationRuleRequest,
  ): Promise<AutomationRule>;
  updateAutomationRule(
    workspaceSlug: string,
    projectId: string,
    ruleId: string,
    data: SaveAutomationRuleRequest,
  ): Promise<AutomationRule>;
  deleteAutomationRule(workspaceSlug: string, projectId: string, ruleId: string): Promise<void>;
  toggleAutomationRule(
    workspaceSlug: string,
    projectId: string,
    ruleId: string,
  ): Promise<AutomationRule>;
  listAutomationEvents(workspaceSlug: string, projectId: string): Promise<AutomationEvent[]>;
  /**
   * One page of a project's automation execution history, newest first. Page
   * on with the returned `next_cursor`; its absence means the last page.
   */
  listAutomationHistory(
    workspaceSlug: string,
    projectId: string,
    opts?: { limit?: number; cursor?: string },
  ): Promise<AutomationHistoryPage>;

  // Automation Runs (Bowrain AD-013)
  listAutomationRuns(
    workspaceSlug: string,
    projectId: string,
    status?: string,
    limit?: number,
  ): Promise<AutomationRun[]>;
  getAutomationRun(
    workspaceSlug: string,
    projectId: string,
    runId: string,
  ): Promise<{ run: AutomationRun; steps: AutomationStep[] }>;
  listStepLogs(
    workspaceSlug: string,
    projectId: string,
    runId: string,
    stepId: string,
    limit?: number,
  ): Promise<AutomationLogEntry[]>;
  cancelAutomationRun(workspaceSlug: string, projectId: string, runId: string): Promise<void>;

  // Flow definitions (Bowrain AD-013) — server-side, project-scoped pipeline
  // graphs that automation run_flow actions reference. Built-in flows are
  // merged into the listing; project flows are persisted server-side.
  listFlowDefinitions(workspaceSlug: string, projectId: string): Promise<FlowDefinitionInfo[]>;
  getFlowDefinition(
    workspaceSlug: string,
    projectId: string,
    flowId: string,
  ): Promise<FlowDefinitionInfo>;
  createFlowDefinition(
    workspaceSlug: string,
    projectId: string,
    def: FlowDefinitionInfo,
  ): Promise<FlowDefinitionInfo>;
  updateFlowDefinition(
    workspaceSlug: string,
    projectId: string,
    flowId: string,
    def: FlowDefinitionInfo,
  ): Promise<FlowDefinitionInfo>;
  deleteFlowDefinition(workspaceSlug: string, projectId: string, flowId: string): Promise<void>;

  // Notifications
  listNotifications(
    workspaceSlug: string,
    limit?: number,
    unreadOnly?: boolean,
  ): Promise<{ notifications: NotificationInfo[]; unread_count: number }>;
  markNotificationRead(workspaceSlug: string, id: string): Promise<void>;
  markAllNotificationsRead(workspaceSlug: string): Promise<void>;
  deleteNotification(workspaceSlug: string, id: string): Promise<void>;

  // Digest settings
  getDigestSettings(workspaceSlug: string): Promise<DigestSettingsDTO>;
  updateDigestSettings(
    workspaceSlug: string,
    settings: DigestSettingsDTO,
  ): Promise<DigestSettingsDTO>;

  // Entity annotations
  createEntity(
    workspaceSlug: string,
    projectId: string,
    itemName: string,
    blockId: string,
    entity: Partial<EntityInfo>,
  ): Promise<EntityInfo>;
  updateEntity(
    workspaceSlug: string,
    projectId: string,
    itemName: string,
    blockId: string,
    entityKey: string,
    entity: Partial<EntityInfo>,
  ): Promise<EntityInfo>;
  deleteEntity(
    workspaceSlug: string,
    projectId: string,
    itemName: string,
    blockId: string,
    entityKey: string,
  ): Promise<void>;
  promoteEntity(
    workspaceSlug: string,
    projectId: string,
    itemName: string,
    blockId: string,
    entityKey: string,
  ): Promise<void>;

  // Voice
  listVoiceProfiles(workspaceSlug: string): Promise<VoiceProfile[]>;
  getVoiceProfile(workspaceSlug: string, profileId: string): Promise<VoiceProfile>;
  createVoiceProfile(workspaceSlug: string, data: CreateVoiceProfileRequest): Promise<VoiceProfile>;
  updateVoiceProfile(workspaceSlug: string, data: UpdateVoiceProfileRequest): Promise<VoiceProfile>;
  deleteVoiceProfile(workspaceSlug: string, profileId: string): Promise<void>;
  getVoiceScores(workspaceSlug: string, projectId: string): Promise<StoredScore[]>;
  getVoiceTrends(workspaceSlug: string, projectId: string): Promise<ScoreTrend[]>;
  /**
   * Workspace-wide voice compliance rollup: one row per project (effective
   * profile, latest score, per-dimension breakdown, trend, drift, last
   * activity), aggregated from stored scores. Paginated for large workspaces.
   */
  getVoiceRollup(workspaceSlug: string, opts?: VoiceRollupOptions): Promise<VoiceRollup>;
  // Correction-learning loop (AD-019)
  /**
   * Record a reviewer's in-place correction (original → corrected) against the
   * bound voice profile. Feeds the correction-learning loop: repeated
   * corrections surface as candidate rules and auto-promote past the profile's
   * threshold. `ref` is the stream (defaults server-side).
   */
  recordVoiceCorrection(
    workspaceSlug: string,
    projectId: string,
    req: VoiceCorrectionRequest,
    stream?: string,
  ): Promise<VoiceCorrectionResult>;
  listVoiceCandidates(
    workspaceSlug: string,
    profileId: string,
    opts?: { minCount?: number; all?: boolean },
  ): Promise<CandidateRule[]>;
  promoteVoiceRule(
    workspaceSlug: string,
    profileId: string,
    rule: { term: string; replacement?: string; correction_count?: number },
  ): Promise<{ promoted: boolean }>;
  rejectVoiceRule(
    workspaceSlug: string,
    profileId: string,
    rule: { term: string; replacement?: string },
  ): Promise<void>;
  evaluateVoiceRule(
    workspaceSlug: string,
    profileId: string,
    req: {
      term: string;
      replacement?: string;
      project_id: string;
      stream?: string;
    },
  ): Promise<BlastRadius>;
  getVoiceDrift(
    workspaceSlug: string,
    projectId: string,
    opts?: { recentDays?: number; minScore?: number; dropPoints?: number },
  ): Promise<DriftResult>;
  /**
   * The workspace's governance profiles: every point its content occupies in
   * the context space, with the voice and collections at each. Derived
   * server-side from what pushes declared; nothing here is separately stored.
   */
  listContextProfiles(workspaceSlug: string): Promise<ContextProfilesResponse>;

  /**
   * The workspace's channel-slug equivalence proposals: the pairs two projects
   * spell differently that look like one channel. An omitted status returns
   * every one, judged and unjudged alike.
   */
  listChannelProposals(
    workspaceSlug: string,
    status?: ChannelProposalStatus,
  ): Promise<ChannelAliasProposalsResponse>;
  /**
   * Settles one proposal. Accepting records that the two spellings name one
   * channel; dismissing records that they do not, and stops the next push's
   * re-sighting from raising the pair again. Neither rewrites a project's slug:
   * a recipe resolves its own coordinates, offline.
   */
  judgeChannelProposal(
    workspaceSlug: string,
    judgement: ChannelAliasJudgement,
  ): Promise<ChannelAliasProposal>;

  createProfileFromStarter(
    workspaceSlug: string,
    pack: string,
    name?: string,
  ): Promise<VoiceProfile>;

  // Context scan (AI brand onboarding — epic 016). A scan drafts a voice
  // profile + candidate terms from pasted text, fetched pages, uploaded files,
  // and repo docs; the draft is reviewed and approved by a human via the
  // ordinary createVoiceProfile/createConcept surface.
  uploadContextScanSources(workspaceSlug: string, files: File[]): Promise<ContextScanUploadResult>;
  startContextScan(workspaceSlug: string, req: ContextScanRequest): Promise<{ job_id: string }>;
  getContextScan(workspaceSlug: string, jobId: string): Promise<ContextScanJob>;
  /**
   * Apply a reviewed scan: store the edited profile and create the approved
   * terms, in one transaction. Terms land at status "proposed" and only where
   * no concept already carries them in that locale, so a retry after a
   * partially applied approval is safe.
   */
  approveContextScan(
    workspaceSlug: string,
    scanId: string,
    req: ContextScanApproveRequest,
  ): Promise<ContextScanApproveResult>;
  /** Stateless deterministic check of sample text against an in-progress draft. */
  checkVoiceDraft(
    workspaceSlug: string,
    profile: VoiceProfile,
    text: string,
  ): Promise<ContextScanCheckResult>;

  /**
   * Approve one axis a scan proposed: record the recipe line it implies, for a
   * pull to write into `kapi.yaml`.
   *
   * Nothing here declares a coordinate. The recipe is the only thing that
   * mints one, so this returns a PENDING change and the axis becomes real once
   * that line lands in git and a push carries content at it.
   *
   * Rejects with a 409 when the claim cannot be composed — a structural axis
   * with no collection named, or a collection whose other half is not set yet.
   * Those messages are written for the reviewer and should be shown as-is.
   */
  approveAxis(
    workspaceSlug: string,
    projectId: string,
    req: ApproveAxisRequest,
  ): Promise<PendingRecipeChange>;

  // Audit log
  listWorkspaceAuditLog(workspaceSlug: string, query?: AuditQuery): Promise<AuditEntry[]>;
  verifyWorkspaceAuditChain(workspaceSlug: string): Promise<AuditChainVerification>;

  // Activities (Bowrain AD-014)
  /**
   * One page of the workspace activity feed. `type` and `types` are prefix
   * matches — `types` ORs one per element, which is how a feed spans families
   * ("review." plus "task.") that no single prefix expresses — and `types`
   * wins when both are given.
   */
  listActivities(
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
  ): Promise<ActivityPage>;

  markActivitiesSeen(workspaceSlug: string): Promise<void>;

  // Convergence runs (server-side `kapi up`) — Bowrain AD-022
  listConvergenceRuns(
    workspaceSlug: string,
    projectId: string,
    limit?: number,
  ): Promise<ConvergenceRun[]>;
  getConvergenceRun(
    workspaceSlug: string,
    projectId: string,
    runId: string,
  ): Promise<ConvergenceRun>;
  startConvergenceRun(
    workspaceSlug: string,
    projectId: string,
    opts?: {
      trigger?: string;
      locales?: string[];
      scope?: ConvergenceRunScope;
      confirmed?: boolean;
    },
  ): Promise<ConvergenceRun | null>;
  cancelConvergenceRun(workspaceSlug: string, projectId: string, runId: string): Promise<void>;
  /**
   * The provider-free pre-flight estimate for a project's next run (epic 019):
   * source readiness first, then per-locale content memory/AI work and credit cost for the
   * ready source, then the workspace balance. Starts no run.
   */
  estimateConvergence(workspaceSlug: string, projectId: string): Promise<ConvergenceEstimate>;

  /**
   * The workspace home's loop rollup: the most recent convergence run across
   * the workspace's projects plus a cached-basis ship-state rollup — one
   * request in place of a per-project fan-out. Absent fields mean "no data";
   * the corresponding card hides.
   */
  getLoopRollup(workspaceSlug: string): Promise<LoopRollup>;

  // Tasks (Bowrain AD-014)
  /**
   * One page of the workspace's tasks. `statuses` and `types` are sets matched
   * exactly and win over their singular counterparts — a surface spanning
   * review, review_terms and source_review has no single `type`.
   */
  listTasks(workspaceSlug: string, query?: TaskQuery): Promise<TaskPage>;
  /**
   * The same filter's totals, counted over the whole set rather than the page
   * — a kanban header that counts the rows it loaded understates a column with
   * more work than one page.
   */
  getTaskCounts(workspaceSlug: string, query?: TaskQuery): Promise<TaskCounts>;
  createTask(workspaceSlug: string, task: CreateTaskRequest): Promise<TaskInfo>;
  getTask(workspaceSlug: string, taskId: string): Promise<TaskInfo>;
  updateTask(
    workspaceSlug: string,
    taskId: string,
    updates: Partial<CreateTaskRequest>,
  ): Promise<TaskInfo>;
  deleteTask(workspaceSlug: string, taskId: string): Promise<void>;
  assignTask(workspaceSlug: string, taskId: string, assigneeId: string): Promise<void>;
  completeTask(workspaceSlug: string, taskId: string): Promise<void>;
  cancelTask(workspaceSlug: string, taskId: string): Promise<void>;
  listMyTasks(workspaceSlug: string, query?: Omit<TaskQuery, "assignee_id">): Promise<TaskPage>;

  // Notification preferences (Bowrain AD-014)
  getNotificationPreferences(
    workspaceSlug: string,
  ): Promise<{ preferences: NotificationPreference[] }>;
  updateNotificationPreferences(
    workspaceSlug: string,
    preferences: NotificationPreference[],
  ): Promise<void>;

  // @bravo Agent
  bravoCreateConversation(
    workspaceSlug: string,
    projectId?: string,
    title?: string,
  ): Promise<BravoConversation>;
  bravoListConversations(
    workspaceSlug: string,
    limit?: number,
    offset?: number,
  ): Promise<{ conversations: BravoConversation[]; total: number }>;
  bravoGetConversation(
    workspaceSlug: string,
    conversationId: string,
  ): Promise<{ conversation: BravoConversation; messages: BravoMessage[] }>;
  bravoDeleteConversation(workspaceSlug: string, conversationId: string): Promise<void>;
  bravoSendMessage(
    workspaceSlug: string,
    conversationId: string,
    content: string,
  ): Promise<{ user_message: BravoMessage; assistant_message: BravoMessage }>;
  bravoListMessages(
    workspaceSlug: string,
    conversationId: string,
    limit?: number,
    offset?: number,
  ): Promise<{ messages: BravoMessage[] }>;
  bravoApproveToolCall(
    workspaceSlug: string,
    conversationId: string,
    toolCallId: string,
  ): Promise<void>;
  bravoDenyToolCall(
    workspaceSlug: string,
    conversationId: string,
    toolCallId: string,
  ): Promise<void>;
  bravoCancelConversation(workspaceSlug: string, conversationId: string): Promise<void>;
  bravoGetConfig(workspaceSlug: string): Promise<BravoConfig>;
  bravoUpdateConfig(workspaceSlug: string, config: Partial<BravoConfig>): Promise<BravoConfig>;
  bravoListTools(workspaceSlug: string): Promise<{ tools: BravoToolInfo[] }>;
  bravoGetUsage(workspaceSlug: string, from?: string, to?: string): Promise<BravoUsageSummary>;
  bravoUpdateMode(
    workspaceSlug: string,
    conversationId: string,
    mode: string,
  ): Promise<{ mode: string; permissions: string[] }>;
  /** Send a message and stream the response via SSE. Returns an AbortController to cancel. */
  bravoSendMessageSSE(
    workspaceSlug: string,
    conversationId: string,
    content: string,
    handler: BravoSSEHandler,
    mode?: string,
    context?: { projectId?: string; stream?: string; itemId?: string },
  ): AbortController;

  // Billing
  billingGetOverview(workspaceSlug: string): Promise<BillingOverview>;
  billingGetUsage(workspaceSlug: string): Promise<BillingUsageBreakdown>;
  billingGetModelUsage(
    workspaceSlug: string,
    from?: string,
    to?: string,
  ): Promise<ModelUsageResponse>;
  /** The plans this deployment can actually sell, and whether the credit pack is buyable. */
  billingGetPlans(workspaceSlug: string): Promise<BillingPlansResponse>;
  /**
   * Start a subscription checkout. The client names a PLAN, never a price — the
   * server resolves the price, so a client can neither invent one nor pick a
   * cheaper one. `seats` applies to per-seat plans and defaults to the
   * workspace's current member count.
   */
  billingCreateCheckout(
    workspaceSlug: string,
    plan: BillingPlan,
    successUrl: string,
    cancelUrl: string,
    seats?: number,
  ): Promise<{ url: string }>;
  /** Buy one credit pack (a one-time payment; the size and price live server-side). */
  billingBuyCredits(
    workspaceSlug: string,
    successUrl: string,
    cancelUrl: string,
  ): Promise<{ url: string }>;
  billingCreatePortal(workspaceSlug: string, returnUrl: string): Promise<{ url: string }>;
  billingGetLedger(
    workspaceSlug: string,
    from?: string,
    to?: string,
    opts?: { limit?: number; offset?: number; operation?: string },
  ): Promise<CreditLedgerEntry[]>;
  /**
   * One page of the credit ledger with the window's totals beside it:
   * `usage_by_operation` is summed over the whole window, so the breakdown
   * stays correct however small the page is.
   */
  billingGetLedgerPage(workspaceSlug: string, query?: CreditLedgerQuery): Promise<CreditLedgerPage>;

  // Brand knowledge graph — Concepts (AD-021)
  // The concept routes share /api/v1/{ws}/concepts with the Terminology block
  // above; these add the graph/governance surface that hangs off a concept.
  // listConcepts/getConcept/createConcept return the same ConceptInfo /
  // TermSearchResult shapes as the Terminology methods; ordinary concept edits
  // reuse updateConcept/deleteConcept above.
  listConcepts(workspaceSlug: string, params?: ListConceptsParams): Promise<TermSearchResult>;
  /**
   * The workspace vocabulary counted by term lifecycle status. A concept
   * counts under every status one of its terms carries, so the buckets overlap
   * and do not sum to the total.
   */
  getConceptStatusCounts(workspaceSlug: string): Promise<ConceptStatusCounts>;
  /** Per-locale concept coverage over the workspace, most complete first. */
  getConceptLocaleCoverage(workspaceSlug: string): Promise<LocaleCoverageReport>;
  getConcept(workspaceSlug: string, conceptId: string): Promise<ConceptInfo>;
  createConcept(workspaceSlug: string, req: AddConceptRequest): Promise<ConceptInfo>;
  getConceptStory(workspaceSlug: string, conceptId: string): Promise<ConceptStory>;
  listConceptRelations(
    workspaceSlug: string,
    conceptId: string,
    scope?: RelationScope,
  ): Promise<ConceptRelation[]>;
  addConceptRelation(
    workspaceSlug: string,
    conceptId: string,
    req: AddConceptRelationRequest,
  ): Promise<ConceptRelation>;
  deleteConceptRelation(
    workspaceSlug: string,
    conceptId: string,
    relationId: string,
  ): Promise<void>;
  getConceptBlastRadius(workspaceSlug: string, conceptId: string): Promise<ConceptUsage>;
  /**
   * Which projects use a concept, from the workspace context graph — two hops
   * out of one vocabulary node rather than a scan over every stored block.
   * getConceptBlastRadius stays for the change-set preview it was built for:
   * that one asks what a draft WOULD do and must read the text to answer.
   */
  getConceptProjects(
    workspaceSlug: string,
    conceptId: string,
    query?: ConceptProjectsQuery,
  ): Promise<ConceptProjects>;
  listObservations(workspaceSlug: string, conceptId: string): Promise<Observation[]>;
  addObservation(
    workspaceSlug: string,
    conceptId: string,
    req: AddObservationRequest,
  ): Promise<Observation>;
  deleteObservation(workspaceSlug: string, conceptId: string, observationId: string): Promise<void>;
  listConceptComments(workspaceSlug: string, conceptId: string): Promise<Comment[]>;
  addConceptComment(
    workspaceSlug: string,
    conceptId: string,
    req: AddCommentRequest,
  ): Promise<Comment>;
  resolveConceptComment(
    workspaceSlug: string,
    conceptId: string,
    commentId: string,
    resolved?: boolean,
  ): Promise<void>;
  deleteConceptComment(workspaceSlug: string, conceptId: string, commentId: string): Promise<void>;

  // Brand knowledge graph — Markets (AD-021)
  listMarkets(workspaceSlug: string): Promise<Market[]>;
  createMarket(workspaceSlug: string, req: MarketRequest): Promise<Market>;
  updateMarket(workspaceSlug: string, marketId: string, req: MarketRequest): Promise<Market>;
  deleteMarket(workspaceSlug: string, marketId: string): Promise<void>;

  // Brand knowledge graph — Change-sets / experiments (AD-021)
  /** Change-set headers, each carrying `ops_count`. */
  listChangesets(workspaceSlug: string, status?: ChangeSetStatus): Promise<ChangeSet[]>;
  /** The workspace's change-sets bucketed by lifecycle status. */
  getChangesetCounts(workspaceSlug: string): Promise<ChangeSetCounts>;
  getChangeset(workspaceSlug: string, changesetId: string): Promise<ChangeSetDetail>;
  createChangeset(workspaceSlug: string, req: CreateChangeSetRequest): Promise<ChangeSet>;
  patchChangeset(
    workspaceSlug: string,
    changesetId: string,
    req: UpdateChangeSetRequest,
  ): Promise<ChangeSet>;
  appendChangesetOp(
    workspaceSlug: string,
    changesetId: string,
    req: AddChangeSetOpRequest,
  ): Promise<ChangeSetOp>;
  removeChangesetOp(workspaceSlug: string, changesetId: string, seq: number): Promise<void>;
  submitChangeset(workspaceSlug: string, changesetId: string): Promise<ChangeSet>;
  approveChangeset(
    workspaceSlug: string,
    changesetId: string,
    req?: ReviewRequest,
  ): Promise<ChangeSet>;
  rejectChangeset(
    workspaceSlug: string,
    changesetId: string,
    req?: ReviewRequest,
  ): Promise<ChangeSet>;
  mergeChangeset(workspaceSlug: string, changesetId: string): Promise<MergeResult>;
  abandonChangeset(workspaceSlug: string, changesetId: string): Promise<ChangeSet>;
  /**
   * A change-set's blast radius. A submitted change-set answers from its
   * stored summary — `stored: true`, `computed_at` set, and null `projects`
   * and `samples`, since the summary never carried the breakdown. Ask
   * {@link ApiAdapter.refreshChangesetBlastRadius} for the breakdown.
   */
  getChangesetBlastRadius(workspaceSlug: string, changesetId: string): Promise<ChangeSetImpact>;
  /**
   * Walk the blast radius now, ignoring any stored summary. Nothing is
   * persisted; the walk has a server-side time budget and reports `partial`
   * when it runs out.
   */
  refreshChangesetBlastRadius(workspaceSlug: string, changesetId: string): Promise<ChangeSetImpact>;
  addPilot(workspaceSlug: string, changesetId: string, req: StartPilotRequest): Promise<Pilot>;
  removePilot(
    workspaceSlug: string,
    changesetId: string,
    projectId: string,
    stream: string,
  ): Promise<void>;
  /**
   * The findings diff for one stream: what the check matchers raise under the
   * live graph and under the graph the draft would produce. No pilot need
   * exist — a reviewer may ask what a draft would do to a stream before binding
   * it to one.
   */
  trialFindings(
    workspaceSlug: string,
    changesetId: string,
    projectId: string,
    stream: string,
  ): Promise<TrialReport>;

  // Utility
  getKnownLocales(): Promise<LocaleInfo[]>;
  listFormats(): Promise<FormatInfo[]>;
  listTools(): Promise<ToolInfo[]>;
}
