/** User info from auth system */
export interface User {
  id: string;
  email: string;
  name: string;
  avatar_url: string;
}

/** Workspace — top-level organizational unit */
export interface Workspace {
  id: string;
  name: string;
  slug: string;
  description: string;
  logo_url: string;
  role: string; // current user's role in the workspace
}

/** Membership — ties a user to a workspace */
export interface Membership {
  user_id: string;
  workspace_id: string;
  role: "owner" | "admin" | "member" | "viewer";
  user: User;
}

/** Project info */
export interface ProjectInfo {
  id: string;
  name: string;
  source_locale: string;
  target_locales: string[];
  workspace_id?: string;
  path?: string;
  items?: ProjectItem[];
  created_at: string;
  modified_at: string;
}

/** Item within a project */
export interface ProjectItem {
  name: string;
  format: string;
  type: string;
  size: number;
  block_count: number;
  word_count: number;
}

/** Server config response */
export interface ConfigResponse {
  mode: "local" | "server";
  version: string;
}
