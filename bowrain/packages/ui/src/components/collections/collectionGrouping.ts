import type { CollectionTranslationStats } from "../../types/api";

/**
 * Grouping a project's collections by where they sit in context space.
 *
 * A collection rollup carries the coordinates its collection row persists —
 * `{product: "bowrain", channel: "app"}` and the like. The axis names are the
 * workspace's own: the framework declares `product` and `channel`, and a
 * workspace is free to persist others. Nothing here knows any axis name, so a
 * workspace that governs by `market` or `tenant` groups just as well.
 */

/** One collection rollup, narrowed to the fields grouping reads. */
type Rollup = Pick<CollectionTranslationStats, "coordinates" | "ungrouped">;

/**
 * What grouping needs of a collection, whichever payload it arrived on: the
 * point it sits at, and what it holds. The overview groups rollups
 * (`CollectionTranslationStats`); the source rail groups the collections
 * themselves (`CollectionInfo`). Both carry a coordinate, and neither should
 * have its own idea of what a group is.
 */
export interface GroupableCollection {
  coordinates?: Record<string, string>;
  ungrouped?: boolean;
  item_count: number;
  word_count?: number;
}

/**
 * The bucket of items belonging to no collection. The server marks it with
 * `ungrouped` rather than an invented id, because its collection id is the
 * empty string and resolves to no collection anywhere else in the API.
 */
export function isUngroupedBucket(c: Rollup): boolean {
  return c.ungrouped === true;
}

/**
 * Splits the rollups into the real collections and the collection-less bucket.
 * The bucket is a different kind of thing from a collection — it has no
 * coordinates to group by and no collection page to open — so every caller
 * handles it separately rather than pretending it is a group of one.
 */
export function partitionRollups(collections: CollectionTranslationStats[]): {
  collections: CollectionTranslationStats[];
  ungrouped?: CollectionTranslationStats;
} {
  const named: CollectionTranslationStats[] = [];
  let ungrouped: CollectionTranslationStats | undefined;
  for (const c of collections) {
    if (isUngroupedBucket(c)) ungrouped = c;
    else named.push(c);
  }
  return { collections: named, ungrouped };
}

/** Every axis any of these collections carries a coordinate on, alphabetically. */
export function coordinateAxes(collections: Rollup[]): string[] {
  const axes = new Set<string>();
  for (const c of collections) {
    for (const axis of Object.keys(c.coordinates ?? {})) axes.add(axis);
  }
  return [...axes].sort();
}

/**
 * The axis to group by when the reader has not chosen one.
 *
 * The best grouping axis is the one that places the most collections and cuts
 * them into the fewest pieces: a coarse axis (two products) makes groups worth
 * reading, while a fine one (ten channels) reproduces the flat list it was
 * meant to replace. So: widest coverage first, then fewest distinct values,
 * then alphabetically so the choice is stable across reloads.
 *
 * Undefined when no collection carries any coordinate — there is nothing to
 * group by, and the overview says so by rendering one flat list.
 */
export function defaultGroupingAxis(collections: Rollup[]): string | undefined {
  let best: { axis: string; coverage: number; distinct: number } | undefined;
  for (const axis of coordinateAxes(collections)) {
    const values = new Set<string>();
    let coverage = 0;
    for (const c of collections) {
      const value = c.coordinates?.[axis];
      if (value) {
        coverage++;
        values.add(value);
      }
    }
    if (coverage === 0) continue;
    const candidate = { axis, coverage, distinct: values.size };
    if (
      !best ||
      candidate.coverage > best.coverage ||
      (candidate.coverage === best.coverage && candidate.distinct < best.distinct)
    ) {
      best = candidate;
    }
  }
  return best?.axis;
}

/** Collections sharing one value on the grouping axis. */
export interface CollectionGroup<T extends GroupableCollection = CollectionTranslationStats> {
  /** The coordinate value the group is named by; empty when the axis is unset. */
  value: string;
  collections: T[];
  itemCount: number;
  wordCount: number;
}

/**
 * Groups collections by their value on one axis, in the order the reader meets
 * them: named values alphabetically, then the collections carrying no value on
 * that axis. Within a group the server's order is kept.
 *
 * Passing no axis returns every collection in one nameless group — the shape a
 * workspace with no coordinates gets, rendered as a plain list.
 */
export function groupCollectionsByAxis<T extends GroupableCollection>(
  collections: T[],
  axis?: string,
): CollectionGroup<T>[] {
  const byValue = new Map<string, T[]>();
  for (const c of collections) {
    const value = axis ? (c.coordinates?.[axis] ?? "") : "";
    const bucket = byValue.get(value);
    if (bucket) bucket.push(c);
    else byValue.set(value, [c]);
  }
  return [...byValue.entries()]
    .sort(([a], [b]) => {
      // The unset value trails the named ones: it is the remainder, not a peer.
      if (!a) return 1;
      if (!b) return -1;
      return a.localeCompare(b);
    })
    .map(([value, group]) => ({
      value,
      collections: group,
      itemCount: group.reduce((n, c) => n + c.item_count, 0),
      wordCount: group.reduce((n, c) => n + (c.word_count ?? 0), 0),
    }));
}

/**
 * A collection's coordinates minus everything already on screen: the axis it is
 * filed under, and any coordinate the channel badge beside it already spells
 * out. A channel is bound as `product/channel`, so for the common collection
 * those two axes *are* the badge, and printing them again says nothing.
 * Coordinates the badge cannot carry — a market, a tenant — survive.
 *
 * Undefined once nothing is left to say, so the caller renders no line rather
 * than an empty one.
 */
export function remainingCoordinates(
  collection: Rollup & { channel?: string },
  groupingAxis?: string,
): Record<string, string> | undefined {
  const coords = collection.coordinates;
  if (!coords) return undefined;
  const spelledOut = new Set((collection.channel ?? "").split("/").filter(Boolean));
  const rest = Object.fromEntries(
    Object.entries(coords).filter(
      ([axis, value]) => axis !== groupingAxis && !spelledOut.has(value),
    ),
  );
  return Object.keys(rest).length > 0 ? rest : undefined;
}
