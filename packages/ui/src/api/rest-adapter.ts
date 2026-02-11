import type { ApiAdapter } from "./adapter";
import type {
  User, Workspace, Membership, ProjectInfo, ConfigResponse,
  BlockInfo, UpdateBlockRequest, UpdateBlockTargetCodedRequest,
  AITranslateFileRequest, TranslationStats, WordCountResult,
  ProviderConfig, ProviderConfigWithKey,
  TMEntryInfo, TMSearchResult, TMUpdateRequest, TMMatchInfo,
  ConceptInfo, TermSearchResult, AddConceptRequest, UpdateConceptRequest,
  BlockTermMatch, LocaleInfo, FormatInfo, ToolInfo,
} from "../types/api";

/**
 * RestApiAdapter talks to the gokapi REST server.
 * Used by the web apps (apps/web and apps/kapi-web).
 */
export class RestApiAdapter implements ApiAdapter {
  private baseUrl: string;
  private token: string | null;

  constructor(baseUrl: string = "", token: string | null = null) {
    this.baseUrl = baseUrl;
    this.token = token;
  }

  setToken(token: string) {
    this.token = token;
  }

  private headers(): Record<string, string> {
    const h: Record<string, string> = { "Content-Type": "application/json" };
    if (this.token) {
      h["Authorization"] = `Bearer ${this.token}`;
    }
    return h;
  }

  private async fetchJSON<T>(path: string, init?: RequestInit): Promise<T> {
    const resp = await fetch(`${this.baseUrl}${path}`, {
      ...init,
      headers: { ...this.headers(), ...init?.headers },
    });
    if (!resp.ok) {
      const body = await resp.text();
      throw new Error(`${resp.status}: ${body}`);
    }
    return resp.json();
  }

  private async fetchBlob(path: string, init?: RequestInit): Promise<Blob> {
    const resp = await fetch(`${this.baseUrl}${path}`, {
      ...init,
      headers: { ...this.headers(), ...init?.headers },
    });
    if (!resp.ok) {
      const body = await resp.text();
      throw new Error(`${resp.status}: ${body}`);
    }
    return resp.blob();
  }

  // ── Config ──────────────────────────────────────────────────────────────

  async getConfig(): Promise<ConfigResponse> {
    return this.fetchJSON("/api/v1/config");
  }

  // ── Auth ─────────────────────────────────────────────────────────────────

  async getCurrentUser(): Promise<User | null> {
    try {
      return await this.fetchJSON("/api/v1/auth/me");
    } catch {
      return null;
    }
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
    return this.fetchJSON(`/api/v1/workspaces/${slug}`);
  }

  async updateWorkspace(slug: string, data: Partial<Workspace>): Promise<Workspace> {
    return this.fetchJSON(`/api/v1/workspaces/${slug}`, {
      method: "PUT",
      body: JSON.stringify(data),
    });
  }

  async deleteWorkspace(slug: string): Promise<void> {
    await this.fetchJSON(`/api/v1/workspaces/${slug}`, { method: "DELETE" });
  }

  // ── Members ──────────────────────────────────────────────────────────────

  async listMembers(workspaceSlug: string): Promise<Membership[]> {
    return this.fetchJSON(`/api/v1/workspaces/${workspaceSlug}/members`);
  }

  async addMember(workspaceSlug: string, userId: string, role: string): Promise<void> {
    await this.fetchJSON(`/api/v1/workspaces/${workspaceSlug}/members`, {
      method: "POST",
      body: JSON.stringify({ user_id: userId, role }),
    });
  }

  async updateMemberRole(workspaceSlug: string, userId: string, role: string): Promise<void> {
    await this.fetchJSON(`/api/v1/workspaces/${workspaceSlug}/members/${userId}/role`, {
      method: "PUT",
      body: JSON.stringify({ role }),
    });
  }

  async removeMember(workspaceSlug: string, userId: string): Promise<void> {
    await this.fetchJSON(`/api/v1/workspaces/${workspaceSlug}/members/${userId}`, {
      method: "DELETE",
    });
  }

  // ── Projects ─────────────────────────────────────────────────────────────

  private ep(ws: string) {
    return `/api/v1/workspaces/${ws}/editor/projects`;
  }

  async listProjects(workspaceSlug: string): Promise<ProjectInfo[]> {
    return this.fetchJSON(this.ep(workspaceSlug));
  }

  async createProject(
    workspaceSlug: string,
    name: string,
    sourceLocale: string,
    targetLocales: string[],
  ): Promise<ProjectInfo> {
    return this.fetchJSON(this.ep(workspaceSlug), {
      method: "POST",
      body: JSON.stringify({ name, source_locale: sourceLocale, target_locales: targetLocales }),
    });
  }

