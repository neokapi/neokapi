// The place graph: the atlas's `places`, indexed for the questions a map
// asks. Containment and overlap are declared, never computed from shapes, and
// follow core/place rule for rule; core/place/testdata/containment.json holds
// the cases both implementations run.

import type { AtlasPlace } from "../types";

/** The place that holds every other. */
export const WORLD = "world";

/** The atlas's place graph. */
export interface PlaceIndex {
  /** Every place, sorted by id. */
  readonly places: readonly AtlasPlace[];
  get(id: string): AtlasPlace | undefined;
  has(id: string): boolean;
  /** Whether a place is made of other places (`of:`). */
  madeOf(id: string): boolean;
  /** Whether `outer` contains `inner`. Every non-empty id contains itself, known or not. */
  contains(outer: string, inner: string): boolean;
  /** Whether two places share ground. */
  overlaps(a: string, b: string): boolean;
  /** The first-parent chain from the world down to `id`; empty for an unknown id. */
  path(id: string): string[];
  /** The place `path` reaches `id` through; undefined for the world and unknown ids. */
  parent(id: string): string | undefined;
  /** The places whose path parent is `id`, and the members of a place made of others, by name. */
  children(id: string): string[];
}

/** Build the place graph. Tolerates a missing list, unknown ids and cycles. */
export function placeIndex(places: readonly AtlasPlace[] | undefined): PlaceIndex {
  const byId = new Map<string, AtlasPlace>();
  for (const p of places ?? []) {
    if (p && typeof p.id === "string" && p.id !== "" && !byId.has(p.id)) byId.set(p.id, p);
  }
  const sorted = [...byId.values()].sort((a, b) => (a.id < b.id ? -1 : a.id > b.id ? 1 : 0));
  const within = (id: string) => byId.get(id)?.within ?? [];
  const of = (id: string) => byId.get(id)?.of ?? [];

  // Results are memoised. A cycle (which the host refuses at load) answers
  // false rather than recursing forever.
  const containsMemo = new Map<string, boolean>();
  const pending = new Set<string>();
  const contains = (o: string, i: string): boolean => {
    if (o === "" || i === "") return false;
    if (o === i) return true;
    if (o === WORLD) return byId.has(i);
    const key = `${o}\u0000${i}`;
    const memo = containsMemo.get(key);
    if (memo !== undefined) return memo;
    if (pending.has(key)) return false;
    pending.add(key);
    let r: boolean;
    const innerOf = of(i);
    if (innerOf.length > 0) r = innerOf.every((m) => contains(o, m));
    // A place made of others contains what one of its members contains;
    // anything else contains what it contains through a parent.
    else r = of(o).some((m) => contains(m, i)) || within(i).some((p) => contains(o, p));
    pending.delete(key);
    containsMemo.set(key, r);
    return r;
  };

  const declared = sorted.filter((p) => p.declared);
  const overlapsMemo = new Map<string, boolean>();
  const overlapsPending = new Set<string>();
  const overlaps = (a: string, b: string): boolean => {
    if (contains(a, b) || contains(b, a)) return true;
    if (a === "" || b === "") return false;
    const key = `${a}\u0000${b}`;
    const memo = overlapsMemo.get(key);
    if (memo !== undefined) return memo;
    if (overlapsPending.has(key)) return false;
    overlapsPending.add(key);
    let r: boolean;
    if (of(a).length > 0) r = of(a).some((m) => overlaps(m, b));
    else if (of(b).length > 0) r = of(b).some((m) => overlaps(a, m));
    // Built-ins form a tree: two of them that do not contain each other share
    // no ground. Only a declared place sitting within several parents joins
    // two branches.
    else r = declared.some((x) => contains(a, x.id) && contains(b, x.id));
    overlapsPending.delete(key);
    overlapsMemo.set(key, r);
    return r;
  };

  const pathMemo = new Map<string, string[]>();
  const pathPending = new Set<string>();
  const path = (id: string): string[] => {
    if (!byId.has(id)) return [];
    if (id === WORLD) return [WORLD];
    const memo = pathMemo.get(id);
    if (memo) return memo;
    if (pathPending.has(id)) return [];
    pathPending.add(id);
    let up: string[] = [];
    const ws = within(id);
    const members = of(id);
    if (ws.length > 0) {
      up = path(ws[0]);
    } else if (members.length > 0) {
      // A place made of others follows the nearest place that contains every
      // one of its members, found by walking up from the first member.
      const first = path(members[0]);
      for (let k = first.length - 1; k >= 0; k--) {
        if (first[k] !== id && members.every((m) => contains(first[k], m))) {
          up = first.slice(0, k + 1);
          break;
        }
      }
    }
    pathPending.delete(id);
    const out = up.length > 0 ? [...up, id] : [];
    pathMemo.set(id, out);
    return out;
  };

  const parent = (id: string): string | undefined => {
    const p = path(id);
    return p.length >= 2 ? p[p.length - 2] : undefined;
  };

  let childMap: Map<string, string[]> | undefined;
  const byName = (a: string, b: string) => {
    const na = byId.get(a)?.name ?? a;
    const nb = byId.get(b)?.name ?? b;
    if (na !== nb) return na < nb ? -1 : 1;
    return a < b ? -1 : a > b ? 1 : 0;
  };
  const children = (id: string): string[] => {
    if (!childMap) {
      childMap = new Map();
      for (const p of sorted) {
        const up = parent(p.id);
        if (up === undefined) continue;
        const list = childMap.get(up) ?? [];
        list.push(p.id);
        childMap.set(up, list);
      }
    }
    const out = new Set(childMap.get(id) ?? []);
    for (const m of of(id)) out.add(m);
    return [...out].sort(byName);
  };

  return {
    places: sorted,
    get: (id) => byId.get(id),
    has: (id) => byId.has(id),
    madeOf: (id) => of(id).length > 0,
    contains,
    overlaps,
    path,
    parent,
    children,
  };
}

