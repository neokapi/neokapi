// Where the Fernwell project's context lives, one fixture per rung of the
// ladder: this Mac only, shared through git (in step, and with pushes
// waiting), a place that could not be reached, and connected to Bowrain.

import type { ContextSyncResult, ProjectSharing, SyncStatus } from "../../components/contextTypes";

/** The workspace directory, under a home that resolves on no machine. */
export const WORKSPACE_DIR = "/fakehome/.local/share/kapi/workspaces/default";

/** The last contact with the shared place: 10:03 today, local time. */
export const CONTACTED = (() => {
  const d = new Date();
  d.setHours(10, 3, 0, 0);
  return d.toISOString();
})();

const GIT_REMOTE = { Kind: "git", Location: "origin refs/kapi/context" };

function status(over: Partial<SyncStatus>): SyncStatus {
  return {
    remote: GIT_REMOTE,
    to_push: 0,
    to_push_logged: 0,
    to_pull: 0,
    contacted: CONTACTED,
    ...over,
  };
}

/** On this Mac: kapi.yaml names no place. */
export const LOCAL_SHARING: ProjectSharing = {
  kind: "local",
  from: "default",
  workspace: WORKSPACE_DIR,
};

/** Shared through git, with nothing waiting either way. */
export const GIT_IN_SYNC: ProjectSharing = {
  kind: "git",
  location: "origin refs/kapi/context",
  from: "recipe",
  status: status({}),
  workspace: WORKSPACE_DIR,
};

/** Shared through git, with two operations this Mac has not pushed. */
export const GIT_PUSHES_WAITING: ProjectSharing = {
  ...GIT_IN_SYNC,
  status: status({ to_push: 2, to_push_logged: 2 }),
};

/** Shared through git, and never synced from this Mac. */
export const GIT_NEVER_SYNCED: ProjectSharing = {
  ...GIT_IN_SYNC,
  status: status({ to_push: 4, to_push_logged: 4, contacted: undefined }),
};

/** Shared through a folder that could not be reached at the last attempt. */
export const FOLDER_UNREACHABLE: ProjectSharing = {
  kind: "file",
  location: "/Volumes/team/kapi/fernwell",
  from: "recipe",
  status: status({
    remote: { Kind: "file", Location: "/Volumes/team/kapi/fernwell" },
    to_push: 2,
    to_push_logged: 2,
    error: "open /Volumes/team/kapi/fernwell: no such file or directory",
  }),
  workspace: WORKSPACE_DIR,
};

/** Shared through S3. */
export const S3_IN_SYNC: ProjectSharing = {
  kind: "s3",
  location: "s3://acme-kapi/fernwell/",
  from: "recipe",
  status: status({ remote: { Kind: "s3", Location: "s3://acme-kapi/fernwell/" } }),
  workspace: WORKSPACE_DIR,
};

/** Connected to Bowrain, and shared through git as well. */
export const BOWRAIN_CONNECTED: ProjectSharing = {
  ...GIT_IN_SYNC,
  venue_host: "app.bowrain.cloud",
  venue_url: "https://app.bowrain.cloud/acme/help",
};

/** A sync that pulled three operations, pushed two and wrote the rules files. */
export const SYNC_RESULT: ContextSyncResult = {
  pull: { ...status({}), segments: 1, merged: 3, seconds: 0.4 },
  push: {
    ...status({}),
    pushed: 2,
    pushed_logged: 2,
    segments: 1,
    blobs: 0,
    seconds: 0.6,
  },
  rules_files: {
    files: [
      { path: "AGENTS.md", action: "updated" },
      { path: "CLAUDE.md", action: "unchanged" },
    ],
  },
};

/** The error a sync returns when the place cannot be reached, as the backend words it. */
export const SYNC_ERROR =
  "context sync: fetch origin refs/kapi/context: exit status 128: fatal: could not read from remote repository";
