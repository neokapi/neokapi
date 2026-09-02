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
 * The json field names a Go struct serializes: the tag's first component, with
 * `-` (never serialized) and untagged fields left out. An untagged field is a
 * finding in itself — it serializes under its Go identifier — so the caller
 * asserts the count it expects rather than trusting the absence.
 */
function goStructJSONFields(source: string, typeName: string): Set<string> {
  const body = blockAfter(source, `type ${typeName} struct`);
  const out = new Set<string>();
  for (const m of body.matchAll(/`json:"([^"]*)"/g)) {
    const name = m[1].split(",")[0];
    if (name && name !== "-") out.add(name);
  }
  return out;
}

/**
 * The property names an `interface Name { … }` declares, top level only —
 * a nested object literal's own keys belong to that literal, not the interface.
 */
function tsInterfaceFields(source: string, typeName: string): Set<string> {
  const body = blockAfter(source, `interface ${typeName} {`)
    .replace(/\/\*[\s\S]*?\*\//g, "")
    .replace(/\/\/.*$/gm, "");
  const out = new Set<string>();
  let depth = 0;
  for (const line of body.split("\n")) {
    if (depth === 0) {
      const m = /^\s*(?:readonly\s+)?(?:"([^"]+)"|([A-Za-z_$][\w$]*))\??\s*:/.exec(line);
      if (m) out.add(m[1] ?? m[2]);
    }
    depth += (line.match(/[{[]/g) ?? []).length - (line.match(/[}\]]/g) ?? []).length;
  }
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
  connector: "core/venue/connector/connector.go",
  handlersConnector: "bowrain/server/handlers_connector.go",
  handlersActivity: "bowrain/server/handlers_activity.go",
  handlersBilling: "bowrain/server/handlers_billing.go",
  handlersCheck: "bowrain/server/handlers_check.go",
  finding: "core/check/finding.go",
  handlersConcepts: "bowrain/server/handlers_concepts.go",
} as const;

const TS = {
  api: "bowrain/packages/ui/src/types/api.ts",
  voiceGraph: "bowrain/packages/ui/src/types/brand-graph.ts",
  blockStatus: "bowrain/packages/ui/src/components/editor/blockStatus.ts",
  chip: "bowrain/packages/ui/src/components/ComplianceRateChip.tsx",
  atoms: "bowrain/packages/ui/src/context-hub/shell/atoms.tsx",
  restAdapter: "bowrain/packages/ui/src/api/rest-adapter.ts",
  voiceTypes: "bowrain/packages/ui/src/voice/types.ts",
  apiError: "bowrain/packages/ui/src/errors/ApiError.ts",
} as const;

const src = {
  target: readRepoFile(GO.target),
  term: readRepoFile(GO.term),
  storeTypes: readRepoFile(GO.storeTypes),
  knowledgeTypes: readRepoFile(GO.knowledgeTypes),
  changeset: readRepoFile(GO.changeset),
  task: readRepoFile(GO.task),
  connector: readRepoFile(GO.connector),
  handlersConnector: readRepoFile(GO.handlersConnector),
  handlersActivity: readRepoFile(GO.handlersActivity),
  handlersBilling: readRepoFile(GO.handlersBilling),
  handlersCheck: readRepoFile(GO.handlersCheck),
  finding: readRepoFile(GO.finding),
  handlersConcepts: readRepoFile(GO.handlersConcepts),
  api: readRepoFile(TS.api),
  voiceGraph: readRepoFile(TS.voiceGraph),
  blockStatus: readRepoFile(TS.blockStatus),
  chip: readRepoFile(TS.chip),
  atoms: readRepoFile(TS.atoms),
  restAdapter: readRepoFile(TS.restAdapter),
  voiceTypes: readRepoFile(TS.voiceTypes),
  apiError: readRepoFile(TS.apiError),
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
      { path: TS.voiceGraph, members: tsUnionMembers(src.voiceGraph, "TermStatus") },
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

describe("ComplianceBasis mirrors store.ComplianceBasis", () => {
  const members = goConstValues(src.storeTypes, "ComplianceBasis");

  it("enumerates every basis the server can send", () => {
    expect(members.size).toBe(4);
    expectSameMembers(
      "ComplianceBasis",
      { path: GO.storeTypes, members },
      { path: TS.api, members: tsUnionMembers(src.api, "ComplianceBasis") },
    );
  });

  it("has a tooltip explanation for every basis", () => {
    expectSameMembers(
      "ComplianceBasis explanations",
      { path: GO.storeTypes, members },
      { path: TS.chip, members: tsObjectKeys(src.chip, "const basisExplanations") },
    );
  });
});

describe("TermCompliance mirrors store.TermCompliance", () => {
  const members = goConstValues(src.storeTypes, "TermCompliance");

  it("enumerates every verdict the review queue can receive, unchecked included", () => {
    // Three rungs, and the empty one is a member: "not checked" is not
    // "compliant", and a union that dropped it would let the queue read an
    // ungoverned target as a cleared one.
    expect(members.size).toBe(3);
    expect(members.has("")).toBe(true);
    expectSameMembers(
      "TermCompliance",
      { path: GO.storeTypes, members },
      { path: TS.api, members: tsUnionMembers(src.api, "TermCompliance") },
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
      { path: TS.voiceGraph, members: tsUnionMembers(src.voiceGraph, "ChangeSetStatus") },
    );
    expectSameMembers(
      "CHANGE_SET_STATUSES",
      { path: GO.knowledgeTypes, members },
      { path: TS.voiceGraph, members: tsArrayMembers(src.voiceGraph, "CHANGE_SET_STATUSES") },
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
        path: TS.voiceGraph,
        members: tsArrayMembers(src.voiceGraph, "TERMINAL_CHANGESET_STATUSES"),
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

// ── Response shapes ─────────────────────────────────────────────────────────
//
// The tests above catch a member the UI forgot; these catch a FIELD it forgot.
// A response the server names as a struct and the adapter names as an interface
// drifts silently otherwise: a key added server-side still type-checks here, and
// a key renamed leaves the reader holding `undefined` rather than a failure.

describe("SyncStatus mirrors connector.SyncStatus", () => {
  it("is the DTO the adapter normalises, field for field", () => {
    const members = goStructJSONFields(src.connector, "SyncStatus");
    // Every field tagged: an untagged one serializes under its Go identifier,
    // which is exactly the PascalCase leak this struct used to have.
    expect(members.size).toBe(8);
    expectSameMembers(
      "SyncStatus",
      { path: GO.connector, members },
      {
        path: TS.restAdapter,
        members: tsInterfaceFields(src.restAdapter, "ConnectorSyncStatusDTO"),
      },
    );
  });
});

describe("ConnectorStatusBatchResponse mirrors the batch read", () => {
  it("names the same two halves the adapter reads", () => {
    expectSameMembers(
      "ConnectorStatusBatchResponse",
      {
        path: GO.handlersConnector,
        members: goStructJSONFields(src.handlersConnector, "ConnectorStatusBatchResponse"),
      },
      { path: TS.api, members: tsInterfaceFields(src.api, "ConnectorStatusBatch") },
    );
  });
});

describe("ActivityListResponse mirrors the activity page", () => {
  it("declares every field the feed can receive", () => {
    expectSameMembers(
      "ActivityListResponse",
      {
        path: GO.handlersActivity,
        members: goStructJSONFields(src.handlersActivity, "ActivityListResponse"),
      },
      { path: TS.api, members: tsInterfaceFields(src.api, "ActivityPage") },
    );
  });
});

describe("TaskResult mirrors the task page", () => {
  it("declares every field the board can receive", () => {
    expectSameMembers(
      "TaskResult",
      { path: GO.task, members: goStructJSONFields(src.task, "TaskResult") },
      { path: TS.api, members: tsInterfaceFields(src.api, "TaskPage") },
    );
  });
});

describe("BillingUsageResponse mirrors the credit ledger page", () => {
  it("declares the window totals and the page together", () => {
    expectSameMembers(
      "BillingUsageResponse",
      {
        path: GO.handlersBilling,
        members: goStructJSONFields(src.handlersBilling, "BillingUsageResponse"),
      },
      { path: TS.api, members: tsInterfaceFields(src.api, "CreditLedgerPage") },
    );
  });
});

describe("VoiceFinding mirrors check.Finding", () => {
  // The two fields this caught: the grouping field is `category`, not
  // `dimension` (nothing on the wire ever carried `dimension`, so the findings
  // list rendered an empty chip), and `position` is a run range, not a
  // {start,end} character pair.
  it("declares the finding the profile check and stored scores emit", () => {
    expectSameMembers(
      "check.Finding",
      { path: GO.finding, members: goStructJSONFields(src.finding, "Finding") },
      { path: TS.voiceTypes, members: tsInterfaceFields(src.voiceTypes, "VoiceFinding") },
    );
  });
});

describe("CheckIssueResponse mirrors the TS finding type", () => {
  it("declares everything the check endpoints report about a finding", () => {
    expectSameMembers(
      "CheckIssueResponse",
      {
        path: GO.handlersCheck,
        members: goStructJSONFields(src.handlersCheck, "CheckIssueResponse"),
      },
      { path: TS.api, members: tsInterfaceFields(src.api, "CheckIssue") },
    );
  });
});

describe("the governed refusal is one sentence, written once", () => {
  // The desktop refuses a governed concept delete with no server to ask, so it
  // raises the envelope itself. Two copies of the wording is one that drifts —
  // and the drift is invisible, because both sides still produce *a* refusal.
  const goEnvelope = blockAfter(src.handlersConcepts, "func conceptGovernedConflict");
  const goStrings = new Set([...goEnvelope.matchAll(/"([^"]{20,})"/g)].map((m) => m[1]));

  const tsStrings = new Set(
    [...src.apiError.matchAll(/GOVERNED_REFUSAL_(?:ERROR|HINT) =\s*\n?\s*"([^"]+)"/g)].map(
      (m) => m[1],
    ),
  );

  it("uses the error and hint the Go handler writes", () => {
    expect(tsStrings.size).toBe(2);
    expectSameMembers(
      "governed refusal",
      { path: GO.handlersConcepts, members: goStrings },
      { path: TS.apiError, members: tsStrings },
    );
  });
});