/**
 * How a record at `at` bears on a place the map draws: it `sits` there when
 * the drawn place contains it; it is `held` there when it sits at a wider
 * place, a place made of others that covers this one, or one that shares
 * ground with it. A record at the world is neither: it holds everywhere, and
 * the map counts it once rather than hatching the globe.
 */
export function bearing(index: PlaceIndex, drawn: string, at: string): "sits" | "held" | undefined {
  if (at === "" || at === WORLD) return undefined;
  if (index.contains(drawn, at)) return "sits";
  if (index.overlaps(at, drawn)) return "held";
  return undefined;
}

/** What a set of records weighs: in the selection, needing a person, and in the surrounding cohort. */
export interface PlaceTally {
  weight: number;
  attention: number;
  around: number;
}

/** What a drawn place gathers from the records at every place. */
export interface PlaceCount {
  /** Records at places it contains. */
  sits: PlaceTally;
  /** Records at wider or overlapping places that hold here. */
  held: PlaceTally;
  /** The places those held records sit at, by id. */
  heldThrough: string[];
}

const zero = (): PlaceTally => ({ weight: 0, attention: 0, around: 0 });

function add(into: PlaceTally, x: PlaceTally) {
  into.weight += x.weight;
  into.attention += x.attention;
  into.around += x.around;
}

/**
 * Fold per-place tallies (keyed by the place a record sits at) onto the
 * places a map draws.
 */
export function placeCounts(
  index: PlaceIndex,
  drawn: readonly string[],
  byPlace: ReadonlyMap<string, PlaceTally>,
): Map<string, PlaceCount> {
  const out = new Map<string, PlaceCount>();
  for (const d of drawn) {
    const c: PlaceCount = { sits: zero(), held: zero(), heldThrough: [] };
    for (const [at, x] of byPlace) {
      const b = bearing(index, d, at);
      if (b === "sits") add(c.sits, x);
      else if (b === "held") {
        add(c.held, x);
        if (x.around > 0 || x.weight > 0) c.heldThrough.push(at);
      }
    }
    c.heldThrough.sort();
    out.set(d, c);
  }
  return out;
}

/**
 * The places of `values` that share ground with `id`: what selecting `id` on
 * the map retains, so the atlas keeps every record that holds there.
 */
export function overlapping(index: PlaceIndex, id: string, values: readonly string[]): string[] {
  return values.filter((v) => v !== "" && index.overlaps(v, id));
}

/**
 * The deepest place, no deeper than a country, that contains every place in
 * `at`; the world when they share nothing narrower. The map opens there.
 */
export function commonPlace(index: PlaceIndex, at: readonly string[]): string {
  const known = at.filter((p) => p !== WORLD && index.has(p));
  if (known.length === 0) return WORLD;
  const first = index.path(known[0]);
  for (let k = first.length - 1; k >= 0; k--) {
    const p = first[k];
    const type = index.get(p)?.type;
    if (type !== "world" && type !== "region" && type !== "country") continue;
    if (known.every((x) => index.contains(p, x))) return p;
  }
  return WORLD;
}

/** What the shaped map draws with one place in focus. */
export interface MapLevel {
  focus: string;
  /**
   * `countries`: the world's countries are drawn, and those the focus
   * contains are the ones a reader can pick; `inside`: the places inside the
   * focus are drawn over its outline.
   */
  layer: "countries" | "inside";
  /** The places drawn as a shape or a circle, or listed when they have neither. */
  drawn: string[];
  /** Places made of others, and regions, that share ground with the focus without containing it. */
  chips: string[];
  /**
   * The places that contain the focus, the focus first and the world last:
   * a record at any of them holds across the whole view.
   */
  wider: string[];
}

const WIDE_TYPES = new Set(["world", "region"]);

/** The places the shaped map draws, lists and counts with `focus` in focus. */
export function mapLevel(index: PlaceIndex, focus: string): MapLevel {
  const place = index.get(focus);
  const members = place?.of ?? [];
  const countries =
    focus === WORLD ||
    WIDE_TYPES.has(place?.type ?? "") ||
    (members.length > 0 &&
      members.every((m) => {
        const t = index.get(m)?.type ?? "";
        return t === "country" || WIDE_TYPES.has(t);
      }));
  const isChip = (id: string) => index.madeOf(id) || WIDE_TYPES.has(index.get(id)?.type ?? "");
  const drawn = countries
    ? index.places
        .filter((p) => p.type === "country" && index.contains(focus, p.id))
        .map((p) => p.id)
    : index.children(focus).filter((c) => !isChip(c));
  const drawnSet = new Set(drawn);
  const chips = index.places
    .filter(
      (p) =>
        p.id !== focus &&
        p.id !== WORLD &&
        !drawnSet.has(p.id) &&
        isChip(p.id) &&
        !index.contains(p.id, focus) &&
        index.overlaps(p.id, focus),
    )
    .map((p) => p.id);
  const depth = (id: string) => index.path(id).length;
  const rank = (id: string) => (id === focus ? Number.POSITIVE_INFINITY : depth(id));
  const wider = index.places
    .filter((p) => index.contains(p.id, focus))
    .map((p) => p.id)
    .sort((a, b) => rank(b) - rank(a) || (a < b ? -1 : a > b ? 1 : 0));
  return { focus, layer: countries ? "countries" : "inside", drawn, chips, wider };
}
