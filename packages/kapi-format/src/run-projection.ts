/**
 * Projects Run sequences into display text, chips, segments or other values.
 *
 * Use a {@link RunSpec} to handle every kind in {@link RUN_KINDS}. Concatenating
 * only runs with a text property silently drops placeholders, paired codes and
 * plurals. The mapped type requires an explicit choice for each kind:
 *
 *     text:   (run) => T                     render one value
 *     plural: { expand: (run) => T[] }       render several values
 *     ph:     { dropped: "why" }             omit intentionally
 *     sub:    { unsupported: "why" }         report unsupported content
 *
 * Intentional omission supports projections such as offset domains, where inline
 * codes have no width. Unsupported content is reported and uses the spec's
 * required fallback.
 *
 * Adding a kind to RUN_KINDS requires every projection to handle it before the
 * code compiles. scripts/check-run-projection.sh rejects ad hoc projection loops.
 */

/**
 * Every kind of run the model defines (RFC 0001), in the model's order.
 *
 * This list is the exhaustiveness contract: a kind added here is a compile
 * error in every projection that has not answered for it.
 */
export const RUN_KINDS = ["text", "ph", "pcOpen", "pcClose", "sub", "plural", "select"] as const;

/** The discriminator key of a run. */
export type RunKind = (typeof RUN_KINDS)[number];

/** The minimum shape a projection needs: an object keyed by its discriminator. */
export interface RunLike {
  text?: string;
  ph?: unknown;
  pcOpen?: unknown;
  pcClose?: unknown;
  sub?: unknown;
  plural?: { pivot?: string; forms: Record<string, unknown[]> };
  select?: { pivot?: string; cases: Record<string, unknown[]> };
}

/**
 * The member of `R` carrying kind `K`, when `R` is a discriminated union — so a
 * rule for `ph` reads `run.ph` with no narrowing of its own. A loose run type
 * (every key optional, as the preview kit's local mirror still is) has no such
 * member and the rule receives the run itself.
 */
export type RunOf<R, K extends RunKind> = [Extract<R, { [P in K]: unknown }>] extends [never]
  ? R
  : Extract<R, { [P in K]: unknown }>;

/**
 * What a projection does with one kind of run — a function renders it, and the
 * three object forms are the ways of saying something other than "render".
 */
export type RunRule<R, T, K extends RunKind> =
  | ((run: RunOf<R, K>) => T)
  | { readonly expand: (run: RunOf<R, K>) => readonly T[] }
  | { readonly dropped: string }
  | { readonly unsupported: string };

/**
 * Handles every known run kind and provides a fallback for unsupported content.
 * The fallback also handles run kinds introduced by a newer engine, ensuring the
 * projection displays a replacement instead of silently omitting content.
 */
export type RunSpec<R, T> = {
  readonly [K in RunKind]: RunRule<R, T, K>;
} & {
  readonly fallback: (kind: string, why: string) => T;
};

/** The strict model union, for specs written against `Run` itself. */
export type ModelRunSpec<T> = RunSpec<import("./block.ts").Run, T>;

/** The discriminator key of a run, or `null` when it carries none this build knows. */
export function runKindOf(run: RunLike): RunKind | null {
  if (typeof run.text === "string") return "text";
  if (run.ph) return "ph";
  if (run.pcOpen) return "pcOpen";
  if (run.pcClose) return "pcClose";
  if (run.sub) return "sub";
  if (run.plural) return "plural";
  if (run.select) return "select";
  return null;
}

/** How an unrenderable run is reported. Replaceable so a host can route it. */
export type RunProjectionReporter = (kind: string, why: string) => void;

let reporter: RunProjectionReporter = defaultReporter;

/** Route unrenderable-run reports (to Sentry, a test spy, …). */
export function setRunProjectionReporter(fn: RunProjectionReporter | null): void {
  reporter = fn ?? defaultReporter;
}

const reported = new Set<string>();

/**
 * Throw in non-production Node environments so tests and build steps detect
 * unsupported runs. In browsers, report each (kind, reason) once and render the
 * spec's fallback, allowing the rest of the content to remain visible.
 */
function defaultReporter(kind: string, why: string): void {
  const message = `run projection: cannot render a "${kind}" run — ${why}`;
  // `globalThis.process` rather than a bare `process`: this module is imported
  // by the browser runtime, which has no Node types and no `process` at all.
  const env = (globalThis as { process?: { env?: Record<string, string | undefined> } }).process
    ?.env?.NODE_ENV;
  if (env !== undefined && env !== "production") throw new Error(message);
  const key = `${kind} ${why}`;
  if (reported.has(key)) return;
  reported.add(key);
  console.error(message);
}

/** Project a run sequence through a spec, in document order. */
export function projectRuns<R extends RunLike, T>(
  runs: readonly R[] | undefined,
  spec: RunSpec<R, T>,
): T[] {
  const out: T[] = [];
  if (!runs) return out;

  for (const run of runs) {
    const kind = runKindOf(run);
    if (kind === null) {
      // Handle unknown run kinds and malformed payloads through the required
      // fallback so their content is visibly marked as unsupported.
      const why = "the run carries no discriminator this build knows";
      reporter("unknown", why);
      out.push(spec.fallback("unknown", why));
      continue;
    }
    const rule = spec[kind] as RunRule<R, T, RunKind>;
    if (typeof rule === "function") {
      out.push(rule(run as RunOf<R, RunKind>));
    } else if ("expand" in rule) {
      out.push(...rule.expand(run as RunOf<R, RunKind>));
    } else if ("unsupported" in rule) {
      reporter(kind, rule.unsupported);
      out.push(spec.fallback(kind, rule.unsupported));
    }
    // `dropped` is the remaining case: nothing, on purpose, with its reason
    // stated at the declaration.
  }
  return out;
}

/** {@link projectRuns} for a string projection, joined in document order. */
export function projectRunsText<R extends RunLike>(
  runs: readonly R[] | undefined,
  spec: RunSpec<R, string>,
): string {
  return projectRuns(runs, spec).join("");
}

/**
 * The branch a plural or select run contributes when a projection reads one
 * form: ICU's `other`, else the first present. Mirrors `model.RunsText`, so a
 * position computed over a projection means what the engine means by it.
 */
export function otherBranch<R>(branches: Record<string, R[]>): R[] {
  const other = branches.other;
  if (other) return other;
  for (const key of Object.keys(branches)) return branches[key];
  return [];
}
