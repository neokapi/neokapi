import type { ApiAdapter } from "./adapter";
import type { User, Workspace, Membership, ProjectInfo, ConfigResponse } from "../types/api";

/**
 * RestApiAdapter talks to the gokapi REST server.
 * Used by the web app (apps/web) and optionally by kapi serve.
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

  async getConfig(): Promise<ConfigResponse> {
    return this.fetchJSON("/api/v1/config");
  }

  async getCurrentUser(): Promise<User | null> {
    try {
      return await this.fetchJSON("/api/v1/auth/me");
    } catch {
      return null;
    }
  }

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

  async listProjects(workspaceSlug: string): Promise<ProjectInfo[]> {
    return this.fetchJSON(`/api/v1/workspaces/${workspaceSlug}/projects`);
  }

  async createProject(
    workspaceSlug: string,
    name: string,
    sourceLocale: string,
    targetLocales: string[],
  ): Promise<ProjectInfo> {
    return this.fetchJSON(`/api/v1/workspaces/${workspaceSlug}/projects`, {
      method: "POST",
      body: JSON.stringify({ name, source_locale: sourceLocale, target_locales: targetLocales }),
    });
  }
}