  async getProject(workspaceSlug: string, projectId: string): Promise<ProjectInfo> {
    return this.fetchJSON(`${this.ep(workspaceSlug)}/${projectId}`);
  }

  async deleteProject(workspaceSlug: string, projectId: string): Promise<void> {
    await this.fetchJSON(`${this.ep(workspaceSlug)}/${projectId}`, {
      method: "DELETE",
    });
  }

  async uploadFiles(workspaceSlug: string, projectId: string, files: File[]): Promise<ProjectInfo> {
    const formData = new FormData();
    for (const file of files) {
      formData.append("files", file);
    }
    const resp = await fetch(`${this.baseUrl}${this.ep(workspaceSlug)}/${projectId}/files`, {
      method: "POST",
      headers: this.token ? { Authorization: `Bearer ${this.token}` } : {},
      body: formData,
    });
    if (!resp.ok) {
      const body = await resp.text();
      throw new Error(`${resp.status}: ${body}`);
    }
    return resp.json();
  }

  async removeFile(workspaceSlug: string, projectId: string, fileName: string): Promise<ProjectInfo> {
    return this.fetchJSON(
      `${this.ep(workspaceSlug)}/${projectId}/files/${encodeURIComponent(fileName)}`,
      { method: "DELETE" },
    );
  }

  // ── Editor ───────────────────────────────────────────────────────────────

  async getFileBlocks(workspaceSlug: string, projectId: string, fileName: string): Promise<BlockInfo[]> {
    return this.fetchJSON(
      `${this.ep(workspaceSlug)}/${projectId}/files/${encodeURIComponent(fileName)}/blocks`,
    );
  }

  async updateBlockTarget(workspaceSlug: string, req: UpdateBlockRequest): Promise<void> {
    await this.fetchJSON(
      `${this.ep(workspaceSlug)}/${req.project_id}/blocks/${req.block_id}`,
      { method: "PUT", body: JSON.stringify(req) },
    );
  }

  async updateBlockTargetCoded(workspaceSlug: string, req: UpdateBlockTargetCodedRequest): Promise<void> {
    await this.fetchJSON(
      `${this.ep(workspaceSlug)}/${req.project_id}/blocks/${req.block_id}/coded`,
      { method: "PUT", body: JSON.stringify(req) },
    );
  }

  async pseudoTranslateFile(workspaceSlug: string, projectId: string, fileName: string, targetLocale: string): Promise<TranslationStats> {
    return this.fetchJSON(
      `${this.ep(workspaceSlug)}/${projectId}/files/${encodeURIComponent(fileName)}/pseudo`,
      { method: "POST", body: JSON.stringify({ target_locale: targetLocale }) },
    );
  }

  async aiTranslateFile(workspaceSlug: string, req: AITranslateFileRequest): Promise<TranslationStats> {
    return this.fetchJSON(
      `${this.ep(workspaceSlug)}/${req.project_id}/files/${encodeURIComponent(req.item_name)}/ai-translate`,
      { method: "POST", body: JSON.stringify(req) },
    );
  }

  async tmTranslateFile(workspaceSlug: string, projectId: string, fileName: string, targetLocale: string): Promise<TranslationStats> {
    return this.fetchJSON(
      `${this.ep(workspaceSlug)}/${projectId}/files/${encodeURIComponent(fileName)}/tm-translate`,
      { method: "POST", body: JSON.stringify({ target_locale: targetLocale }) },
    );
  }

  async getWordCount(workspaceSlug: string, projectId: string, fileName: string): Promise<WordCountResult> {
    return this.fetchJSON(
      `${this.ep(workspaceSlug)}/${projectId}/files/${encodeURIComponent(fileName)}/wordcount`,
    );
  }

  async exportTranslatedFile(workspaceSlug: string, projectId: string, fileName: string, targetLocale: string): Promise<Blob> {
    return this.fetchBlob(
      `${this.ep(workspaceSlug)}/${projectId}/files/${encodeURIComponent(fileName)}/export`,
      { method: "POST", body: JSON.stringify({ target_locale: targetLocale }) },
    );
  }

  async lookupTMForBlock(workspaceSlug: string, projectId: string, itemName: string, blockId: string, targetLocale: string): Promise<TMMatchInfo[]> {
    return this.fetchJSON(
      `${this.ep(workspaceSlug)}/${projectId}/blocks/${blockId}/tm-lookup?item=${encodeURIComponent(itemName)}&target_locale=${targetLocale}`,
    );
  }

  async lookupTermsForBlock(workspaceSlug: string, projectId: string, itemName: string, blockId: string, targetLocale: string): Promise<BlockTermMatch[]> {
    return this.fetchJSON(
      `${this.ep(workspaceSlug)}/${projectId}/blocks/${blockId}/term-lookup?item=${encodeURIComponent(itemName)}&target_locale=${targetLocale}`,
    );
  }

