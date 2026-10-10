// Build the context graph from the engine's own answers.
//
// Every part of the graph is one JSON answer: `kapi ls --stats` for the
// content, `kapi status` for the editions and their coverage, `kapi context
// <file>` for the point, the voice and the terms in force there, `kapi terms
// search` for the concepts, `kapi terms occurrences` for where each is used,
// `kapi memory stats` for the content memory, and `kapi context log` for the
// record. A command that fails leaves its part of the graph empty rather than
// failing the build, so the lens draws what the project has at that moment.

import type {
  Coverage,
  EdgeKind,
  GraphEdge,
  GraphModel,
  GraphNode,
  LineRunner,
  NodeKind,
} from "./types.ts";

// The browser build colours every stream, --json included.
// eslint-disable-next-line no-control-regex
const ANSI = /\x1b\[[0-9;]*[A-Za-z]/g;

/** Parse a command's JSON, or null when it printed none. */
export function parseJson<T>(out: string): T | null {
  const text = out.replace(ANSI, "").trim();
  if (!text) return null;
  try {
    return JSON.parse(text) as T;
  } catch {
    // A report may precede or follow the JSON; take the outermost braces.
    const a = text.indexOf("{");
    const b = text.lastIndexOf("}");
    if (a >= 0 && b > a) {
      try {
        return JSON.parse(text.slice(a, b + 1)) as T;
      } catch {
        return null;
      }
    }
    return null;
  }
}

interface LsAnswer {
  files?: { path: string; format?: string; blocks?: number; words?: number }[];
  blocks?: number;
  words?: number;
}

interface StatusAnswer {
  project?: string;
  source?: { total?: number };
  locales?: {
    locale: string;
    collection?: string;
    total?: number;
    pct?: Partial<Coverage>;
    shipState?: string;
    blocking?: string;
    failingChecks?: number;
    shippable?: boolean;
  }[];
  monolingual?: boolean;
}

interface ContextAnswer {
  point?: {
    path?: string;
    profile?: string;
    channel?: string;
    collection?: string;
    default?: boolean;
  };
  voice?: { name: string; source?: string; field?: string };
  terms?: { concept_id: string; term: string; status?: string; replacement?: string }[];
  rules?: unknown[];
  constraints?: { constraint?: { id?: string; statement?: string; kind?: string } }[];
  provenance?: { revision?: number; stale?: boolean };
}

interface TermsAnswer {
  concepts?: {
    id: string;
    definition?: string;
    domain?: string;
    terms?: { text: string; locale?: string; status?: string }[];
  }[];
}

interface OccurrencesAnswer {
  occurrences?: { document: string; block_id?: string }[] | null;
  blocks?: number;
}

interface MemoryAnswer {
  entries?: number;
  locale_pairs?: Record<string, number>;
}

interface LogAnswer {
  operations?: {
    id: string;
    kind: string;
    actor?: { kind?: string; name?: string };
    subject?: { kind?: string; text?: string };
    at?: string;
    status?: string;
    scope?: { level?: string };
  }[];
}

export interface BuildOptions {
  /** The file whose point the graph shows (default: the first file listed). */
  sourceFile?: string;
  /** Ask the engine where each concept is used (one command per concept). */
  occurrences?: boolean;
}

function pct(p?: Partial<Coverage>): Coverage {
  return {
    draft: Math.round(p?.draft ?? 0),
    established: Math.round(p?.established ?? 0),
    translated: Math.round(p?.translated ?? 0),
  };
}

/** The locales a record's subject names (a decision record is about editions). */
function localesIn(text: string, known: string[]): string[] {
  return known.filter((l) => new RegExp(`(^|[^A-Za-z-])${l}([^A-Za-z-]|$)`).test(text));
}

/** What a record is about, in two or three words: "import: voice profile", "20 established". */
export function recordLabel(kind: string, what: string, count: number): string {
  const past: Record<string, string> = {
    establish: "established",
    decide: "decided",
    import: "imported",
    note: "noted",
    keep: "kept",
    drop: "dropped",
    withdraw: "withdrawn",
  };
  if (count > 1) return `${count} ${past[kind] ?? kind}`;
  const lower = what.toLowerCase();
  const m = lower.match(/(\d+)\s+(decisions?|concepts?|entries|rules?)/);
  const short = lower.includes("voice profile")
    ? "voice profile"
    : m
      ? `${m[1]} ${m[2]}`
      : lower.includes("memory")
        ? "content memory"
        : lower.includes("term")
          ? "terms"
          : what.length > 22
            ? `${what.slice(0, 21)}…`
            : what;
  return `${kind}: ${short}`;
}

function shellQuote(s: string): string {
  return `'${s.replace(/'/g, "'\\''")}'`;
}

export async function buildContextGraph(
  run: LineRunner,
  opts: BuildOptions = {},
): Promise<GraphModel> {
  const json = async <T>(line: string): Promise<T | null> => {
    try {
      const { out } = await run(line);
      return parseJson<T>(out);
    } catch {
      return null;
    }
  };

  const nodes: GraphNode[] = [];
  const edges: GraphEdge[] = [];
  const node = (n: GraphNode) => {
    nodes.push(n);
    return n;
  };
  const edge = (from: string, to: string, kind: EdgeKind, extra: Partial<GraphEdge> = {}) => {
    edges.push({ id: `${kind}:${from}>${to}`, from, to, kind, ...extra });
  };

  const ls = await json<LsAnswer>("kapi ls --stats --json");
  const status = await json<StatusAnswer>("kapi status --json");
  const files = ls?.files ?? [];
  const sourceFile = opts.sourceFile ?? files[0]?.path;
  const ctx = sourceFile
    ? await json<ContextAnswer>(`kapi context ${shellQuote(sourceFile)} --json`)
    : null;
  const terms = await json<TermsAnswer>("kapi terms search '' --json");
  const memory = await json<MemoryAnswer>("kapi memory stats --json");
  const log = await json<LogAnswer>("kapi context log --json");

  // ── Content ──────────────────────────────────────────────────────────────
  const projectName = status?.project ?? "project";
  node({
    id: "project",
    kind: "project",
    label: projectName,
    sub: `${files.length} file${files.length === 1 ? "" : "s"}, ${ls?.blocks ?? 0} blocks`,
    detail: [
      ["files", String(files.length)],
      ["blocks", String(ls?.blocks ?? 0)],
      ["words", String(ls?.words ?? 0)],
    ],
  });

  const collectionName = ctx?.point?.collection ?? status?.locales?.[0]?.collection ?? "collection";
  node({ id: "collection", kind: "collection", label: collectionName, sub: "collection" });
  edge("project", "collection", "in");

  for (const f of files) {
    const id = `file:${f.path}`;
    node({
      id,
      kind: "source",
      label: f.path,
      sub: `${f.blocks ?? 0} blocks, ${f.words ?? 0} words`,
      count: f.blocks,
      detail: [
        ["format", f.format ?? ""],
        ["blocks", String(f.blocks ?? 0)],
        ["words", String(f.words ?? 0)],
      ],
    });
    edge("collection", id, "in");
  }

  const locales = (status?.locales ?? []).map((l) => l.locale);
  const sourceId = sourceFile ? `file:${sourceFile}` : undefined;
  for (const l of status?.locales ?? []) {
    const id = `edition:${l.locale}`;
    const cov = pct(l.pct);
    const state = l.shipState ?? (l.shippable ? "translated" : "withheld");
    node({
      id,
      kind: "edition",
      label: l.locale,
      sub: `${cov.translated}% translated, ${cov.established}% established`,
      status: state,
      pct: cov,
      count: l.total,
      detail: [
        ["ship state", state],
        ["translated", `${cov.translated}%`],
        ["established", `${cov.established}%`],
        ...(l.blocking ? ([["blocked on", l.blocking]] as [string, string][]) : []),
        ...(l.failingChecks
          ? ([["failing checks", String(l.failingChecks)]] as [string, string][])
          : []),
      ],
    });
    if (sourceId) edge(sourceId, id, "edition", { label: l.locale });
  }

  // ── The point ────────────────────────────────────────────────────────────
  const p = ctx?.point;
  const pointLabel =
    p && !p.default && (p.profile || p.channel)
      ? [p.profile, p.channel].filter(Boolean).join(" / ")
      : "project default";
  node({
    id: "point",
    kind: "point",
    label: pointLabel,
    sub: sourceFile ? `where ${sourceFile} sits` : "where the content sits",
    detail: [
      ["profile", p?.profile ?? "(default)"],
      ["channel", p?.channel ?? "(default)"],
      ["collection", p?.collection ?? collectionName],
    ],
  });
  if (sourceId) edge(sourceId, "point", "at");

  // ── Context in force at the point ────────────────────────────────────────
  if (ctx?.voice) {
    node({
      id: "voice",
      kind: "voice",
      label: ctx.voice.name,
      sub: `voice profile, ${ctx.voice.source ?? "store"}`,
      count: ctx.constraints?.length ?? 0,
      detail: [
        ["source", ctx.voice.source ?? ""],
        ["bound by", ctx.voice.field ?? ""],
        ["pattern rules", String(ctx.constraints?.length ?? 0)],
        ...(ctx.constraints ?? [])
          .slice(0, 3)
          .map((c) => ["rule", c.constraint?.statement ?? ""] as [string, string]),
      ],
    });
    edge("point", "voice", "governs");
  }

  const concepts = terms?.concepts ?? [];
  const statusByConcept = new Map<string, { status?: string; replacement?: string }>();
  for (const t of ctx?.terms ?? []) {
    if (!statusByConcept.has(t.concept_id) || t.status === "forbidden") {
      statusByConcept.set(t.concept_id, { status: t.status, replacement: t.replacement });
    }
  }
  if (concepts.length > 0) {
    node({
      id: "terms",
      kind: "terms",
      label: "Terms",
      sub: `${concepts.length} concept${concepts.length === 1 ? "" : "s"}, ${ctx?.rules?.length ?? 0} word rules`,
      count: concepts.length,
    });
    edge("point", "terms", "applies");
  }
  for (const c of concepts) {
    const forms = (c.terms ?? []).map((t) => t.text);
    const head = forms[0] ?? c.id;
    const st = statusByConcept.get(c.id);
    const stat = st?.status ?? c.terms?.[0]?.status ?? "preferred";
    const id = `concept:${c.id}`;
    node({
      id,
      kind: "concept",
      label: head,
      sub: st?.replacement ? `say ${st.replacement}` : (c.definition ?? forms.slice(1).join(", ")),
      status: stat,
      detail: [
        ["status", stat],
        ["forms", forms.join(", ")],
        ...(c.definition ? ([["definition", c.definition]] as [string, string][]) : []),
        ...(st?.replacement ? ([["say instead", st.replacement]] as [string, string][]) : []),
      ],
    });
    edge("terms", id, "has");
  }

  // Where each concept is used, once the engine has read the content.
  const stale = ctx?.provenance?.stale ?? true;
  if (opts.occurrences !== false && !stale && sourceId) {
    for (const c of concepts) {
      const head = c.terms?.[0]?.text;
      if (!head) continue;
      const occ = await json<OccurrencesAnswer>(
        `kapi terms occurrences ${shellQuote(head)} --json`,
      );
      const n = occ?.blocks ?? occ?.occurrences?.length ?? 0;
      if (n > 0) {
        edge(`concept:${c.id}`, sourceId, "uses", { label: `${n}`, weight: n });
        const cn = nodes.find((x) => x.id === `concept:${c.id}`);
        if (cn) cn.detail?.push(["used in", `${n} block${n === 1 ? "" : "s"}`]);
      }
    }
  }

  if (memory && (memory.entries ?? 0) > 0) {
    const pairs = memory.locale_pairs ?? {};
    node({
      id: "memory",
      kind: "memory",
      label: "Content memory",
      sub: `${memory.entries} entr${memory.entries === 1 ? "y" : "ies"}`,
      count: memory.entries,
      detail: Object.entries(pairs).map(([l, n]) => [l, `${n}`] as [string, string]),
    });
    edge("point", "memory", "applies");
    for (const l of locales) {
      const n = pairs[l] ?? 0;
      if (n > 0) edge("memory", `edition:${l}`, "recalls", { label: `${n}`, weight: n });
    }
  }

  // ── The record ───────────────────────────────────────────────────────────
  const ops = log?.operations ?? [];
  const groups = new Map<
    string,
    { kind: string; text: string; count: number; at: string; actor: string; status: string }
  >();
  for (const op of ops) {
    const text = op.subject?.text ?? op.subject?.kind ?? "";
    const key = `${op.kind}:${text}`;
    const g = groups.get(key);
    if (g) {
      g.count++;
      if ((op.at ?? "") > g.at) g.at = op.at ?? g.at;
    } else {
      groups.set(key, {
        kind: op.kind,
        text,
        count: 1,
        at: op.at ?? "",
        actor: op.actor?.name ?? op.actor?.kind ?? "",
        status: op.status ?? "",
      });
    }
  }
  const sorted = [...groups.values()].sort((a, b) => a.at.localeCompare(b.at));
  sorted.forEach((g, i) => {
    const id = `record:${i}`;
    const what = g.text || g.kind;
    node({
      id,
      kind: "record",
      label: recordLabel(g.kind, what, g.count),
      sub: what,
      count: g.count,
      status: g.status,
      detail: [
        ["what", what],
        ["by", g.actor],
        ["when", g.at.replace("T", " ").slice(0, 16)],
        ["status", g.status],
      ],
    });
    const lower = what.toLowerCase();
    const targets: string[] = [];
    if (lower.includes("voice") && nodes.some((n) => n.id === "voice")) targets.push("voice");
    if (
      (lower.includes("concept") || lower.includes("term")) &&
      nodes.some((n) => n.id === "terms")
    )
      targets.push("terms");
    if (lower.includes("memory") && nodes.some((n) => n.id === "memory")) targets.push("memory");
    for (const l of localesIn(what, locales)) targets.push(`edition:${l}`);
    if (
      targets.length === 0 &&
      (lower.includes("decision") || g.kind === "establish" || g.kind === "decide")
    ) {
      for (const l of locales) targets.push(`edition:${l}`);
    }
    if (targets.length === 0) targets.push("project");
    for (const t of targets) edge(id, t, "records");
  });

  // The decisions a person made on blocks are recorded beside the context
  // operations, and the log of those leaves them out. They show as the share
  // of each edition that is established, so the record row carries one entry
  // per edition that holds any, counted from that share.
  let decided = 0;
  for (const l of status?.locales ?? []) {
    const blocks = Math.round((pct(l.pct).established / 100) * (l.total ?? 0));
    if (blocks <= 0) continue;
    decided += blocks;
    const id = `decisions:${l.locale}`;
    node({
      id,
      kind: "record",
      label: `${blocks} established`,
      sub: `${l.locale} edition, decided by a person`,
      count: blocks,
      status: "established",
      detail: [
        ["edition", l.locale],
        ["blocks", `${blocks} of ${l.total ?? 0}`],
        ["status", "established"],
      ],
    });
    edge(id, `edition:${l.locale}`, "records");
  }

  const n = (kind: NodeKind) => nodes.filter((x) => x.kind === kind).length;
  const parts = [
    `${ls?.blocks ?? 0} source blocks`,
    `${n("edition")} edition${n("edition") === 1 ? "" : "s"}`,
    `${n("concept")} concept${n("concept") === 1 ? "" : "s"}`,
    `${memory?.entries ?? 0} memory entries`,
    `${ops.length} recorded operation${ops.length === 1 ? "" : "s"}`,
    ...(decided > 0 ? [`${decided} block decision${decided === 1 ? "" : "s"}`] : []),
  ];
  return {
    nodes,
    edges,
    revision: ctx?.provenance?.revision,
    stale,
    summary: parts.join(", "),
  };
}
