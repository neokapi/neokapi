// The context explorer, driven against the local project.
//
// The desktop holds one project on one stream, so workspace, project and stream
// are pinned and only collection, path, coordinate and language move. Everything
// the panes show is resolved by the Go backend through the same
// project.ResolveGovernanceFor and host.SearchContext a run and a `kapi check`
// go through, so the explorer cannot show a context the loop would not enforce.

import type {
  ContentAnswer,
  ContextDataSource,
  Dimension,
  ExplorerCapabilities,
  GovernanceAnswer,
  RelationSubject,
  RelationsAnswer,
  RungOption,
  ScopeTuple,
  ScopedQuery,
  SearchAnswer,
  SearchGroup,
  TermHit,
} from "@neokapi/context-explorer";
import { isBannedStatus } from "@neokapi/concept-ui";
import type { TermStatus } from "@neokapi/concept-ui";
import { call } from "../hooks/useApi";

/** The Wails surface the source reads through, injectable for tests. */
export interface ContextBackend {
  /**
   * The by-location answer for one file — the same resolution `kapi context
   * <path>` and the `context://` resource serve.
   */
  at(tabID: string, path: string, limit: number): Promise<AtResult | null>;
  governs(
    tabID: string,
    collection: string,
    path: string,
    limit: number,
  ): Promise<GovernsResult | null>;
  lives(
    tabID: string,
    collection: string,
    path: string,
    limit: number,
  ): Promise<LivesResult | null>;
  relates(
    tabID: string,
    kind: string,
    subject: string,
    limit: number,
  ): Promise<RelatesResult | null>;
  search(tabID: string, query: string, locale: string, limit: number): Promise<SearchResult | null>;
  options(tabID: string, dimension: string): Promise<OptionResult[] | null>;
}

interface PointResult {
  path?: string;
  profile?: string;
  channel?: string;
  collection?: string;
  ref?: string;
  default: boolean;
}

interface TermResult {
  text: string;
  locale: string;
  status: string;
  note?: string;
  validity?: { valid_from?: string; valid_to?: string };
}

interface ConceptResult {
  id: string;
  domain?: string;
  definition?: string;
  terms: TermResult[];
}

interface ProfileWindowResult {
  name: string;
  valid_from?: string;
  valid_to?: string;
  state: string;
}

/**
 * The by-location answer, verbatim from the retrieval surface. Its term rows
 * are already the shape the panes render, so nothing here re-projects them.
 */
interface AtResult {
  point: PointResult;
  scope: string;
  voice?: { name: string; source?: string; field?: string; guide?: string };
  terms?: TermHit[];
  terms_total?: number;
  profiles?: ProfileWindowResult[];
  notes?: string[];
}

interface GovernsResult {
  point: PointResult;
  voice?: { name: string; source?: string; field?: string; guide?: string };
  concepts: ConceptResult[];
  concepts_total: number;
  profiles: ProfileWindowResult[];
  notes: string[];
}

interface ItemResult {
  hash: string;
  block_id?: string;
  document?: string;
  collection?: string;
  locale?: string;
  text?: string;
}

interface LivesResult {
  point: PointResult;
  collections: Array<{ name: string; channel?: string; profile?: string; blocks: number }>;
  coverage: Array<{ locale: string; units: number; covered: number }>;
  items: ItemResult[];
  items_total: number;
  notes: string[];
}

/**
 * One recorded use of a term, as the backend reads it off the context graph:
 * one row per (block, term, language) with how often the term is used there,
 * and the passage where the block cache still holds it.
 */
interface UseResult {
  concept_id: string;
  term: string;
  term_locale?: string;
  content_key: string;
  block_id?: string;
  document?: string;
  collection?: string;
  locale?: string;
  occurrences: number;
  snippet?: string;
}

interface RelatesResult {
  subject: string;
  reach: string;
  occurrences: UseResult[];
  occurrences_total: number;
  blessings: Array<{
    unit: string;
    variant?: string;
    status?: string;
    target_hash?: string;
  }>;
  notes: string[];
}

interface SearchResult {
  answer: {
    query: string;
    scope: string;
    terms?: TermHit[];
    precedent?: Array<{
      entry_id: string;
      text: string;
      locale?: string;
      note?: string;
      discouraged?: string[];
    }>;
    profiles?: ProfileWindowResult[];
    notes?: string[];
  };
  content: ItemResult[];
}

interface OptionResult {
  value: string;
  label?: string;
  count?: number;
}

/** The production backend: each method dispatches to its Wails binding. */
export const wailsContextBackend: ContextBackend = {
  at: (tabID, path, limit) => call<AtResult>("ContextAt", tabID, path, limit),
  governs: (tabID, collection, path, limit) =>
    call<GovernsResult>("ContextGoverns", tabID, collection, path, limit),
  lives: (tabID, collection, path, limit) =>
    call<LivesResult>("ContextLives", tabID, collection, path, limit),
  relates: (tabID, kind, subject, limit) =>
    call<RelatesResult>("ContextRelates", tabID, kind, subject, limit),
  search: (tabID, query, locale, limit) =>
    call<SearchResult>("ContextSearch", tabID, query, locale, limit),
  options: (tabID, dimension) => call<OptionResult[]>("ContextOptions", tabID, dimension),
};

/** The dimensions a desktop tab lets the reader move. */
export const DESKTOP_FREE_DIMENSIONS: Dimension[] = ["collection", "path", "coordinate", "locale"];

