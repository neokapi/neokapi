// The Map of Fernwell, a studio-software project, in its three states: no
// content files yet, suggestions waiting for a person, and every suggestion
// kept. The suggestion rows are the Fernwell digest's (fernwell.ts), so a
// row's Decide lands on the same item in the Decide view.

import type { MapFileRow, MapSuggestionRow, MapSummary } from "../../types";
import { FERNWELL_PROJECT, FERNWELL_SUGGESTED, LOGIN_CONFLICT } from "./fernwell";

const minute = 60_000;
const ago = (minutes: number) => new Date(Date.now() - minutes * minute).toISOString();

const conflict = LOGIN_CONFLICT;

/** The digest's disagreement, then its suggestions, as MapSummary lists them. */
const SUGGESTIONS: MapSuggestionRow[] = [
  { group: "conflict", item: conflict.sides[0], conflict, blocks: 2, files: 2 },
  ...FERNWELL_SUGGESTED.flatMap((theme, ti) =>
    theme.map((item): MapSuggestionRow => ({
      group: "suggested",
      item,
      blocks: 1 + ti,
      files: 1,
    })),
  ),
];

const FILES: MapFileRow[] = [
  {
    path: "/work/fernwell/docs/billing.md",
    relative: "docs/billing.md",
    format: "markdown",
    collection: "docs",
    blocks: 42,
    findings: 3,
  },
  {
    path: "/work/fernwell/app/strings/en.json",
    relative: "app/strings/en.json",
    format: "json",
    collection: "app",
    blocks: 118,
    findings: 1,
  },
  {
    path: "/work/fernwell/docs/account.md",
    relative: "docs/account.md",
    format: "markdown",
    collection: "docs",
    blocks: 27,
    findings: 0,
  },
  {
    path: "/work/fernwell/docs/getting-started/recording-your-first-take-with-the-studio-console.md",
    relative: "docs/getting-started/recording-your-first-take-with-the-studio-console.md",
    format: "markdown",
    collection: "docs",
    blocks: 64,
    findings: 0,
  },
];

const base = {
  project_key: FERNWELL_PROJECT,
  target_languages: ["de", "nb"],
  read_at: ago(2),
  checked_at: ago(1),
};

/** A project whose collections match no file yet. */
export const MAP_EMPTY: MapSummary = {
  ...base,
  files: 0,
  blocks: 0,
  rules: 0,
  findings: null,
  checked_at: undefined,
  read_files: 0,
  suggested: 0,
  conflicts: 0,
  awaiting: 0,
  file_rows: [],
  suggestions: [],
};

/** An assistant noticed things, and one disagreement waits for a person. */
export const MAP_PENDING: MapSummary = {
  ...base,
  files: FILES.length,
  blocks: FILES.reduce((n, f) => n + f.blocks, 0),
  rules: 23,
  findings: FILES.reduce((n, f) => n + f.findings, 0),
  read_files: FILES.length,
  suggested: SUGGESTIONS.length - 1,
  conflicts: 1,
  awaiting: SUGGESTIONS.length,
  file_rows: FILES,
  suggestions: SUGGESTIONS,
};

/** Every suggestion kept: the rules hold, and the files pass. */
export const MAP_ALL_KEPT: MapSummary = {
  ...base,
  files: FILES.length,
  blocks: FILES.reduce((n, f) => n + f.blocks, 0),
  rules: 28,
  findings: 0,
  read_files: FILES.length,
  suggested: 0,
  conflicts: 0,
  awaiting: 0,
  file_rows: [...FILES]
    .map((f) => ({ ...f, findings: 0 }))
    .sort((a, b) => a.relative.localeCompare(b.relative)),
  suggestions: [],
};

/** Suggestions and no disagreement: Decide carries the one number. */
export const MAP_SUGGESTED_ONLY: MapSummary = {
  ...MAP_PENDING,
  suggested: SUGGESTIONS.length - 1,
  conflicts: 0,
  awaiting: SUGGESTIONS.length - 1,
  suggestions: SUGGESTIONS.slice(1),
};

/** One file no reader opens, so kapi read fewer files than the project has. */
export const MAP_PARTLY_READ: MapSummary = {
  ...MAP_ALL_KEPT,
  read_files: FILES.length - 1,
};

/** A source-only project, never checked in this session. */
export const MAP_SOURCE_ONLY_UNCHECKED: MapSummary = {
  ...MAP_ALL_KEPT,
  target_languages: [],
  findings: null,
  checked_at: undefined,
  file_rows: MAP_ALL_KEPT.file_rows,
};
