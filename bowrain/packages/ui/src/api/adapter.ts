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
  BillingOverview,
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

/**
 * ApiAdapter abstracts the communication layer so that the same
 * UI components work in both the Wails desktop app and the REST web app.
 */
export interface ApiAdapter {
  // Server config
  getConfig(): Promise<ConfigResponse>;

  // Auth
  getCurrentUser(): Promise<User | null>;

  // Account management — onboarding (handle pick), email change, slug check.
  getOnboardingStatus(): Promise<OnboardingStatus>;
  completeOnboarding(slug: string, displayName?: string): Promise<Workspace>;
  checkSlug(slug: string): Promise<SlugCheckResponse>;
  requestEmailChange(newEmail: string): Promise<EmailChangeRequestResponse>;
  confirmEmailChange(token: string): Promise<EmailChangeConfirmResponse>;

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
    data: { description?: string; visibility?: string },
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
  ): Promise<ProjectInfo>;
  removeFile(
    workspaceSlug: string,
    projectId: string,
    fileName: string,
    stream?: string,
  ): Promise<ProjectInfo>;

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
  ): Promise<ProjectInfo>;

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
  // Correction-learning loop (AD-019)
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
    req: { term: string; replacement?: string; project_id: string; stream?: string },
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

  // Audit log
  listWorkspaceAuditLog(workspaceSlug: string, query?: AuditQuery): Promise<AuditEntry[]>;

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
  ): Promise<{ activities: ActivityInfo[]; next_cursor: string; new_count?: number }>;

  markActivitiesSeen(workspaceSlug: string): Promise<void>;

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
  billingCreateCheckout(
    workspaceSlug: string,
    priceId: string,
    successUrl: string,
    cancelUrl: string,
  ): Promise<{ url: string }>;
  billingCreatePortal(workspaceSlug: string, returnUrl: string): Promise<{ url: string }>;
  billingGetLedger(workspaceSlug: string, from?: string, to?: string): Promise<CreditLedgerEntry[]>;

  // Utility
  getKnownLocales(): Promise<LocaleInfo[]>;
  listFormats(): Promise<FormatInfo[]>;
  listTools(): Promise<ToolInfo[]>;
}