const DESKTOP_CAPABILITIES: Partial<ExplorerCapabilities> = {
  free: DESKTOP_FREE_DIMENSIONS,
  governance: "resolved",
  // A local project holds its own subgraph only. Saying "workspace" here would
  // make "which projects use this concept" read as "only this one does".
  relations: "project",
  content: true,
  search: true,
};

/** A concept's terms, flattened into the term rows the governance pane renders. */
function conceptTerms(concepts: ConceptResult[]): TermHit[] {
  const hits: TermHit[] = [];
  for (const concept of concepts) {
    const preferred = concept.terms.find((term) => term.status === "preferred");
    for (const term of concept.terms) {
      const discouraged = isBannedStatus(term.status as TermStatus);
      hits.push({
        concept_id: concept.id,
        term: term.text,
        locale: term.locale,
        status: term.status,
        definition: concept.definition,
        domain: concept.domain,
        replacement: discouraged && preferred?.text !== term.text ? preferred?.text : undefined,
        discouraged,
        valid_from: term.validity?.valid_from,
        valid_to: term.validity?.valid_to,
      });
    }
  }
  return hits;
}

function item(row: ItemResult) {
  return {
    id: `${row.hash}:${row.locale ?? ""}`,
    document: row.document,
    collection: row.collection,
    locale: row.locale || undefined,
    text: row.text,
  };
}

/** The message a call outside the Wails runtime answers with. */
const NO_RUNTIME = "the desktop backend is not available";

function required<T>(value: T | null): T {
  if (value === null) throw new Error(NO_RUNTIME);
  return value;
}

/**
 * The explorer's data source for one open project tab.
 *
 * `backend` is injectable so the mapping can be exercised against a fake; the
 * production path always dispatches to Wails.
 */
export function createLocalContextSource(
  tabID: string,
  backend: ContextBackend = wailsContextBackend,
): ContextDataSource {
  const limitOf = (q: ScopedQuery) => q.limit ?? 25;

  return {
    capabilities: DESKTOP_CAPABILITIES,

    async governs(q: ScopedQuery): Promise<GovernanceAnswer> {
      // A file is the finest declared point and the form the retrieval surface
      // owns, so it answers there rather than here. A collection has no
      // location to resolve and takes the coarser path.
      if (q.scope.path) {
        const at = required(await backend.at(tabID, q.scope.path, limitOf(q)));
        return {
          point: at.point,
          scope: "project",
          voice: at.voice,
          terms: at.terms ?? [],
          terms_total: at.terms_total,
          profiles: at.profiles ?? [],
          notes: at.notes ?? [],
        };
      }
      const res = required(await backend.governs(tabID, q.scope.collection ?? "", "", limitOf(q)));
      return {
        point: res.point,
        scope: "project",
        voice: res.voice,
        terms: conceptTerms(res.concepts),
        terms_total: res.concepts_total,
        profiles: res.profiles,
        notes: res.notes ?? [],
      };
    },

    async lives(q: ScopedQuery): Promise<ContentAnswer> {
      const res = required(
        await backend.lives(tabID, q.scope.collection ?? "", q.scope.path ?? "", limitOf(q)),
      );
      const locale = q.scope.locale;
      return {
        point: res.point,
        scope: "project",
        collections: res.collections.map((c) => ({
          id: c.name,
          name: c.name,
          point: c.profile && c.channel ? `${c.profile}/${c.channel}` : c.channel,
          items: c.blocks,
        })),
        items: res.items.filter((row) => !locale || row.locale === locale).map(item),
        coverage: res.coverage
          .filter((row) => !locale || row.locale === locale)
          .map((row) => ({ locale: row.locale, units: row.units, covered: row.covered })),
        items_total: res.items_total,
        notes: res.notes ?? [],
      };
    },

    async relates(subject: RelationSubject, q: ScopedQuery): Promise<RelationsAnswer> {
      const res = required(
        await backend.relates(tabID, subject.kind, subject.label ?? subject.id, limitOf(q)),
      );
      return {
        subject,
        reach: "project",
        occurrences: res.occurrences.map((o) => ({
          concept_id: o.concept_id,
          term: o.term,
          term_locale: o.term_locale,
          block_id: o.block_id,
          document: o.document,
          collection: o.collection,
          locale: o.locale ?? "",
          // The graph records which term the block uses, not how the text
          // spells it; the term is the closest name the row has for the match.
          matched: o.term,
          snippet: o.snippet ?? "",
        })),
        occurrences_total: res.occurrences_total,
        // Cross-project reach is a workspace question; a local answer that
        // listed one project would read as a complete list of users.
        projects: [],
        blessings: res.blessings.map((b) => ({
          unit: b.unit,
          variant: b.variant,
          status: b.status,
          target_hash: b.target_hash,
        })),
        notes: res.notes ?? [],
      };
    },

    async search(query: string, q: ScopedQuery): Promise<SearchAnswer> {
      const res = required(await backend.search(tabID, query, q.scope.locale ?? "", limitOf(q)));
      const groups: SearchGroup[] = [];
      if (res.answer.terms?.length) groups.push({ kind: "terms", hits: res.answer.terms });
      if (res.answer.precedent?.length) {
        groups.push({ kind: "precedent", hits: res.answer.precedent });
      }
      if (res.content.length) groups.push({ kind: "content", hits: res.content.map(item) });
      if (res.answer.profiles?.length) {
        groups.push({ kind: "profiles", hits: res.answer.profiles });
      }
      return {
        query: res.answer.query,
        scope: "project",
        groups,
        notes: res.answer.notes ?? [],
      };
    },

    async options(_scope: ScopeTuple, dimension: Dimension): Promise<RungOption[]> {
      const res = await backend.options(tabID, dimension);
      return res ?? [];
    },
  };
}
