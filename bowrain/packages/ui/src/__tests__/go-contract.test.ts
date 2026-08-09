// Contract tests for the TypeScript mirrors of Go enumerations.
//
// Every predicate the UI answers about a status is only as correct as its copy
// of the Go vocabulary. A member added on the Go side and forgotten here is
// invisible: the union still compiles, the Record still type-checks, and the
// UI silently mis-buckets the new value. These tests read the Go source as text
// and assert set equality against the TypeScript declarations, so the drift
// fails here — naming the missing member and both files — instead of in a
// dashboard.
import { describe, it, expect } from "vite-plus/test";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const REPO_ROOT = join(dirname(fileURLToPath(import.meta.url)), "..", "..", "..", "..", "..");

/** Repo-relative path of a Go source file, read as text. */
function readRepoFile(relative: string): string {
  return readFileSync(join(REPO_ROOT, relative), "utf8");
}

// ── Extraction ──────────────────────────────────────────────────────────────

/**
 * Values of the Go constants declared with an explicit `<Type>` — the shape
 * `Name Type = "value"` inside a const block. Empty-string values (the zero
 * rung of a ladder) are kept: they are members too.
 */
function goConstValues(source: string, typeName: string): Set<string> {
  const re = new RegExp(`\\b\\w+\\s+${typeName}\\s*=\\s*"([^"]*)"`, "g");
  const out = new Set<string>();
  for (const m of source.matchAll(re)) out.add(m[1]);
  return out;
}

/** Go constant identifier → value, for the constants of one type. */
function goConstIdents(source: string, typeName: string): Map<string, string> {
  const re = new RegExp(`\\b(\\w+)\\s+${typeName}\\s*=\\s*"([^"]*)"`, "g");
  const out = new Map<string, string>();
  for (const m of source.matchAll(re)) out.set(m[1], m[2]);
  return out;
}

/**
 * The body of the literal `marker` opens. The opening delimiter is the LAST one
 * on the marker's own line, so a type in the declaration (`struct{}{`,
 * `ChangeSetStatus[] = [`) is not mistaken for the literal.
 */
function blockAfter(source: string, marker: string, open = "{", close = "}"): string {
  const start = source.indexOf(marker);
  if (start < 0) throw new Error(`marker not found: ${marker}`);
  const lineEnd = source.indexOf("\n", start);
  const line = source.slice(start, lineEnd < 0 ? source.length : lineEnd);
  const onLine = line.lastIndexOf(open);
  const from = onLine >= 0 ? start + onLine : source.indexOf(open, start);
  if (from < 0) throw new Error(`no ${open} after: ${marker}`);
  let depth = 0;
  for (let i = from; i < source.length; i++) {
    if (source[i] === open) depth++;
    else if (source[i] === close) {
      depth--;
      if (depth === 0) return source.slice(from + 1, i);
    }
  }
  throw new Error(`unbalanced ${open} after: ${marker}`);
}

/** The string literals of a TS union type alias `export type Name = ...;`. */
function tsUnionMembers(source: string, typeName: string): Set<string> {
  const decl = new RegExp(`export type ${typeName}\\s*=([\\s\\S]*?);`).exec(source);
  if (!decl) throw new Error(`TS type not found: ${typeName}`);
  const out = new Set<string>();
  for (const m of decl[1].matchAll(/"([^"]*)"/g)) out.add(m[1]);
  return out;
}

/** The string literals inside a TS declaration's first `[...]`. */
function tsArrayMembers(source: string, declName: string): Set<string> {
  const body = blockAfter(source, declName, "[", "]");
  const out = new Set<string>();
  for (const m of body.matchAll(/"([^"]*)"/g)) out.add(m[1]);
  return out;
}

/**
 * The keys of a TS object literal that follows `declName`. Quoted keys keep
 * their text; bare identifier keys are taken verbatim.
 */
