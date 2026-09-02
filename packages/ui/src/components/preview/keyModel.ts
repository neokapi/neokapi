// keyModel — the row and group model behind the keyed table preview.
//
// A catalog-keyvalue file (JSON, YAML, properties, ARB, …) is a set of units
// each addressed by a key path. Rendering one as a document produces a flat
// column of strings with nothing to say which string is which: a reviewer
// reading `messages.json` sees the French, and has to guess whether the second
// line is the button label or the error. The table shows the key beside the
// text, grouped the way the file nests, so `errors.network.timeout` sits under
// `errors` then `network`.
//
// Everything here is pure, so the grouping is unit-testable without a DOM. The
// presentational layer is KeyedTable.tsx.

import type { ContentNode, ContentTree, Run } from "./types";
import { entryKey } from "./renderDoc";

/** One unit of a keyed document: a block, its key path, and its content. */
export interface KeyedRow {
  /**
   * The block's id. This is the `data-block-id` the table puts on the row, so
   * selection and scroll-into-view work exactly as they do in the document
   * preview and a host needs no new plumbing.
   */
  id: string;
  /** The full key as the reader stated it ("errors.network.timeout"). */
  key: string;
  /** The key split into its segments (["errors", "network", "timeout"]). */
  path: string[];
  /** The last segment, shown in the key column under its group. */
  leaf: string;
  /** The source run sequence. */
  source: Run[];
  /** Per-locale target run sequences, keyed by locale. */
  targets: Record<string, Run[]>;
  /** The block's source locale, when it states one. */
  sourceLocale?: string;
  /** The originating node, for a host that wants its overlays or properties. */
  node: ContentNode;
}

/** A group of rows sharing a key prefix, with the deeper groups nested inside. */
export interface KeyGroup {
  /** The prefix segments this group stands for ([] for the root group). */
  path: string[];
  /** Rows whose key sits directly at this prefix. */
  rows: KeyedRow[];
  /** Groups one segment deeper. */
  groups: KeyGroup[];
}

/**
 * Split a key into the segments a reader nests it by.
 *
 * Three spellings reach here and all three mean the same nesting:
 *
 *   errors.network.timeout      JSON, YAML, properties, ARB
 *   /errors/network/timeout     a JSON pointer, which the JSON reader emits
 *                               under `useLeadingSlashOnKeyPath`
 *   $.errors.network.timeout    a JSONPath, whose "$" names the document
 *   items[0].label              an array element
 *
 * An index is kept as its own segment so a list of ten strings groups under one
 * `items` heading rather than ten headings of one row each. A key with no
 * separator is a single segment, which is what a flat properties file has.
 */
export function splitKeyPath(key: string): string[] {
  if (!key) return [];
  // A JSONPath root ("$.greeting") names the document, not a level of nesting.
  const rooted = key.startsWith("$.") ? key.slice(2) : key === "$" ? "" : key;
  const slashed = rooted.startsWith("/");
  const parts = (slashed ? rooted.slice(1) : rooted).split(slashed ? "/" : ".");
  const out: string[] = [];
  for (const part of parts) {
    if (part === "") continue;
    // "items[0]" is two segments; "items[0][1]" is three.
    const head = part.indexOf("[");
    if (head <= 0 || !part.endsWith("]")) {
      out.push(part);
      continue;
    }
    out.push(part.slice(0, head));
    for (const index of part.slice(head).matchAll(/\[([^\]]*)\]/g)) {
      out.push(index[1]);
    }
  }
  return out;
}

/** Every block in the tree, in document order. */
function eachBlock(nodes: ContentNode[] | undefined, visit: (node: ContentNode) => void): void {
  if (!nodes) return;
  for (const node of nodes) {
    if (node.kind === "block") visit(node);
    eachBlock(node.children, visit);
  }
}

/**
 * The rows of a keyed document, in document order.
 *
 * The key comes from `entryKey`, the same reading the document preview's entry
 * list uses: the block's `name` (which is where the JSON, YAML and properties
 * readers put the key path) or whichever property carries it. A block with no
 * key keeps its id as the key, so a unit is never unlabelled.
 */
export function keyedRows(tree: ContentTree): KeyedRow[] {
  const rows: KeyedRow[] = [];
  eachBlock(tree.root, (node) => {
    const key = entryKey(node) ?? node.id;
    const path = splitKeyPath(key);
    rows.push({
      id: node.id,
      key,
      path,
      leaf: path.length > 0 ? path[path.length - 1] : key,
      source: node.source ?? [],
      targets: node.targets ?? {},
      ...(node.sourceLocale ? { sourceLocale: node.sourceLocale } : {}),
      node,
    });
  });
  return rows;
}

/**
 * Group rows by their key nesting, keeping document order within every group
 * and ordering groups by where their first row appears.
 *
 * A file whose keys share no prefix (a flat properties file, an interchange
 * format's unit ids) produces one root group holding every row, which is the
 * table without headings.
 */
export function groupRows(rows: KeyedRow[]): KeyGroup {
  const root: KeyGroup = { path: [], rows: [], groups: [] };

  for (const row of rows) {
    // The last segment names the row; everything before it names its group.
    const prefix = row.path.slice(0, -1);
    let group = root;
    for (let depth = 0; depth < prefix.length; depth++) {
      const segment = prefix[depth];
      let child = group.groups.find((g) => g.path[depth] === segment);
      if (!child) {
        child = { path: [...prefix.slice(0, depth), segment], rows: [], groups: [] };
        group.groups.push(child);
      }
      group = child;
    }
    group.rows.push(row);
  }

  return root;
}

/** Every locale any row carries a target for, in first-seen order. */
export function rowLocales(rows: KeyedRow[]): string[] {
  const seen = new Set<string>();
  const order: string[] = [];
  for (const row of rows) {
    for (const locale of Object.keys(row.targets)) {
      if (seen.has(locale)) continue;
      seen.add(locale);
      order.push(locale);
    }
  }
  return order;
}

/** Total rows under a group, including every nested group. */
export function groupSize(group: KeyGroup): number {
  return group.rows.length + group.groups.reduce((n, g) => n + groupSize(g), 0);
}
