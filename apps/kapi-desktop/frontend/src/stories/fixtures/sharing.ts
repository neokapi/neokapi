// Where the Fernwell project's context lives, one fixture per rung of the
// ladder: this Mac only, shared through git (in step, and with pushes
// waiting), a place that could not be reached, and connected to Bowrain with
// and without a team place.

import type {
  ContextSyncResult,
  ProjectSharing,
  ShareRungID,
  ShareView,
  SyncStatus,
} from "../../types/api";

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

/** Shared through a folder every machine mounts, with nothing waiting. */
export const FOLDER_IN_SYNC: ProjectSharing = {
  kind: "file",
  location: "/Volumes/team/kapi/fernwell",
  from: "recipe",
  status: status({ remote: { Kind: "file", Location: "/Volumes/team/kapi/fernwell" } }),
  workspace: WORKSPACE_DIR,
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

/** Connected to Bowrain, with no team place named under `context:`. */
export const BOWRAIN_WITHOUT_TEAM: ProjectSharing = {
  ...LOCAL_SHARING,
  venue_host: "app.bowrain.cloud",
  venue_url: "https://app.bowrain.cloud/acme/help",
};

/** Connected to Bowrain, with the context kept there under `context: {backend: bowrain}`. */
export const BOWRAIN_BACKEND: ProjectSharing = {
  kind: "bowrain",
  location: "https://app.bowrain.cloud/acme/help",
  from: "recipe",
  status: status({ remote: { Kind: "bowrain", Location: "https://app.bowrain.cloud/acme/help" } }),
  workspace: WORKSPACE_DIR,
  venue_host: "app.bowrain.cloud",
  venue_url: "https://app.bowrain.cloud/acme/help",
};

/** The Share page over one of the places above, with twelve rules in force. */
function view(sharing: ProjectSharing, over: Partial<ShareView> = {}): ShareView {
  const rung: ShareRungID = sharing.venue_host
    ? "bowrain"
    : sharing.kind !== "local"
      ? "team"
      : "machine";
  return { rung, sharing, rules: 12, bowrain_plugin: false, ...over };
}

/** On this Mac, with the kapi-bowrain plugin not installed. */
export const LOCAL_VIEW = view(LOCAL_SHARING);
/** A `context:` block the recipe check refuses, as the backend words it. */
export const BROKEN_PLACE_ERROR =
  'context.backend: "dropbox" is not a backend; use local, file, git, s3 or bowrain';
/** On this Mac, because the recipe's `context:` block cannot be read. */
export const BROKEN_PLACE_VIEW = view(LOCAL_SHARING, { error: BROKEN_PLACE_ERROR });
/** On this Mac, with the kapi-bowrain plugin installed. */
export const LOCAL_VIEW_WITH_PLUGIN = view(LOCAL_SHARING, { bowrain_plugin: true });
export const GIT_VIEW = view(GIT_IN_SYNC);
export const GIT_PUSHES_WAITING_VIEW = view(GIT_PUSHES_WAITING);
export const GIT_NEVER_SYNCED_VIEW = view(GIT_NEVER_SYNCED);
export const FOLDER_VIEW = view(FOLDER_IN_SYNC);
export const FOLDER_UNREACHABLE_VIEW = view(FOLDER_UNREACHABLE);
export const S3_VIEW = view(S3_IN_SYNC);
export const BOWRAIN_VIEW = view(BOWRAIN_CONNECTED, { bowrain_plugin: true });
export const BOWRAIN_WITHOUT_TEAM_VIEW = view(BOWRAIN_WITHOUT_TEAM, { bowrain_plugin: true });
export const BOWRAIN_BACKEND_VIEW = view(BOWRAIN_BACKEND, { bowrain_plugin: true });
/** A project whose path names a long folder, as a network share often does. */
export const FOLDER_LONG_PATH_VIEW = view({
  ...FOLDER_IN_SYNC,
  location: "/Volumes/team-shares/Engineering/Documentation/kapi/projects/fernwell/context",
});

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
