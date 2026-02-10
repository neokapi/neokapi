import type { User, Workspace, Membership, ProjectInfo, ConfigResponse } from "../types/api";

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

  // Projects (workspace-scoped)
  listProjects(workspaceSlug: string): Promise<ProjectInfo[]>;
  createProject(
    workspaceSlug: string,
    name: string,
    sourceLocale: string,
    targetLocales: string[],
  ): Promise<ProjectInfo>;
}
