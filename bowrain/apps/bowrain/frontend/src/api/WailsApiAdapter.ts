import type { ApiAdapter } from "@gokapi/ui";
import type {
  User, Workspace, Membership, ProjectInfo, ConfigResponse,
  BlockInfo, UpdateBlockRequest, UpdateBlockTargetCodedRequest,
  AITranslateFileRequest, TranslationStats, WordCountResult,
  ProviderConfig, ProviderConfigWithKey,
  TMEntryInfo, TMSearchResult, TMUpdateRequest, TMMatchInfo,
  ConceptInfo, TermSearchResult, AddConceptRequest, UpdateConceptRequest,
  BlockTermMatch, LocaleInfo, FormatInfo, ToolInfo,
  Invite, AcceptInviteResponse, ClaimProjectResponse,
} from "@gokapi/ui";

// eslint-disable-next-line @typescript-eslint/ban-ts-comment
// @ts-ignore – generated .js bindings outside the TS project root
import * as Backend from "../../bindings/github.com/gokapi/gokapi/bowrain/apps/bowrain/backend/app.js";

/**
 * ApiAdapter implementation for the Bowrain desktop app.
 * Wraps Wails v3 generated bindings to satisfy the shared ApiAdapter interface.
 * Workspace parameters are ignored (single-user desktop context).
 */
export class WailsApiAdapter implements ApiAdapter {
  // --- Server config ---
  async getConfig(): Promise<ConfigResponse> {
    const v = await Backend.GetVersion();
    return { mode: "standalone", version: v.version };
  }

  // --- Auth (not applicable in desktop) ---
  async getCurrentUser(): Promise<User | null> {
    return { id: "local", email: "local@bowrain", name: "Local User", avatar_url: "" };
  }

  // --- Workspaces (single workspace in desktop) ---
  async listWorkspaces(): Promise<Workspace[]> {
    return [{ id: "local", name: "Personal", slug: "personal", description: "", logo_url: "", type: "personal" as const, role: "owner" }];
  }
  async createWorkspace(): Promise<Workspace> {
    throw new Error("Workspaces not supported in desktop mode");
  }
  async getWorkspace(): Promise<Workspace> {
    return { id: "local", name: "Personal", slug: "personal", description: "", logo_url: "", type: "personal" as const, role: "owner" };
  }
  async updateWorkspace(): Promise<Workspace> {
    throw new Error("Workspaces not supported in desktop mode");
  }
  async deleteWorkspace(): Promise<void> {
    throw new Error("Workspaces not supported in desktop mode");
  }

  // --- Members (not applicable) ---
  async listMembers(): Promise<Membership[]> { return []; }
  async addMember(): Promise<void> { throw new Error("Not supported"); }
  async updateMemberRole(): Promise<void> { throw new Error("Not supported"); }
  async removeMember(): Promise<void> { throw new Error("Not supported"); }

  // --- Invites (not applicable in desktop) ---
  async listInvites(): Promise<Invite[]> { return []; }
  async createInvite(): Promise<Invite> { throw new Error("Not supported in desktop mode"); }
  async deleteInvite(): Promise<void> { throw new Error("Not supported in desktop mode"); }
  async acceptInvite(): Promise<AcceptInviteResponse> { throw new Error("Not supported in desktop mode"); }
  async claimProject(): Promise<ClaimProjectResponse> { throw new Error("Not supported in desktop mode"); }

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
  async deleteProject(_ws: string, projectId: string): Promise<void> {
    return Backend.CloseProject(projectId);
  }
  async uploadFiles(_ws: string, projectId: string, files: File[]): Promise<ProjectInfo> {
    // In Wails v3, File objects from DnD have a .path property
    const paths = files.map(f => (f as unknown as { path?: string }).path || f.name);
    return Backend.AddItems(projectId, paths) as Promise<ProjectInfo>;
  }
  async removeFile(_ws: string, projectId: string, fileName: string): Promise<ProjectInfo> {
    return Backend.RemoveItem(projectId, fileName) as Promise<ProjectInfo>;
  }

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
    return Backend.PseudoTranslateItem(projectId, fileName, targetLocale) as Promise<TranslationStats>;
  }
  async aiTranslateFile(_ws: string, req: AITranslateFileRequest): Promise<TranslationStats> {
    return Backend.AITranslateItem(req) as Promise<TranslationStats>;
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
    return Backend.LookupTMForBlock(projectId, itemName, blockId, targetLocale) as Promise<TMMatchInfo[]>;
  }
  async lookupTermsForBlock(
    _ws: string,
    projectId: string,
    itemName: string,
    blockId: string,
    targetLocale: string,
  ): Promise<BlockTermMatch[]> {
    return Backend.LookupTermsForBlock(projectId, itemName, blockId, targetLocale) as Promise<BlockTermMatch[]>;
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
    return Backend.GetTMEntries("", query, sourceLocale, targetLocale, offset, limit) as Promise<TMSearchResult>;
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
    return Backend.AddTMEntry("", source, target, sourceLocale, targetLocale) as Promise<TMEntryInfo>;
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
    return Backend.GetTerms("", query, sourceLocale, targetLocale, offset, limit) as Promise<TermSearchResult>;
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
    return Backend.ImportTermsCSV("", csvContent, sourceLocale, targetLocale, domain, hasHeader) as Promise<number>;
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

  // --- Desktop-specific helpers (not in ApiAdapter) ---
  async openFileInOS(path: string): Promise<void> {
    return Backend.OpenFileInOS(path);
  }
  async renderDocumentPreview(
    projectId: string,
    itemName: string,
    targetLocale: string,
  ): Promise<string> {
    return Backend.RenderDocumentPreview(projectId, itemName, targetLocale);
  }
  async renderBlockHTML(
    projectId: string,
    itemName: string,
    blockId: string,
    targetLocale: string,
  ): Promise<string> {
    return Backend.RenderBlockHTML(projectId, itemName, blockId, targetLocale);
  }
}
