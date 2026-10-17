// The workspace home's context: each project's standing, the rules a person
// applied to every project, and the project rows that carry both.
//
// Acme Help Center has a claude-code session working now with four new items;
// KapiMart has a codex session that disagrees with an earlier rule, so it
// needs a person; Field Guide is quiet.

import type {
  ContextProjectStanding,
  WorkspaceHome,
  WorkspaceProject,
  WorkspaceRule,
} from "../../types/api";

const hour = 3_600_000;
const ago = (hours: number) => new Date(Date.now() - hours * hour).toISOString();

export const LOCATION = "/fakehome/.local/share/kapi/workspaces/default";

export function standing(
  over: Partial<ContextProjectStanding> & { project: string },
): ContextProjectStanding {
  return { rules: 0, suggested: 0, conflicts: 0, terms: 0, new: 0, ...over };
}

/** A claude-code session is working in Acme Help Center right now. */
export const ACME: ContextProjectStanding = standing({
  project: "prj_acmehelp",
  project_name: "Acme Help Center",
  rules: 23,
  suggested: 4,
  terms: 142,
  new: 4,
  since: ago(30),
  latest: {
    actor: { kind: "agent", name: "claude-code", session: "s-acme-2" },
    at: ago(0.01),
    working: true,
    recorded: 4,
    corrected: 0,
    new_by_theme: { names: 3, words: 1 },
  },
});

/** A codex session recorded the other side of a kept rule in KapiMart. */
export const KAPIMART: ContextProjectStanding = standing({
  project: "prj_kapimart",
  project_name: "KapiMart",
  rules: 31,
  suggested: 3,
  conflicts: 1,
  terms: 100,
  new: 2,
  since: ago(80),
  latest: {
    actor: { kind: "agent", name: "codex", session: "s-kapimart-2" },
    at: ago(3),
    working: false,
    recorded: 2,
    corrected: 0,
    new_by_theme: { names: 1, words: 1 },
  },
});

/** Field Guide holds rules and nothing new. */
export const FIELD_GUIDE: ContextProjectStanding = standing({
  project: "prj_fieldguide",
  project_name: "Field Guide",
  rules: 7,
  terms: 18,
  since: ago(20),
  latest: {
    actor: { kind: "person", session: "day-1" },
    at: ago(26),
    working: false,
    recorded: 0,
    corrected: 2,
  },
});

/** A project nobody has looked at: everything it holds is new. */
export const HANDBOOK: ContextProjectStanding = standing({
  project: "prj_handbook",
  project_name: "Handbook",
  rules: 0,
  suggested: 4,
  new: 4,
  latest: {
    actor: { kind: "agent", name: "cursor", session: "s-handbook-1" },
    at: ago(6),
    working: false,
    recorded: 4,
    corrected: 0,
    new_by_theme: { names: 2, words: 1, writing: 1 },
  },
});

/** One project, with news and a session working now. */
export const ONE_WORKING: ContextProjectStanding[] = [ACME];

/** Several projects; one needs a person. */
export const SEVERAL: ContextProjectStanding[] = [FIELD_GUIDE, KAPIMART, ACME, HANDBOOK];

/** Projects that hold nothing yet. */
export const NOTHING: ContextProjectStanding[] = [
  standing({ project: "prj_acmehelp", project_name: "Acme Help Center" }),
];

/** Two rules a person applied to every project. */
export const WORKSPACE_RULES: WorkspaceRule[] = [
  {
    id: "0nk2workspace0000000001",
    short: "0nk2worksp",
    sentence: "Write sign in, not log in",
    subject: { kind: "term", term: { term: "log in", replacement: "sign in" } },
    project_key: "prj_acmehelp",
    project_name: "Acme Help Center",
    at: "2026-09-12T10:04:00Z",
    how: ["kept by you"],
  },
  {
    id: "0nk2workspace0000000002",
    short: "0nk2worksq",
    sentence: "Address the reader as you",
    subject: { kind: "voice", text: "Address the reader as you" },
    project_key: "prj_kapimart",
    project_name: "KapiMart",
    at: "2026-10-02T15:30:00Z",
    how: ["merged in #412"],
  },
];

function checkout(dir: string) {
  return { path: `/fakehome/src/${dir}`, recipe: `/fakehome/src/${dir}/kapi.yaml`, missing: false };
}

const ACME_ROW: WorkspaceProject = {
  key: "prj_acmehelp",
  name: "Acme Help Center",
  last_active: ago(0.05),
  checkouts: [checkout("acme-help")],
  context: ACME,
  voice: "Fernwell",
  sharing: { kind: "local", from: "default", workspace: LOCATION },
};

const KAPIMART_ROW: WorkspaceProject = {
  key: "prj_kapimart",
  name: "KapiMart",
  last_active: ago(3),
  checkouts: [checkout("kapimart"), checkout("kapimart-release")],
  context: KAPIMART,
  voice: "kapimart-store",
  sharing: {
    kind: "git",
    location: "origin refs/kapi/context",
    from: "recipe",
    workspace: LOCATION,
  },
};

const FIELD_GUIDE_ROW: WorkspaceProject = {
  key: "prj_fieldguide",
  name: "Field Guide",
  last_active: ago(26),
  checkouts: [],
  context: FIELD_GUIDE,
  sharing: {
    kind: "s3",
    location: "s3://acme-context/field-guide",
    from: "recipe",
    workspace: LOCATION,
    venue_host: "app.bowrain.cloud",
  },
};

const HANDBOOK_ROW: WorkspaceProject = {
  key: "prj_handbook",
  name: "Handbook",
  last_active: ago(6),
  checkouts: [checkout("handbook")],
  context: HANDBOOK,
  sharing: { kind: "file", location: "/Volumes/team/context", from: "recipe", workspace: LOCATION },
};

export function workspaceOf(projects: WorkspaceProject[], readOnly = false): WorkspaceHome {
  return { location: LOCATION, read_only: readOnly, projects };
}

/** The rows of the one-project home. */
export const ONE_PROJECT_ROWS: WorkspaceProject[] = [ACME_ROW];

/** The rows of the several-project home. */
export const SEVERAL_ROWS: WorkspaceProject[] = [
  ACME_ROW,
  KAPIMART_ROW,
  FIELD_GUIDE_ROW,
  HANDBOOK_ROW,
];