  // ── Translation Memory ───────────────────────────────────────────────────

  private tmEp(ws: string) {
    return `/api/v1/workspaces/${ws}/tm`;
  }

  async getTMEntries(workspaceSlug: string, query: string, sourceLocale: string, targetLocale: string, offset: number, limit: number): Promise<TMSearchResult> {
    const params = new URLSearchParams({ q: query, source_locale: sourceLocale, target_locale: targetLocale, offset: String(offset), limit: String(limit) });
    return this.fetchJSON(`${this.tmEp(workspaceSlug)}?${params}`);
  }

  async getTMCount(workspaceSlug: string): Promise<number> {
    const resp: { count: number } = await this.fetchJSON(`${this.tmEp(workspaceSlug)}/count`);
    return resp.count;
  }

  async addTMEntry(workspaceSlug: string, source: string, target: string, sourceLocale: string, targetLocale: string): Promise<TMEntryInfo> {
    return this.fetchJSON(this.tmEp(workspaceSlug), {
      method: "POST",
      body: JSON.stringify({ source, target, source_locale: sourceLocale, target_locale: targetLocale }),
    });
  }

  async updateTMEntry(workspaceSlug: string, req: TMUpdateRequest): Promise<void> {
    await this.fetchJSON(`${this.tmEp(workspaceSlug)}/${req.entry_id}`, {
      method: "PUT",
      body: JSON.stringify(req),
    });
  }

  async deleteTMEntry(workspaceSlug: string, entryId: string): Promise<void> {
    await this.fetchJSON(`${this.tmEp(workspaceSlug)}/${entryId}`, { method: "DELETE" });
  }

  // ── Terminology ──────────────────────────────────────────────────────────

  private termsEp(ws: string) {
    return `/api/v1/workspaces/${ws}/terms`;
  }

  async getTerms(workspaceSlug: string, query: string, sourceLocale: string, targetLocale: string, offset: number, limit: number): Promise<TermSearchResult> {
    const params = new URLSearchParams({ q: query, source_locale: sourceLocale, target_locale: targetLocale, offset: String(offset), limit: String(limit) });
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
    await this.fetchJSON(`${this.termsEp(workspaceSlug)}/${conceptId}`, { method: "DELETE" });
  }

  async importTermsCSV(workspaceSlug: string, csvContent: string, sourceLocale: string, targetLocale: string, domain: string, hasHeader: boolean): Promise<number> {
    const resp: { imported: number } = await this.fetchJSON(`${this.termsEp(workspaceSlug)}/import/csv`, {
      method: "POST",
      body: JSON.stringify({ csv_content: csvContent, source_locale: sourceLocale, target_locale: targetLocale, domain, has_header: hasHeader }),
    });
    return resp.imported;
  }

  async importTermsJSON(workspaceSlug: string, jsonContent: string): Promise<number> {
    const resp: { imported: number } = await this.fetchJSON(`${this.termsEp(workspaceSlug)}/import/json`, {
      method: "POST",
      body: JSON.stringify({ json_content: jsonContent }),
    });
    return resp.imported;
  }

  async exportTermsJSON(workspaceSlug: string, name: string): Promise<string> {
    return this.fetchJSON(`${this.termsEp(workspaceSlug)}/export/json?name=${encodeURIComponent(name)}`);
  }

  // ── Providers ────────────────────────────────────────────────────────────

  async listProviderConfigs(workspaceSlug: string): Promise<ProviderConfig[]> {
    return this.fetchJSON(`/api/v1/workspaces/${workspaceSlug}/providers`);
  }

  async saveProviderConfig(workspaceSlug: string, cfg: ProviderConfigWithKey): Promise<ProviderConfig> {
    return this.fetchJSON(`/api/v1/workspaces/${workspaceSlug}/providers`, {
      method: "POST",
      body: JSON.stringify(cfg),
    });
  }

  async deleteProviderConfig(workspaceSlug: string, id: string): Promise<void> {
    await this.fetchJSON(`/api/v1/workspaces/${workspaceSlug}/providers/${id}`, { method: "DELETE" });
  }

  async testProviderConfig(workspaceSlug: string, cfg: ProviderConfigWithKey): Promise<void> {
    await this.fetchJSON(`/api/v1/workspaces/${workspaceSlug}/providers/test`, {
      method: "POST",
      body: JSON.stringify(cfg),
    });
  }

  // ── Utility ──────────────────────────────────────────────────────────────

  async getKnownLocales(): Promise<LocaleInfo[]> {
    return this.fetchJSON("/api/v1/locales");
  }

  async listFormats(): Promise<FormatInfo[]> {
    return this.fetchJSON("/api/v1/formats");
  }

  async listTools(): Promise<ToolInfo[]> {
    return this.fetchJSON("/api/v1/tools");
  }
}
