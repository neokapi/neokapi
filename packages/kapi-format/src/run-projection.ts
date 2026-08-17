/**
 * run-projection — the one way to turn a Run sequence into something else.
 *
 * A run sequence is the content model's ground truth, and every surface that
 * shows content has to project it into its own shape: a string, a chip, a
 * segment. The projection is where content gets lost, because the lossy version
 * of the loop is the one that is easy to write:
 *
 *     for (const r of runs) if (typeof r.text === "string") out += r.text;
 *
 * That reads as "concatenate the text" and behaves as "silently delete every
 * placeholder, every paired code, every plural". It shipped three times in this
 * repository — a review pane whose source read "Your credits reset on ." beside
 * a target that showed the variable; a document preview that drew a plural
 * block as an empty line; a lab that measured segmentation over text no reader
 * ever saw. Nothing failed. The content was simply gone.
 *
 * So a projection is *declared*, never written as a loop. A {@link RunSpec}
 * answers for every kind of run the model defines, and its type is a mapped
 * type over {@link RUN_KINDS} — leave `plural` out and the call site does not
 * compile. Each kind gets one of four answers, all of them explicit:
 *
 *     text:   (run) => T                     render it
 *     plural: { expand: (run) => T[] }       render it as several values
 *     ph:     { dropped: "why" }             contribute nothing, and say why
 *     sub:    { unsupported: "why" }         must not occur — report it if it does
 *
 * "Contribute nothing" stays available, because some projections genuinely have
 * no width for an inline code (an offset domain mirroring `model.RunsText`).
 * What is no longer available is contributing nothing by accident.
 *
 * Adding a kind to the model means adding it to {@link RUN_KINDS}, which breaks
 * every projection in the workspace until each has said what it does with it.
 * That is the point: the compiler asks the question reviewers forget.
 *
 * `scripts/check-run-projection.sh` guards the rule against a hand-rolled loop
 * growing back beside this one.
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
 * A complete answer for every kind of run, plus what to show in place of a run
 * this projection cannot render — an `unsupported` kind, or one the model has
 * gained that this build does not know. `fallback` is required because the
 * alternative is the silence this module exists to prevent: whatever a surface
 * puts there, it puts *something*.
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
 * Loud where it can be — a thrown error under Node (tests, build steps, the
 * CLI), so a projection that meets a run it cannot render is fixed before it
 * ships. In a browser, a surface that throws mid-render is worse than one that
 * shows a marker, so the report goes to the console once per (kind, reason) and
 * the spec's fallback is drawn in the run's place.
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
      // A run this build has no discriminator for: a newer engine, or a payload
      // that lost its shape. Either way it is content, and it is not silent.
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
