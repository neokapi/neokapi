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
  SaveAutomationRuleRequest,
  NotificationInfo,
  EntityInfo,
  StreamInfo,
  StreamDiffResult,
  StreamMergeResult,
  CollectionInfo,
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
  ModelUsageResponse,
  TranslationDashboardStats,
  ActivityInfo,
  TaskInfo,
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
  BillingUsageBreakdown,
  CreditLedgerEntry,
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
    return {
      mode: "standalone",
      version: v.version,
      commit: v.commit,
      build_date: v.build_date,
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
  async updateProject(): Promise<ProjectInfo> {
    throw new Error("Not implemented");
  }
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
  async uploadToCollection(): Promise<ProjectInfo> {
    throw new Error("Not implemented");
  }

  // --- Editor ---
  async getFileBlocks(_ws: string, projectId: string, fileName: string): Promise<BlockInfo[]> {
    return Backend.GetItemBlocks(projectId, fileName) as Promise<BlockInfo[]>;
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
  async getTranslationDashboard(
    _ws: string,
    projectId: string,
    stream?: string,
  ): Promise<TranslationDashboardStats> {
    return Backend.GetTranslationDashboard(
      projectId,
      stream ?? "",
    ) as Promise<TranslationDashboardStats>;
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

  // --- Rollback / restore + governance (#778): server-only, not in desktop ---
  async rollbackBlock(): Promise<void> {}
  async revertBatch(): Promise<{ reverted: number }> {
    return { reverted: 0 };
  }
  async restoreToPoint(): Promise<{ restored: number }> {
    return { restored: 0 };
  }
  async setBlockStatus(): Promise<void> {}
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
  async demoteBrandRule(): Promise<void> {}

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

  // --- Brand Voice (proxied to the server's REST governance endpoints) ---
  async listBrandProfiles(workspaceSlug: string): Promise<VoiceProfile[]> {
    return Backend.ListBrandProfiles(workspaceSlug) as Promise<VoiceProfile[]>;
  }
  async getBrandProfile(workspaceSlug: string, profileId: string): Promise<VoiceProfile> {
    return Backend.GetBrandProfile(workspaceSlug, profileId) as Promise<VoiceProfile>;
  }
  async createBrandProfile(_ws: string, _data: CreateVoiceProfileRequest): Promise<VoiceProfile> {
    // Authoring profiles is a web/MCP workflow; the desktop governance surface
    // is review (promote/reject/evaluate), not profile creation.
    throw new Error("Creating brand profiles is not available in the desktop app");
  }
  async updateBrandProfile(_ws: string, _data: UpdateVoiceProfileRequest): Promise<VoiceProfile> {
    throw new Error("Editing brand profiles is not available in the desktop app");
  }
  async deleteBrandProfile(_ws: string, _profileId: string): Promise<void> {
    throw new Error("Deleting brand profiles is not available in the desktop app");
  }
  async getBrandScores(workspaceSlug: string, projectId: string): Promise<StoredScore[]> {
    return Backend.GetBrandScores(workspaceSlug, projectId) as Promise<StoredScore[]>;
  }
  async getBrandTrends(workspaceSlug: string, projectId: string): Promise<ScoreTrend[]> {
    return Backend.GetBrandTrends(workspaceSlug, projectId) as Promise<ScoreTrend[]>;
  }
  async listStarterPacks(): Promise<{ name: string; description: string }[]> {
    return Backend.ListStarterPacks() as Promise<{ name: string; description: string }[]>;
  }
  async createProfileFromStarter(
    _ws: string,
    _pack: string,
    _name?: string,
  ): Promise<VoiceProfile> {
    throw new Error("Creating brand profiles is not available in the desktop app");
  }

  // --- Correction-learning loop (AD-019, proxied to the server's REST endpoints) ---
  async listBrandCandidates(
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
  async promoteBrandRule(
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
  async rejectBrandRule(
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
    return Backend.EvaluateRule(workspaceSlug, profileId, {
      term: req.term,
      replacement: req.replacement ?? "",
      project_id: req.project_id,
      stream: req.stream ?? "",
    }) as Promise<BlastRadius>;
  }
  async getBrandDrift(
    workspaceSlug: string,
    projectId: string,
    opts?: { recentDays?: number; minScore?: number; dropPoints?: number },
  ): Promise<DriftResult> {
    return Backend.GetBrandDrift(
      workspaceSlug,
      projectId,
      opts?.recentDays ?? 0,
      opts?.minScore ?? 0,
      opts?.dropPoints ?? 0,
    ) as Promise<DriftResult>;
  }

  // --- Activities (Bowrain AD-014, not yet supported in desktop) ---
  async listActivities(
    _ws: string,
    _query?: {
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
    return { activities: [], next_cursor: "", new_count: 0 };
  }

  async markActivitiesSeen(_ws: string): Promise<void> {}

  // --- Tasks (Bowrain AD-014, not yet supported in desktop) ---
  async listTasks(
    _ws: string,
    _query?: {
      project_id?: string;
      assignee_id?: string;
      status?: string;
      type?: string;
      priority?: string;
      cursor?: string;
      limit?: number;
    },
  ): Promise<{ tasks: TaskInfo[]; next_cursor: string }> {
    return { tasks: [], next_cursor: "" };
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
  ): Promise<{ tasks: TaskInfo[]; next_cursor: string }> {
    return { tasks: [], next_cursor: "" };
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
  async billingCreateCheckout(
    _ws: string,
    _priceId: string,
    _successUrl: string,
    _cancelUrl: string,
  ): Promise<{ url: string }> {
    throw new Error("not implemented in desktop app");
  }
  async billingCreatePortal(_ws: string, _returnUrl: string): Promise<{ url: string }> {
    throw new Error("not implemented in desktop app");
  }
  async billingGetLedger(_ws: string, _from?: string, _to?: string): Promise<CreditLedgerEntry[]> {
    return [];
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
}
