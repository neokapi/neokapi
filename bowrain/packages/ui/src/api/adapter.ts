import type {
  User,
  Workspace,
  Membership,
  ProjectInfo,
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
  ActivityInfo,
  ConvergenceRun,
  ConvergenceEstimate,
  LoopRollup,
  ConvergenceRunScope,
  TaskInfo,
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
  ReviewDemotion,
  ApprovePassingRequest,
  ApprovePassingResult,
  BrandScanRequest,
  BrandScanUploadResult,
  BrandScanJob,
  BrandScanCheckResult,
  InstallationRepo,
  BindInstallationRepoRequest,
  BindInstallationRepoResult,
  RepoDetection,
  RepoDetectOptions,
  ModelRecommendationsResponse,
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
  BrandRollup,
  BrandRollupOptions,
  BrandCorrectionRequest,
  BrandCorrectionResult,
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
    },
  ): Promise<ProjectMembership>;
  updateProjectMember(
    workspaceSlug: string,
    projectId: string,
    userId: string,
    data: {
      role_id: string;
      languages?: string[];
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
  getProject(workspaceSlug: string, projectId: string, stream?: string): Promise<ProjectInfo>;
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
  getFileBlocks(
    workspaceSlug: string,
    projectId: string,
    fileName: string,
    stream?: string,
  ): Promise<BlockInfo[]>;
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
  tmTranslateFile(
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
  lookupTMForBlock(
    workspaceSlug: string,
    projectId: string,
    itemName: string,
    blockId: string,
    targetLocale: string,
    stream?: string,
  ): Promise<TMMatchInfo[]>;
  lookupTermsForBlock(
    workspaceSlug: string,
    projectId: string,
    itemName: string,
    blockId: string,
    targetLocale: string,
    stream?: string,
  ): Promise<BlockTermMatch[]>;

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
   * ladder, distinct from the governance workflow lifecycle above. `demoteTo`
   * picks the rung a clearing call lands on: omitted/"translated" for a plain
   * un-review, "draft" for a reviewer rejection (re-enters the work queue).
   * It is ignored when `reviewed` is true.
   */
  reviewBlock(
    workspaceSlug: string,
    projectId: string,
    itemName: string,
    blockId: string,
    targetLocale: string,
    reviewed: boolean,
    stream?: string,
    demoteTo?: ReviewDemotion,
  ): Promise<void>;

  /**
   * Bulk "Approve all passing": promote to reviewed every pending block that
   * passes checks + the on-brand bar, leaving flagged ones. When this empties
   * the project's review queue the server starts the completing convergence
   * run + delivery (`review_completed: true`). The solo-founder fast path.
   */
  approvePassingReview(
    workspaceSlug: string,
    projectId: string,
    req?: ApprovePassingRequest,
  ): Promise<ApprovePassingResult>;

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
  demoteBrandRule(workspaceSlug: string, profileId: string, term: string): Promise<void>;

  // QA
  runQACheck(
    workspaceSlug: string,
    projectId: string,
    blockId: string,
    locale: string,
    stream?: string,
  ): Promise<QAIssue[]>;
  runFileQACheck(
    workspaceSlug: string,
    projectId: string,
    fileName: string,
    locale: string,
    stream?: string,
  ): Promise<FileQAResult[]>;

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

  // Translation Memory
  getTMEntries(
    workspaceSlug: string,
    query: string,
    sourceLocale: string,
    targetLocale: string,
    offset: number,
    limit: number,
  ): Promise<TMSearchResult>;
  getTMCount(workspaceSlug: string): Promise<number>;
  addTMEntry(
    workspaceSlug: string,
    source: string,
    target: string,
    sourceLocale: string,
    targetLocale: string,
  ): Promise<TMEntryInfo>;
  updateTMEntry(workspaceSlug: string, req: TMUpdateRequest): Promise<void>;
  deleteTMEntry(workspaceSlug: string, entryId: string): Promise<void>;

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
  listAutomationHistory(
    workspaceSlug: string,
    projectId: string,
  ): Promise<AutomationHistoryEntry[]>;

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

  // Brand Voice
  listBrandProfiles(workspaceSlug: string): Promise<VoiceProfile[]>;
  getBrandProfile(workspaceSlug: string, profileId: string): Promise<VoiceProfile>;
  createBrandProfile(workspaceSlug: string, data: CreateVoiceProfileRequest): Promise<VoiceProfile>;
  updateBrandProfile(workspaceSlug: string, data: UpdateVoiceProfileRequest): Promise<VoiceProfile>;
  deleteBrandProfile(workspaceSlug: string, profileId: string): Promise<void>;
  getBrandScores(workspaceSlug: string, projectId: string): Promise<StoredScore[]>;
  getBrandTrends(workspaceSlug: string, projectId: string): Promise<ScoreTrend[]>;
  /**
   * Workspace-wide brand-compliance rollup: one row per project (effective
   * profile, latest score, per-dimension breakdown, trend, drift, last
   * activity), aggregated from stored scores. Paginated for large workspaces.
   */
  getBrandRollup(workspaceSlug: string, opts?: BrandRollupOptions): Promise<BrandRollup>;
  // Correction-learning loop (AD-019)
  /**
   * Record a reviewer's in-place correction (original → corrected) against the
   * bound brand profile. Feeds the correction-learning loop: repeated
   * corrections surface as candidate rules and auto-promote past the profile's
   * threshold. `ref` is the stream (defaults server-side).
   */
  recordBrandCorrection(
    workspaceSlug: string,
    projectId: string,
    req: BrandCorrectionRequest,
    stream?: string,
  ): Promise<BrandCorrectionResult>;
  listBrandCandidates(
    workspaceSlug: string,
    profileId: string,
    opts?: { minCount?: number; all?: boolean },
  ): Promise<CandidateRule[]>;
  promoteBrandRule(
    workspaceSlug: string,
    profileId: string,
    rule: { term: string; replacement?: string; correction_count?: number },
  ): Promise<{ promoted: boolean }>;
  rejectBrandRule(
    workspaceSlug: string,
    profileId: string,
    rule: { term: string; replacement?: string },
  ): Promise<void>;
  evaluateBrandRule(
    workspaceSlug: string,
    profileId: string,
    req: {
      term: string;
      replacement?: string;
      project_id: string;
      stream?: string;
    },
  ): Promise<BlastRadius>;
  getBrandDrift(
    workspaceSlug: string,
    projectId: string,
    opts?: { recentDays?: number; minScore?: number; dropPoints?: number },
  ): Promise<DriftResult>;
  listStarterPacks(): Promise<{ name: string; description: string }[]>;
  createProfileFromStarter(
    workspaceSlug: string,
    pack: string,
    name?: string,
  ): Promise<VoiceProfile>;

  // Brand scan (AI brand onboarding — epic 016). A scan drafts a voice
  // profile + candidate glossary from pasted text, fetched pages, uploaded
  // files, and repo docs; the draft is reviewed and approved by a human via
  // the ordinary createBrandProfile/createConcept surface.
  uploadBrandScanSources(workspaceSlug: string, files: File[]): Promise<BrandScanUploadResult>;
  startBrandScan(workspaceSlug: string, req: BrandScanRequest): Promise<{ job_id: string }>;
  getBrandScan(workspaceSlug: string, jobId: string): Promise<BrandScanJob>;
  /** Stateless deterministic check of sample text against an in-progress draft. */
  checkBrandDraft(
    workspaceSlug: string,
    profile: VoiceProfile,
    text: string,
  ): Promise<BrandScanCheckResult>;

  // Audit log
  listWorkspaceAuditLog(workspaceSlug: string, query?: AuditQuery): Promise<AuditEntry[]>;
  verifyWorkspaceAuditChain(workspaceSlug: string): Promise<AuditChainVerification>;

  // Activities (Bowrain AD-014)
  listActivities(
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
  }>;

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
   * source readiness first, then per-locale TM/AI work and credit cost for the
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
  listTasks(
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
  ): Promise<{ tasks: TaskInfo[]; next_cursor: string }>;
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
  listMyTasks(
    workspaceSlug: string,
    query?: { status?: string; cursor?: string; limit?: number },
  ): Promise<{ tasks: TaskInfo[]; next_cursor: string }>;

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
  billingGetLedger(workspaceSlug: string, from?: string, to?: string): Promise<CreditLedgerEntry[]>;

  // Brand knowledge graph — Concepts (AD-021)
  // The concept routes share /api/v1/{ws}/concepts with the Terminology block
  // above; these add the graph/governance surface that hangs off a concept.
  // listConcepts/getConcept/createConcept return the same ConceptInfo /
  // TermSearchResult shapes as the Terminology methods; ordinary concept edits
  // reuse updateConcept/deleteConcept above.
  listConcepts(workspaceSlug: string, params?: ListConceptsParams): Promise<TermSearchResult>;
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
  listChangesets(workspaceSlug: string, status?: ChangeSetStatus): Promise<ChangeSet[]>;
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
  getChangesetBlastRadius(workspaceSlug: string, changesetId: string): Promise<ChangeSetImpact>;
  addPilot(workspaceSlug: string, changesetId: string, req: StartPilotRequest): Promise<Pilot>;
  removePilot(
    workspaceSlug: string,
    changesetId: string,
    projectId: string,
    stream: string,
  ): Promise<void>;

  // Utility
  getKnownLocales(): Promise<LocaleInfo[]>;
  listFormats(): Promise<FormatInfo[]>;
  listTools(): Promise<ToolInfo[]>;
}
