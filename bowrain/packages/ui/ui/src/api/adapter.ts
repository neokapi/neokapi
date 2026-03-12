import type {
  User, Workspace, Membership, ProjectInfo, ConfigResponse,
  BlockInfo, UpdateBlockRequest, UpdateBlockTargetCodedRequest,
  AITranslateFileRequest, TranslationStats, WordCountResult,
  ProviderConfig, ProviderConfigWithKey,
  TMEntryInfo, TMSearchResult, TMUpdateRequest, TMMatchInfo,
  ConceptInfo, TermSearchResult, AddConceptRequest, UpdateConceptRequest,
  BlockTermMatch, BlockNote, BlockHistoryEntry, LocaleInfo, FormatInfo, ToolInfo,
  Invite, AcceptInviteResponse, ClaimProjectResponse,
  ApiToken, CreateApiTokenResponse,
  QAIssue, FileQAResult,
  AutomationRule, AutomationEvent, AutomationHistoryEntry, SaveAutomationRuleRequest,
  NotificationInfo, EntityInfo,
  StreamInfo, StreamDiffResult, StreamMergeResult, CreateStreamRequest,
} from "../types/api";

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
  createInvite(workspaceSlug: string, email: string, role: string, maxUses: number): Promise<Invite>;
  deleteInvite(workspaceSlug: string, inviteId: string): Promise<void>;
  acceptInvite(code: string): Promise<AcceptInviteResponse>;

  // API Tokens
  listApiTokens(workspaceSlug: string): Promise<ApiToken[]>;
  createApiToken(workspaceSlug: string, name: string, expireDays: number): Promise<CreateApiTokenResponse>;
  deleteApiToken(workspaceSlug: string, tokenId: string): Promise<void>;

  // Claim
  claimProject(claimToken: string): Promise<ClaimProjectResponse>;

  // Streams
  listStreams(workspaceSlug: string, projectId: string): Promise<StreamInfo[]>;
  createStream(workspaceSlug: string, projectId: string, req: CreateStreamRequest): Promise<StreamInfo>;
  getStream(workspaceSlug: string, projectId: string, streamName: string): Promise<StreamInfo>;
  deleteStream(workspaceSlug: string, projectId: string, streamName: string): Promise<void>;
  diffStream(workspaceSlug: string, projectId: string, streamName: string): Promise<StreamDiffResult>;
  mergeStream(workspaceSlug: string, projectId: string, streamName: string, dryRun?: boolean): Promise<StreamMergeResult>;

  // Projects (workspace-scoped)
  listProjects(workspaceSlug: string): Promise<ProjectInfo[]>;
  createProject(
    workspaceSlug: string,
    name: string,
    sourceLocale: string,
    targetLocales: string[],
  ): Promise<ProjectInfo>;
  getProject(workspaceSlug: string, projectId: string, stream?: string): Promise<ProjectInfo>;
  deleteProject(workspaceSlug: string, projectId: string): Promise<void>;
  uploadFiles(workspaceSlug: string, projectId: string, files: File[], stream?: string): Promise<ProjectInfo>;
  removeFile(workspaceSlug: string, projectId: string, fileName: string, stream?: string): Promise<ProjectInfo>;

  // Editor
  getFileBlocks(workspaceSlug: string, projectId: string, fileName: string, stream?: string): Promise<BlockInfo[]>;
  updateBlockTarget(workspaceSlug: string, req: UpdateBlockRequest): Promise<void>;
  updateBlockTargetCoded(workspaceSlug: string, req: UpdateBlockTargetCodedRequest): Promise<void>;
  pseudoTranslateFile(workspaceSlug: string, projectId: string, fileName: string, targetLocale: string, stream?: string): Promise<TranslationStats>;
  aiTranslateFile(workspaceSlug: string, req: AITranslateFileRequest): Promise<TranslationStats>;
  tmTranslateFile(workspaceSlug: string, projectId: string, fileName: string, targetLocale: string, stream?: string): Promise<TranslationStats>;
  getWordCount(workspaceSlug: string, projectId: string, fileName: string, stream?: string): Promise<WordCountResult>;
  exportTranslatedFile(workspaceSlug: string, projectId: string, fileName: string, targetLocale: string, stream?: string): Promise<Blob>;
  lookupTMForBlock(workspaceSlug: string, projectId: string, itemName: string, blockId: string, targetLocale: string, stream?: string): Promise<TMMatchInfo[]>;
  lookupTermsForBlock(workspaceSlug: string, projectId: string, itemName: string, blockId: string, targetLocale: string, stream?: string): Promise<BlockTermMatch[]>;

  // Block notes
  addBlockNote(workspaceSlug: string, projectId: string, blockId: string, text: string): Promise<BlockNote>;
  listBlockNotes(workspaceSlug: string, projectId: string, blockId: string): Promise<BlockNote[]>;
  deleteBlockNote(workspaceSlug: string, projectId: string, noteId: string): Promise<void>;

  // Block history
  getBlockHistory(workspaceSlug: string, projectId: string, blockId: string, locale: string, limit?: number, stream?: string): Promise<BlockHistoryEntry[]>;

  // QA
  runQACheck(workspaceSlug: string, projectId: string, blockId: string, locale: string, stream?: string): Promise<QAIssue[]>;
  runFileQACheck(workspaceSlug: string, projectId: string, fileName: string, locale: string, stream?: string): Promise<FileQAResult[]>;

  // Preview
  renderDocumentPreview(workspaceSlug: string, projectId: string, fileName: string, targetLocale: string, stream?: string): Promise<string>;
  renderBlockHTML(workspaceSlug: string, projectId: string, blockId: string, targetLocale: string, stream?: string): Promise<string>;

  // Translation Memory
  getTMEntries(workspaceSlug: string, query: string, sourceLocale: string, targetLocale: string, offset: number, limit: number): Promise<TMSearchResult>;
  getTMCount(workspaceSlug: string): Promise<number>;
  addTMEntry(workspaceSlug: string, source: string, target: string, sourceLocale: string, targetLocale: string): Promise<TMEntryInfo>;
  updateTMEntry(workspaceSlug: string, req: TMUpdateRequest): Promise<void>;
  deleteTMEntry(workspaceSlug: string, entryId: string): Promise<void>;

  // Terminology
  getTerms(workspaceSlug: string, query: string, sourceLocale: string, targetLocale: string, offset: number, limit: number): Promise<TermSearchResult>;
  getTermCount(workspaceSlug: string): Promise<number>;
  addConcept(workspaceSlug: string, req: AddConceptRequest): Promise<ConceptInfo>;
  updateConcept(workspaceSlug: string, req: UpdateConceptRequest): Promise<void>;
  deleteConcept(workspaceSlug: string, conceptId: string): Promise<void>;
  importTermsCSV(workspaceSlug: string, csvContent: string, sourceLocale: string, targetLocale: string, domain: string, hasHeader: boolean): Promise<number>;
  importTermsJSON(workspaceSlug: string, jsonContent: string): Promise<number>;
  exportTermsJSON(workspaceSlug: string, name: string): Promise<string>;

  // Providers
  listProviderConfigs(workspaceSlug: string): Promise<ProviderConfig[]>;
  saveProviderConfig(workspaceSlug: string, cfg: ProviderConfigWithKey): Promise<ProviderConfig>;
  deleteProviderConfig(workspaceSlug: string, id: string): Promise<void>;
  testProviderConfig(workspaceSlug: string, cfg: ProviderConfigWithKey): Promise<void>;

  // Automations
  listAutomationRules(workspaceSlug: string, projectId: string): Promise<AutomationRule[]>;
  createAutomationRule(workspaceSlug: string, projectId: string, data: SaveAutomationRuleRequest): Promise<AutomationRule>;
  updateAutomationRule(workspaceSlug: string, projectId: string, ruleId: string, data: SaveAutomationRuleRequest): Promise<AutomationRule>;
  deleteAutomationRule(workspaceSlug: string, projectId: string, ruleId: string): Promise<void>;
  toggleAutomationRule(workspaceSlug: string, projectId: string, ruleId: string): Promise<AutomationRule>;
  listAutomationEvents(workspaceSlug: string, projectId: string): Promise<AutomationEvent[]>;
  listAutomationHistory(workspaceSlug: string, projectId: string): Promise<AutomationHistoryEntry[]>;

  // Notifications
  listNotifications(workspaceSlug: string, limit?: number, unreadOnly?: boolean): Promise<{ notifications: NotificationInfo[]; unread_count: number }>;
  markNotificationRead(workspaceSlug: string, id: string): Promise<void>;
  markAllNotificationsRead(workspaceSlug: string): Promise<void>;
  deleteNotification(workspaceSlug: string, id: string): Promise<void>;

  // Entity annotations
  createEntity(workspaceSlug: string, projectId: string, itemName: string, blockId: string, entity: Partial<EntityInfo>): Promise<EntityInfo>;
  updateEntity(workspaceSlug: string, projectId: string, itemName: string, blockId: string, entityKey: string, entity: Partial<EntityInfo>): Promise<EntityInfo>;
  deleteEntity(workspaceSlug: string, projectId: string, itemName: string, blockId: string, entityKey: string): Promise<void>;
  promoteEntity(workspaceSlug: string, projectId: string, itemName: string, blockId: string, entityKey: string): Promise<void>;

  // Utility
  getKnownLocales(): Promise<LocaleInfo[]>;
  listFormats(): Promise<FormatInfo[]>;
  listTools(): Promise<ToolInfo[]>;
}