function tsObjectKeys(source: string, declName: string): Set<string> {
  const body = blockAfter(source, declName);
  const out = new Set<string>();
  for (const m of body.matchAll(/(?:^|[,{\n])\s*(?:"([^"]+)"|([A-Za-z_$][\w$-]*))\s*:/g)) {
    out.add(m[1] ?? m[2]);
  }
  return out;
}

// ── Assertion ───────────────────────────────────────────────────────────────

/** Set equality with a failure that names the drifting members and both files. */
function expectSameMembers(
  what: string,
  go: { path: string; members: Set<string> },
  ts: { path: string; members: Set<string> },
) {
  const missing = [...go.members].filter((m) => !ts.members.has(m)).sort();
  const extra = [...ts.members].filter((m) => !go.members.has(m)).sort();
  const problems: string[] = [];
  if (missing.length > 0) {
    problems.push(
      `${what}: ${ts.path} is missing ${missing.map((m) => JSON.stringify(m)).join(", ")} — ` +
        `declared in ${go.path}`,
    );
  }
  if (extra.length > 0) {
    problems.push(
      `${what}: ${ts.path} declares ${extra.map((m) => JSON.stringify(m)).join(", ")}, ` +
        `absent from ${go.path}`,
    );
  }
  expect(problems.join("\n")).toBe("");
}

// ── Sources ─────────────────────────────────────────────────────────────────

const GO = {
  target: "core/model/target.go",
  term: "core/model/term.go",
  storeTypes: "bowrain/core/store/types.go",
  knowledgeTypes: "bowrain/knowledge/types.go",
  changeset: "bowrain/knowledge/changeset.go",
  task: "bowrain/store/task.go",
} as const;

const TS = {
  api: "bowrain/packages/ui/src/types/api.ts",
  brandGraph: "bowrain/packages/ui/src/types/brand-graph.ts",
  blockStatus: "bowrain/packages/ui/src/components/editor/blockStatus.ts",
  chip: "bowrain/packages/ui/src/components/OnBrandRateChip.tsx",
  atoms: "bowrain/packages/ui/src/brand-hub/shell/atoms.tsx",
} as const;

const src = {
  target: readRepoFile(GO.target),
  term: readRepoFile(GO.term),
  storeTypes: readRepoFile(GO.storeTypes),
  knowledgeTypes: readRepoFile(GO.knowledgeTypes),
  changeset: readRepoFile(GO.changeset),
  task: readRepoFile(GO.task),
  api: readRepoFile(TS.api),
  brandGraph: readRepoFile(TS.brandGraph),
  blockStatus: readRepoFile(TS.blockStatus),
  chip: readRepoFile(TS.chip),
  atoms: readRepoFile(TS.atoms),
};

// ── Tests ───────────────────────────────────────────────────────────────────

describe("TargetStatus mirrors model.TargetStatus", () => {
  const members = goConstValues(src.target, "TargetStatus");

  it("enumerates the whole review ladder, empty rung included", () => {
    expect(members.size).toBeGreaterThan(1);
    expectSameMembers(
      "TargetStatus",
      { path: GO.target, members },
      { path: TS.api, members: tsUnionMembers(src.api, "TargetStatus") },
    );
  });
});

describe("TermStatus mirrors model.TermStatus", () => {
  const members = goConstValues(src.term, "TermStatus");

  it("is enumerated by the brand-graph union", () => {
    expect(members.size).toBeGreaterThan(1);
    expectSameMembers(
      "TermStatus",
      { path: GO.term, members },
      { path: TS.brandGraph, members: tsUnionMembers(src.brandGraph, "TermStatus") },
    );
  });

  it("is coloured completely by the editor's term-status ladder", () => {
    expectSameMembers(
      "TermStatus colours",
      { path: GO.term, members },
      { path: TS.blockStatus, members: tsObjectKeys(src.blockStatus, "const colors:") },
    );
  });

  it("is badged completely by the brand hub", () => {
    expectSameMembers(
      "TermStatus badges",
      { path: GO.term, members },
      { path: TS.atoms, members: tsObjectKeys(src.atoms, "const TERM_STATUS_CLASS") },
    );
  });
});

describe("OnBrandBasis mirrors store.OnBrandBasis", () => {
  const members = goConstValues(src.storeTypes, "OnBrandBasis");

  it("enumerates every basis the server can send", () => {
    expect(members.size).toBe(4);
    expectSameMembers(
      "OnBrandBasis",
      { path: GO.storeTypes, members },
      { path: TS.api, members: tsUnionMembers(src.api, "OnBrandBasis") },
    );
  });

  it("has a tooltip explanation for every basis", () => {
    expectSameMembers(
      "OnBrandBasis explanations",
      { path: GO.storeTypes, members },
      { path: TS.chip, members: tsObjectKeys(src.chip, "const basisExplanations") },
    );
  });
});

describe("ChangeSetStatus mirrors knowledge.ChangeSetStatus", () => {
  const members = goConstValues(src.knowledgeTypes, "ChangeSetStatus");

  it("is enumerated by the union and the ordered list", () => {
    expect(members.size).toBeGreaterThan(1);
    expectSameMembers(
      "ChangeSetStatus",
      { path: GO.knowledgeTypes, members },
      { path: TS.brandGraph, members: tsUnionMembers(src.brandGraph, "ChangeSetStatus") },
    );
    expectSameMembers(
      "CHANGE_SET_STATUSES",
      { path: GO.knowledgeTypes, members },
      { path: TS.brandGraph, members: tsArrayMembers(src.brandGraph, "CHANGE_SET_STATUSES") },
    );
  });

  it("is badged and labelled completely by the brand hub", () => {
    expectSameMembers(
      "ChangeSetStatus badges",
      { path: GO.knowledgeTypes, members },
      { path: TS.atoms, members: tsObjectKeys(src.atoms, "const CHANGESET_STATUS_CLASS") },
    );
    expectSameMembers(
      "ChangeSetStatus labels",
      { path: GO.knowledgeTypes, members },
      { path: TS.atoms, members: tsObjectKeys(src.atoms, "const CHANGESET_STATUS_LABEL") },
    );
  });

  it("agrees on which statuses are terminal", () => {
    // Terminal = no outgoing edge, i.e. absent as a key of the Go transition
    // table. Reading the table rather than a hand-list is the point: superseded
    // became terminal without anyone editing a list of terminal statuses.
    const idents = goConstIdents(src.knowledgeTypes, "ChangeSetStatus");
    const table = blockAfter(src.changeset, "allowedChangeSetTransitions =");
    const withEdges = new Set<string>();
    for (const m of table.matchAll(/\n\t(ChangeSet\w+):\s*\{/g)) {
      const value = idents.get(m[1]);
      expect(value, `unknown status ${m[1]} keys the transition table`).toBeDefined();
      withEdges.add(value!);
    }
    expect(withEdges.size).toBeGreaterThan(0);
    const terminal = new Set([...members].filter((m) => !withEdges.has(m)));

    expectSameMembers(
      "TERMINAL_CHANGESET_STATUSES",
      { path: GO.changeset, members: terminal },
      {
        path: TS.brandGraph,
        members: tsArrayMembers(src.brandGraph, "TERMINAL_CHANGESET_STATUSES"),
      },
    );
  });
});

describe("TaskStatus mirrors store.TaskStatus", () => {
  it("enumerates every task lifecycle state", () => {
    const members = goConstValues(src.task, "TaskStatus");
    expect(members.size).toBeGreaterThan(1);
    expectSameMembers(
      "TaskStatus",
      { path: GO.task, members },
      { path: TS.api, members: tsUnionMembers(src.api, "TaskStatus") },
    );
  });
});
