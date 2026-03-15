import type { ApiAdapter } from "@neokapi/ui";
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
  CollectionInfo,
  AuditEntry,
  ArchivedProject,
} from "@neokapi/ui";

// eslint-disable-next-line @typescript-eslint/ban-ts-comment
// @ts-ignore – generated .js bindings outside the TS project root
import * as Backend from "../../bindings/github.com/neokapi/neokapi/bowrain/apps/bowrain/backend/app.js";

/**
 * ApiAdapter implementation for the Bowrain desktop app.
 * Wraps Wails v3 generated bindings to satisfy the shared ApiAdapter interface.
 * Workspace parameters are ignored (single-user desktop context).
 */
export class WailsApiAdapter implements ApiAdapter {
  // --- Server config ---
  async getConfig(): Promise<ConfigResponse> {
    const v = await Backend.GetVersion();
    return { mode: "standalone", version: v.version, commit: v.commit, build_date: v.build_date };
  }

  // --- Auth (not applicable in desktop) ---
  async getCurrentUser(): Promise<User | null> {
    return { id: "local", email: "local@bowrain", name: "Local User", avatar_url: "" };
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

  // --- Members (not applicable) ---
  async listMembers(): Promise<Membership[]> {
    return [];
  }
  async addMember(): Promise<void> {
    throw new Error("Not supported");
  }
  async updateMemberRole(): Promise<void> {
    throw new Error("Not supported");
  }
  async removeMember(): Promise<void> {
    throw new Error("Not supported");
  }

  // --- Invites (not applicable in desktop) ---
  async listInvites(): Promise<Invite[]> {
    return [];
  }
  async createInvite(): Promise<Invite> {
    throw new Error("Not supported in desktop mode");
  }
  async deleteInvite(): Promise<void> {
    throw new Error("Not supported in desktop mode");
  }
  async acceptInvite(): Promise<AcceptInviteResponse> {
    throw new Error("Not supported in desktop mode");
  }
  async claimProject(): Promise<ClaimProjectResponse> {
    throw new Error("Not supported in desktop mode");
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
  async updateStream(): Promise<StreamInfo> { throw new Error("Streams not yet supported in desktop mode"); }
  async deleteStream(): Promise<void> {
    throw new Error("Streams not yet supported in desktop mode");
  }
  async diffStream(): Promise<StreamDiffResult> {
    throw new Error("Streams not yet supported in desktop mode");
  }
  async mergeStream(): Promise<StreamMergeResult> {
    throw new Error("Streams not yet supported in desktop mode");
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
  async updateProject(): Promise<ProjectInfo> { throw new Error("Not implemented"); }
  async deleteProject(_ws: string, projectId: string): Promise<void> {
    return Backend.CloseProject(projectId);
  }
  async uploadFiles(_ws: string, projectId: string, files: File[]): Promise<ProjectInfo> {
    // In Wails v3, File objects from DnD have a .path property
    const paths = files.map((f) => (f as unknown as { path?: string }).path || f.name);
    return Backend.AddItems(projectId, paths) as Promise<ProjectInfo>;
  }
  async removeFile(_ws: string, projectId: string, fileName: string): Promise<ProjectInfo> {
    return Backend.RemoveItem(projectId, fileName) as Promise<ProjectInfo>;
  }

  // --- Archive / Bin (not applicable in desktop) ---
  async restoreProject(): Promise<void> {}
  async permanentlyDeleteProject(): Promise<void> {}
  async listArchivedProjects(): Promise<ArchivedProject[]> { return []; }
  async restoreStream(): Promise<void> {}

  // --- Audit Log (not applicable in desktop) ---
  async listWorkspaceAuditLog(): Promise<AuditEntry[]> { return []; }

  // --- Collections (not yet implemented in desktop) ---
  async listCollections(): Promise<CollectionInfo[]> { return []; }
  async createCollection(): Promise<CollectionInfo> { throw new Error("Not implemented"); }
  async getCollection(): Promise<CollectionInfo> { throw new Error("Not implemented"); }
  async updateCollection(): Promise<CollectionInfo> { throw new Error("Not implemented"); }
  async deleteCollection(): Promise<void> { throw new Error("Not implemented"); }
  async uploadToCollection(): Promise<ProjectInfo> { throw new Error("Not implemented"); }

  // --- Editor ---
  async getFileBlocks(_ws: string, projectId: string, fileName: string): Promise<BlockInfo[]> {
    return Backend.GetItemBlocks(projectId, fileName) as Promise<BlockInfo[]>;
  }
  async updateBlockTarget(_ws: string, req: UpdateBlockRequest): Promise<void> {
    return Backend.UpdateBlockTarget(req);
  }
  async updateBlockTargetCoded(_ws: string, req: UpdateBlockTargetCodedRequest): Promise<void> {
    return Backend.UpdateBlockTargetCoded(req);
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
  async tmTranslateFile(
    _ws: string,
    projectId: string,
    fileName: string,
    targetLocale: string,
  ): Promise<TranslationStats> {
    return Backend.TMTranslateItem(projectId, fileName, targetLocale) as Promise<TranslationStats>;
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
  async lookupTMForBlock(
    _ws: string,
    projectId: string,
    itemName: string,
    blockId: string,
    targetLocale: string,
  ): Promise<TMMatchInfo[]> {
    return Backend.LookupTMForBlock(projectId, itemName, blockId, targetLocale) as Promise<
      TMMatchInfo[]
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

  // --- Translation Memory ---
  async getTMEntries(
    _ws: string,
    query: string,
    sourceLocale: string,
    targetLocale: string,
    offset: number,
    limit: number,
  ): Promise<TMSearchResult> {
    // Bowrain TM API takes projectID as first arg; pass empty string for workspace-level
    return Backend.GetTMEntries(
      "",
      query,
      sourceLocale,
      targetLocale,
      offset,
      limit,
    ) as Promise<TMSearchResult>;
  }
  async getTMCount(): Promise<number> {
    return Backend.GetTMCount("") as Promise<number>;
  }
  async addTMEntry(
    _ws: string,
    source: string,
    target: string,
    sourceLocale: string,
    targetLocale: string,
  ): Promise<TMEntryInfo> {
    return Backend.AddTMEntry(
      "",
      source,
      target,
      sourceLocale,
      targetLocale,
    ) as Promise<TMEntryInfo>;
  }
  async updateTMEntry(_ws: string, req: TMUpdateRequest): Promise<void> {
    return Backend.UpdateTMEntry(req);
  }
  async deleteTMEntry(_ws: string, entryId: string): Promise<void> {
    return Backend.DeleteTMEntry("", entryId);
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

  // --- QA (desktop: not yet backed by Wails bindings) ---
  async runQACheck(
    _ws: string,
    _projectId: string,
    _blockId: string,
    _locale: string,
  ): Promise<QAIssue[]> {
    return [];
  }
  async runFileQACheck(
    _ws: string,
    _projectId: string,
    _fileName: string,
    _locale: string,
  ): Promise<FileQAResult[]> {
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
  async listAutomationHistory(_ws: string, _projectId: string): Promise<AutomationHistoryEntry[]> {
    return [];
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

  // --- Desktop-specific helpers (not in ApiAdapter) ---
  async openFileInOS(path: string): Promise<void> {
    return Backend.OpenFileInOS(path);
  }
}
