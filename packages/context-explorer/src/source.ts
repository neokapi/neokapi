// The data-source contract the context explorer consumes (Apache-2.0).
//
// One interface lets the SAME components run against both halves of the product:
//
//   • kapi (desktop) drives it against the LOCAL project — the host's
//     by-location governance resolution, its grouped context search, and the
//     project's own occurrence graph. Workspace, project and stream are pinned.
//   • Bowrain drives it against its REST API — the same questions with the
//     dimensions free, so a workspace answer spans projects.
//
// Methods a source cannot answer are omitted; the panes gate on the resolved
// capabilities and state the reach of what they show, rather than rendering an
// empty section that reads as "nothing here".

import type { Dimension, ScopeTuple } from "./scope";
import type {
  ContentAnswer,
  GovernanceAnswer,
  RelationSubject,
  RelationsAnswer,
  RungOption,
  ScopedQuery,
  SearchAnswer,
} from "./types";

/** A value that may be returned synchronously or as a promise. */
export type Awaitable<T> = T | Promise<T>;

/** What a source can answer, and how far it can see. */
export interface ExplorerCapabilities {
  /** Dimensions the reader may move. Everything else is pinned by the mount. */
  free: Dimension[];
  /**
   * How "what governs here" is answered. `resolved` means the source walks the
   * declared bindings for the exact point; `derived` means it aggregates from
   * what content has declared, so a point with no content is invisible.
   */
  governance: "resolved" | "derived";
  /** How far "how it relates" can see. `none` hides the pane. */
  relations: "none" | "project" | "workspace";
  /** True when the source can list a point's content. */
  content: boolean;
  /** True when the source answers by-content search. */
  search: boolean;
}

/**
 * The contract the explorer consumes. Implement `governs`; add the rest as the
 * backend gains them.
 */
export interface ContextDataSource {
  /**
   * Explicit capability flags. When omitted, capabilities are DERIVED from which
   * optional methods this source implements (see deriveCapabilities). Set this
   * to state a reach the method set cannot express — most importantly whether
   * `relates` sees one project or the whole workspace.
   */
  capabilities?: Partial<ExplorerCapabilities>;

  /** What governs at the tuple's point. Required. */
  governs(q: ScopedQuery): Awaitable<GovernanceAnswer>;

  /** What lives at the tuple's point. */
  lives?(q: ScopedQuery): Awaitable<ContentAnswer>;

  /** How a selection relates to the rest of the graph. */
  relates?(subject: RelationSubject, q: ScopedQuery): Awaitable<RelationsAnswer>;

  /** By-content search across every store the tuple's scope holds. */
  search?(query: string, q: ScopedQuery): Awaitable<SearchAnswer>;

  /**
   * The values one dimension can take under the given tuple — the filter bar's
   * options and the ladder's next rung. A source that omits it renders a free
   * dimension as a text field rather than a picker.
   */
  options?(scope: ScopeTuple, dimension: Dimension): Awaitable<RungOption[]>;
}

/** The capabilities a source with no optional methods advertises. */
export const MINIMAL_CAPABILITIES: ExplorerCapabilities = {
  free: [],
  governance: "resolved",
  relations: "none",
  content: false,
  search: false,
};

/**
 * Capabilities derived from the methods a source implements, overlaid with any
 * it declares. Derivation cannot tell a project-reach `relates` from a
 * workspace-reach one, so it assumes the narrower of the two: an answer that
 * understates its reach is honest, one that overstates it is a lie about the
 * graph.
 */
export function deriveCapabilities(source: ContextDataSource): ExplorerCapabilities {
  const has = (key: keyof ContextDataSource): boolean => typeof source[key] === "function";
  return {
    ...MINIMAL_CAPABILITIES,
    relations: has("relates") ? "project" : "none",
    content: has("lives"),
    search: has("search"),
    ...source.capabilities,
    free: source.capabilities?.free ?? [],
  };
}
