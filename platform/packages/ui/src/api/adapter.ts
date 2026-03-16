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
  TranslationDashboardStats,
  ActivityInfo,
  TaskInfo,
  CreateTaskRequest,
  NotificationPreference,
} from "../types/api";
import type {
  VoiceProfile,
  StoredScore,
  ScoreTrend,
  CreateVoiceProfileRequest,
  UpdateVoiceProfileRequest,
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

  // API Tokens
  listApiTokens(workspaceSlug: string): Promise<ApiToken[]>;
  createApiToken(
    workspaceSlug: string,
    name: string,
    expireDays: number,
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
    data: { name?: string; target_languages?: string[] },
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

  // Notifications
  listNotifications(
    workspaceSlug: string,
    limit?: number,
    unreadOnly?: boolean,
  ): Promise<{ notifications: NotificationInfo[]; unread_count: number }>;
  markNotificationRead(workspaceSlug: string, id: string): Promise<void>;
  markAllNotificationsRead(workspaceSlug: string): Promise<void>;
  deleteNotification(workspaceSlug: string, id: string): Promise<void>;

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

  // Audit log
  listWorkspaceAuditLog(workspaceSlug: string, query?: AuditQuery): Promise<AuditEntry[]>;

  // Activities (AD-027)
  listActivities(
    workspaceSlug: string,
    query?: { project_id?: string; stream?: string; actor_id?: string; type?: string; cursor?: string; limit?: number },
  ): Promise<{ activities: ActivityInfo[]; next_cursor: string }>;

  // Tasks (AD-027)
  listTasks(
    workspaceSlug: string,
    query?: { project_id?: string; assignee_id?: string; status?: string; type?: string; priority?: string; cursor?: string; limit?: number },
  ): Promise<{ tasks: TaskInfo[]; next_cursor: string }>;
  createTask(workspaceSlug: string, task: CreateTaskRequest): Promise<TaskInfo>;
  getTask(workspaceSlug: string, taskId: string): Promise<TaskInfo>;
  updateTask(workspaceSlug: string, taskId: string, updates: Partial<CreateTaskRequest>): Promise<TaskInfo>;
  deleteTask(workspaceSlug: string, taskId: string): Promise<void>;
  assignTask(workspaceSlug: string, taskId: string, assigneeId: string): Promise<void>;
  completeTask(workspaceSlug: string, taskId: string): Promise<void>;
  cancelTask(workspaceSlug: string, taskId: string): Promise<void>;
  listMyTasks(
    workspaceSlug: string,
    query?: { status?: string; cursor?: string; limit?: number },
  ): Promise<{ tasks: TaskInfo[]; next_cursor: string }>;

  // Notification preferences (AD-027)
  getNotificationPreferences(workspaceSlug: string): Promise<{ preferences: NotificationPreference[] }>;
  updateNotificationPreferences(workspaceSlug: string, preferences: NotificationPreference[]): Promise<void>;

  // Utility
  getKnownLocales(): Promise<LocaleInfo[]>;
  listFormats(): Promise<FormatInfo[]>;
  listTools(): Promise<ToolInfo[]>;
}
