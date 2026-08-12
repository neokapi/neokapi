// The scope tuple and the resolution ladder it induces (Apache-2.0).
//
// One tuple carries every dimension a context question can be asked at. A
// surface declares which dimensions it PINS and which it leaves FREE: kapi runs
// the explorer with workspace/project/stream pinned to the one project on disk,
// Bowrain runs it with them free. The components read the same tuple either way
// — pinning is a property of the mount, never a second set of views.

/** A dimension of the context space, coarsest first. */
export type Dimension =
  | "workspace"
  | "project"
  | "stream"
  | "collection"
  | "path"
  | "coordinate"
  | "locale";

/**
 * The ladder: the dimensions that nest, in resolution order. Coordinate and
 * locale are filters over a point rather than rungs of it — content sits at a
 * path, and its coordinate is what the path resolves TO — so they slice the
 * views without appearing in the breadcrumbs.
 */
export const LADDER: readonly Dimension[] = [
  "workspace",
  "project",
  "stream",
  "collection",
  "path",
] as const;

/** The dimensions that are filters, not rungs. */
export const FILTERS: readonly Dimension[] = ["coordinate", "locale"] as const;

/** Where a question is being asked. Every field is optional and AND-combined. */
export interface ScopeTuple {
  workspace?: string;
  project?: string;
  stream?: string;
  collection?: string;
  /** A project-relative, slash-separated file path. */
  path?: string;
  /** A point in the context space, written `profile/channel` or `channel`. */
  coordinate?: string;
  locale?: string;
}

/** Read one dimension off a tuple. */
export function dimensionValue(scope: ScopeTuple, d: Dimension): string | undefined {
  return scope[d];
}

/** A tuple with `d` set, and every finer ladder rung cleared. */
export function withDimension(
  scope: ScopeTuple,
  d: Dimension,
  value: string | undefined,
): ScopeTuple {
  const next: ScopeTuple = { ...scope, [d]: value || undefined };
  const rung = LADDER.indexOf(d);
  if (rung >= 0) {
    for (const finer of LADDER.slice(rung + 1)) next[finer] = undefined;
  }
  return next;
}

/** A tuple with `d` and every finer ladder rung cleared. */
export function clearDimension(scope: ScopeTuple, d: Dimension): ScopeTuple {
  return withDimension(scope, d, undefined);
}

/** One rung of the rendered ladder. */
export interface LadderRung {
  dimension: Dimension;
  value: string;
  /** The tuple this rung addresses — what selecting it navigates to. */
  scope: ScopeTuple;
  /** The `context://` address of that tuple. */
  address: string;
  /** False when the mount pins this dimension, so the rung reads but does not move. */
  navigable: boolean;
}

/**
 * The ladder a tuple stands on: one rung per set dimension, coarsest first.
 *
 * A path contributes one rung per segment, because a file's governance resolves
 * per directory as readily as per file and a reader walking back up expects the
 * directories to be there.
 */
export function ladderOf(scope: ScopeTuple, free: readonly Dimension[]): LadderRung[] {
  const rungs: LadderRung[] = [];
  let walked: ScopeTuple = {};

  for (const d of LADDER) {
    const value = scope[d];
    if (!value) continue;

    if (d !== "path") {
      walked = { ...walked, [d]: value };
      rungs.push({
        dimension: d,
        value,
        scope: { ...walked },
        address: contextAddress({ ...walked }, free),
        navigable: free.includes(d),
      });
      continue;
    }

    const segments = value.split("/").filter(Boolean);
    segments.forEach((segment, i) => {
      const partial = segments.slice(0, i + 1).join("/");
      walked = { ...walked, path: partial };
      rungs.push({
        dimension: "path",
        value: segment,
        scope: { ...walked },
        address: contextAddress({ ...walked }, free),
        navigable: true,
      });
    });
  }

  return rungs;
}

/**
 * The `context://` address of a tuple.
 *
 * Pinned dimensions are implicit; free dimensions are named. A kapi mount pins
 * workspace, project and stream, so its addresses are exactly the
 * `context://<path>` the CLI and the MCP resource template use. A Bowrain mount
 * leaves them free, so its addresses name them:
 *
 *   context://acme/site@main/docs/help/billing.md
 *
 * The rule is one rule, not two syntaxes: an address states what the reader
 * cannot infer from where they are standing.
 */
export function contextAddress(scope: ScopeTuple, free: readonly Dimension[]): string {
  const authority: string[] = [];
  if (free.includes("workspace") && scope.workspace) authority.push(scope.workspace);
  if (free.includes("project") && scope.project) {
    authority.push(
      free.includes("stream") && scope.stream ? `${scope.project}@${scope.stream}` : scope.project,
    );
  }

  const segments = [...authority];
  if (scope.collection) segments.push(scope.collection);
  if (scope.path) segments.push(scope.path);

  return `context://${segments.join("/")}`;
}

/** The `context://profile/<name>` address of a named voice profile. */
export function profileAddress(name: string): string {
  return `context://profile/${name}`;
}

/** True when nothing narrows the tuple below the mount's pinned dimensions. */
export function isRoot(scope: ScopeTuple, free: readonly Dimension[]): boolean {
  return ladderOf(scope, free).every((rung) => !rung.navigable);
}
