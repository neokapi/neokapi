// The types and backend calls the Share and Assistant panels read, with the
// JSON names of the desktop backend's DTOs (ProjectSharing, AgentHostStatus,
// RulesFileView) and of the host results they return.
//
// Each panel imports `api` and its types from here, in one line. Once the
// backend mirrors these in `types/api.ts` and `hooks/useApi.ts`, this module
// becomes a re-export of those, or each import line moves to them.

import { call } from "../hooks/useApi";

/** The `context:` block of a recipe: where a project's context is shared. */
export interface ContextBackend {
  /** local, file, git or s3. Empty means local. */
  backend?: string;
  /** git: the ref the context is kept on. Default refs/kapi/context. */
  ref?: string;
  /** git: the remote pushed to and fetched from. Default origin. */
  remote?: string;
  /** file: the directory the context is kept in. */
  path?: string;
  bucket?: string;
  prefix?: string;
  region?: string;
  /** s3: an S3-compatible service other than AWS. */
  endpoint?: string;
}

/** Which remote a sync status describes. The Go struct carries no JSON tags. */
export interface RemoteDescriptor {
  Kind: string;
  Location: string;
}

/** How far this machine and the shared place are apart. */
export interface SyncStatus {
  remote: RemoteDescriptor;
  /** The project's operations the place is not known to hold. */
  to_push: number;
  /** The context operations among them, the ones `kapi context log` lists. */
  to_push_logged: number;
  to_pull: number;
  /** The last attempt to reach the place; absent when it never was. */
  contacted?: string;
  /** What the last attempt reported, absent when it succeeded. */
  error?: string;
}

/** Where one project's context lives and how far it is shared. */
export interface ProjectSharing {
  /** local, file, git or s3. */
  kind: string;
  location?: string;
  /** "recipe" when kapi.yaml names the place, "default" when it names none. */
  from: string;
  /** Absent for a local project. */
  status?: SyncStatus;
  /** The host of the recipe's `bowrain:` block, when it declares one. */
  venue_host?: string;
  venue_url?: string;
  /** The workspace directory on this machine. */
  workspace: string;
}

/** What a pull merged. */
export interface ContextPull extends SyncStatus {
  segments: number;
  /** The operations the log did not hold before. */
  merged: number;
  rebuilt?: boolean;
  checkpoint?: string;
  /** Nothing has been pushed to the place yet. */
  empty?: boolean;
  seconds: number;
}

/** What a push wrote. */
export interface ContextPush extends SyncStatus {
  pushed: number;
  pushed_logged: number;
  segments: number;
  blobs: number;
  checkpoint?: string;
  seconds: number;
}

/** What settling a merge established. */
export interface ContextSettleResult {
  range?: string;
  commit?: string;
  signals: unknown[];
  established: string[];
}

/** created, updated, unchanged or removed. */
export type RulesFileAction = string;

export interface RulesFile {
  /** Project-relative and slash-separated. */
  path: string;
  action: RulesFileAction;
}

/** What writing the rules files did. */
export interface RulesFilesResult {
  files: RulesFile[];
}

/** What one `kapi context sync` did. */
export interface ContextSyncResult {
  pull?: ContextPull;
  settle?: ContextSettleResult;
  push?: ContextPush;
  rules_files?: RulesFilesResult;
  /** Why the rules files could not be refreshed. The sync itself succeeded. */
  rules_files_error?: string;
}

/** One assistant kapi can connect, and whether it is connected for this project. */
export interface AgentHostStatus {
  /** claude-code, cursor, vscode, codex or claude-desktop. */
  host: string;
  label: string;
  detected: boolean;
  /** kapi's entry is present for this project. */
  wired: boolean;
  /** The files that carry it, relative to the project. Claude Desktop's is absolute. */
  files?: string[];
}

/** One file or skill directory connecting an assistant touched. */
export interface AgentWiringFile {
  /** "mcp" for a server configuration, "skill" for a skill directory. */
  kind: string;
  host: string;
  path: string;
  /** created, updated, unchanged or kept. */
  action: string;
}

/** What connecting an assistant wrote. */
export interface AgentWiringResult {
  hosts?: string[];
  files?: AgentWiringFile[];
}

/** One rules file kapi writes, with its section as written. */
export interface RulesFileView {
  /** Relative to the project root. */
  path: string;
  /** "" for the root. */
  folder: string;
  /** The kapi-owned section as written. */
  section: string;
  exists: boolean;
}

/** The backend calls the panels make, named after the Go methods. */
export const api = {
  projectSharing: (tabID: string) => call<ProjectSharing>("ProjectSharing", tabID),
  setProjectSharing: (tabID: string, spec: ContextBackend) =>
    call<ProjectSharing>("SetProjectSharing", tabID, spec),
  syncProjectContext: (tabID: string) => call<ContextSyncResult>("SyncProjectContext", tabID),
  agentHosts: (tabID: string) => call<AgentHostStatus[]>("AgentHosts", tabID),
  connectAgentHost: (tabID: string, host: string) =>
    call<AgentWiringResult>("ConnectAgentHost", tabID, host),
  rulesFiles: (tabID: string) => call<RulesFileView[]>("RulesFiles", tabID),
  writeRulesFiles: (tabID: string) => call<RulesFilesResult>("WriteRulesFiles", tabID),
};

/** The query keys the panels read under. */
export const contextQk = {
  projectSharing: (tabID: string) => ["project-sharing", tabID] as const,
  agentHosts: (tabID: string) => ["agent-hosts", tabID] as const,
  rulesFiles: (tabID: string) => ["rules-files", tabID] as const,
};
