import type { CSSProperties, ReactElement } from "react";
import Link from "@docusaurus/Link";
import Layout from "@theme/Layout";
import history from "./_contexteval.json";
import { t } from "@neokapi/i18n-react/runtime";

// The context-eval dashboard. kapi's value proposition is injecting context —
// terminology, a voice guide, an instruction — to steer model output. This
// page measures whether that context is actually followed, per model, as a
// differential: the same engineered corpus translated with and without the
// context, both passes scored by kapi's own check tools. Regenerate with
// `make context-eval-publish`.
//
// Every claim on this page is computed from the committed history rather than
// typed into the prose — a sentence that hardcodes a finding keeps asserting it
// long after the data stops supporting it.

interface Counts {
  scored: number;
  passed: number;
}
interface PassRecord {
  input_tokens: number;
  output_tokens: number;
  elapsed_seconds: number;
  cost_usd?: number;
  missing?: number;
  untranslated?: number;
}
interface DimensionRecord {
  dimension: string;
  bare: Counts;
  steered: Counts;
}
interface KindRecord {
  dimension: string;
  kind: string;
  bare: Counts;
  steered: Counts;
}
interface JudgeRecord {
  provider: string;
  model: string;
  rubric_digest: string;
  criteria: number;
  bare: Counts;
  steered: Counts;
  skipped_same_family?: boolean;
  /** en → en-GB is never judged: a same-language register verdict grades the
   *  source, not the adaptation. */
  skipped_same_language?: boolean;
  error?: string;
}
interface Price {
  input_per_mtok: number;
  output_per_mtok: number;
  route: string;
  metered: boolean;
  as_of?: string;
}
interface Run {
  date: string;
  provider: string;
  model: string;
  target: string;
  repeat: number;
  price?: Price;
  corpus: string;
  corpus_fixtures?: number;
  corpus_checks?: number;
  corpus_digest: string;
  context_tokens?: number;
  simulated?: boolean;
  bare?: PassRecord;
  steered?: PassRecord;
  dimensions?: DimensionRecord[];
  kinds?: KindRecord[];
  judge?: JudgeRecord;
  unmeasured?: boolean;
  error?: string;
}
interface JudgeValidation {
  date: string;
  provider: string;
  model: string;
  rubric_digest: string;
  items: number;
  agreement: number;
  kappa: number;
  /** Locales the human labels covered — a judge validated on en-GB and nb is
   *  being trusted on de and fr, and the page says so. */
  targets?: string[];
}
interface History {
  runs: Run[];
  judge_validations?: JudgeValidation[];
}

// The publication bar for the judged dimension, mirrored from
// scripts/contexteval/judge.go (MinJudgeKappa / MinJudgeItems).
const MIN_JUDGE_KAPPA = 0.6;
const MIN_JUDGE_ITEMS = 30;

const h = history as unknown as History;

// A simulated run measures the harness, not a model. It must never reach a
// chart — a stub's curve is exactly the number someone would quote. (`?? []`:
// a history written by -judge-validate alone may carry no runs.)
const real = (h.runs ?? []).filter((r) => !r.simulated);

// Dimensions the page renders: the corpus's canonical three, plus anything
// else the history actually records — derived, so a dimension added to the
// corpus can never be measured, committed, and silently invisible here.
const DIM_ORDER = ["terminology", "voice", "instruction"];
const DIMENSIONS = [
  ...DIM_ORDER,
  ...[...new Set(real.flatMap((r) => (r.dimensions ?? []).map((d) => d.dimension)))].filter(
    (d) => !DIM_ORDER.includes(d),
  ),
];
const validations = h.judge_validations ?? [];

const label = (r: Run) => r.model || r.provider;

const rate = (c: Counts | undefined): number | null =>
  c && c.scored > 0 ? (c.passed / c.scored) * 100 : null;

const lift = (d: { bare: Counts; steered: Counts }): number | null => {
  const b = rate(d.bare);
  const s = rate(d.steered);
  return b == null || s == null ? null : s - b;
};

function overall(r: Run): { bare: Counts; steered: Counts } {
  const sum = { bare: { scored: 0, passed: 0 }, steered: { scored: 0, passed: 0 } };
  for (const d of r.dimensions ?? []) {
    sum.bare.scored += d.bare.scored;
    sum.bare.passed += d.bare.passed;
    sum.steered.scored += d.steered.scored;
    sum.steered.passed += d.steered.passed;
  }
  return sum;
}

/** Lift bought per 1,000 context tokens — is the guide earning what it costs
 *  to send on this model? Null when either side is unmeasured. */
function liftPer1k(r: Run): number | null {
  const l = lift(overall(r));
  if (l == null || !r.context_tokens) return null;
  return l / (r.context_tokens / 1000);
}

// Runs are grouped per target; within a target, only the newest experiment
// (corpus digest) is plotted — a changed corpus is a different experiment, and
// drawing both on one chart would invent a trend. Within the experiment, the
// newest measurement of each model wins.
interface Experiment {
  target: string;
  digest: string;
  date: string;
  corpus: string;
  runs: Run[];
}

const scored = (r: Run) => !r.unmeasured && !r.error && (r.dimensions?.length ?? 0) > 0;

const experiments: Experiment[] = [...new Set(real.map((r) => r.target))]
  .map((target) => {
    const all = real.filter((r) => r.target === target);
    // The digest to plot is chosen among *scored* runs — a run that was
    // throttled (unmeasured) or failed must not pin the page to an experiment
    // nobody has numbers for. On the latest scored day, the digest with the
    // most scored runs wins, so a same-day corpus change cannot be decided by
    // incidental sort order.
    const scoredRuns = all.filter(scored);
    const pool = scoredRuns.length ? scoredRuns : all;
    const lastDate = pool
      .map((r) => r.date)
      .sort()
      .slice(-1)[0];
    const dayDigests = new Map<string, number>();
    for (const r of pool.filter((x) => x.date === lastDate)) {
      dayDigests.set(r.corpus_digest, (dayDigests.get(r.corpus_digest) ?? 0) + 1);
    }
    const digest = [...dayDigests.entries()]
      .sort((a, b) => a[1] - b[1] || a[0].localeCompare(b[0]))
      .slice(-1)[0]![0];
    const inExp = all.filter((r) => r.corpus_digest === digest);
    const runs = [...new Set(inExp.map(label))]
      .map(
        (m) =>
          inExp
            .filter((r) => label(r) === m)
            .sort((a, b) => a.date.localeCompare(b.date))
            .slice(-1)[0],
      )
      .filter((r): r is Run => r != null);
    return {
      target,
      digest,
      corpus: runs[0]?.corpus ?? "",
      date:
        runs
          .map((r) => r.date)
          .sort()
          .slice(-1)[0] ?? "",
      runs,
    };
  })
  .sort((a, b) => a.target.localeCompare(b.target));

// Findings, computed from the data. The trend question that survives noise:
// which models does the context actually move, and where is it dead weight?
interface Findings {
  bestLift: { model: string; target: string; lift: number } | null;
  deadWeight: { model: string; target: string; lift: number }[];
  weakestDim: { dimension: string; rate: number } | null;
}

function findings(): Findings {
  let bestLift: Findings["bestLift"] = null;
  const deadWeight: Findings["deadWeight"] = [];
  const dimTotals = new Map<string, { scored: number; passed: number }>();

  for (const e of experiments) {
    for (const r of e.runs.filter(scored)) {
      const l = lift(overall(r));
      if (l != null) {
        if (!bestLift || l > bestLift.lift)
          bestLift = { model: label(r), target: e.target, lift: l };
        if (l <= 0) deadWeight.push({ model: label(r), target: e.target, lift: l });
      }
      for (const d of r.dimensions ?? []) {
        const t = dimTotals.get(d.dimension) ?? { scored: 0, passed: 0 };
        t.scored += d.steered.scored;
        t.passed += d.steered.passed;
        dimTotals.set(d.dimension, t);
      }
    }
  }
  let weakestDim: Findings["weakestDim"] = null;
  for (const [dimension, t] of dimTotals) {
    if (!t.scored) continue;
    const r = (t.passed / t.scored) * 100;
    if (!weakestDim || r < weakestDim.rate) weakestDim = { dimension, rate: r };
  }
  return { bestLift, deadWeight, weakestDim };
}

const palette = [
  "#4c78a8",
  "#f58518",
  "#54a24b",
  "#e45756",
  "#b279a2",
  "#72b7b2",
  "#ff9da6",
  "#9d755d",
];

const cell: CSSProperties = {
  padding: "6px 10px",
  borderBottom: "1px solid var(--ifm-table-border-color)",
  textAlign: "right",
  fontVariantNumeric: "tabular-nums",
};
const left: CSSProperties = { ...cell, textAlign: "left" };

const pp = (v: number) => `${v >= 0 ? "+" : "−"}${Math.abs(v).toFixed(1)}pp`;
const usd = (v: number) =>
  v >= 1 ? `$${v.toFixed(2)}` : v >= 0.01 ? `$${v.toFixed(3)}` : `$${v.toFixed(4)}`;

/** Dumbbell chart: one row per model per dimension, a line from the bare
 *  adherence to the steered adherence. The line IS the lift — length and
 *  direction — which a pair of bars would make the reader compute. */
function LiftChart({ runs }: { runs: Run[] }): ReactElement | null {
  const plotted = runs.filter(scored);
  if (!plotted.length) return null;

  const W = 720;
  const rowH = 26;
  const groupGap = 18;
  const pad = { top: 24, right: 30, bottom: 30, left: 150 };

  interface Row {
    dim: string;
    model: string;
    color: string;
    bare: number | null;
    steered: number | null;
    first: boolean;
  }
  const rows: Row[] = [];
  for (const dim of DIMENSIONS) {
    let first = true;
    plotted.forEach((r, i) => {
      const d = r.dimensions?.find((x) => x.dimension === dim);
      if (!d) return;
      rows.push({
        dim,
        model: label(r),
        color: palette[i % palette.length],
        bare: rate(d.bare),
        steered: rate(d.steered),
        first,
      });
      first = false;
    });
  }
  if (!rows.length) return null;

  const groups = new Set(rows.map((r) => r.dim)).size;
  const H = pad.top + rows.length * rowH + groups * groupGap + pad.bottom;
  const x = (v: number) => pad.left + (v / 100) * (W - pad.left - pad.right);

  let y = pad.top;
  const positioned = rows.map((r) => {
    if (r.first) y += groupGap;
    const out = { ...r, y };
    y += rowH;
    return out;
  });

  return (
    <svg
      viewBox={`0 0 ${W} ${H}`}
      style={{ width: "100%", height: "auto", overflow: "visible" }}
      role="img"
      aria-label="Adherence without context (open dot) to adherence with context (filled dot), one row per model per dimension"
    >
      {[0, 25, 50, 75, 100].map((g) => (
        <g key={g}>
          <line
            x1={x(g)}
            x2={x(g)}
            y1={pad.top}
            y2={H - pad.bottom}
            stroke="var(--ifm-color-emphasis-300)"
            strokeWidth={1}
            strokeDasharray={g === 0 || g === 100 ? undefined : "3 3"}
          />
          <text
            x={x(g)}
            y={H - pad.bottom + 16}
            textAnchor="middle"
            fontSize={11}
            fill="var(--ifm-color-emphasis-600)"
          >
            {g}%
          </text>
        </g>
      ))}
      {positioned.map((r) => (
        <g key={`${r.dim}-${r.model}`}>
          {r.first && (
            <text
              x={0}
              y={r.y + 4}
              fontSize={12}
              fontWeight={600}
              fill="var(--ifm-color-emphasis-800)"
            >
              {r.dim}
            </text>
          )}
          <text
            x={pad.left - 8}
            y={r.y + 4}
            textAnchor="end"
            fontSize={11}
            fill="var(--ifm-color-emphasis-700)"
          >
            {r.model}
          </text>
          {r.bare != null && r.steered != null && (
            <line
              x1={x(r.bare)}
              x2={x(r.steered)}
              y1={r.y}
              y2={r.y}
              stroke={r.color}
              strokeWidth={2.5}
              opacity={0.75}
            />
          )}
          {r.bare != null && (
            <circle
              cx={x(r.bare)}
              cy={r.y}
              r={4.5}
              fill="var(--ifm-background-color)"
              stroke={r.color}
              strokeWidth={2}
            >
              <title>
                {r.model} · {r.dim} · bare {r.bare.toFixed(1)}%
              </title>
            </circle>
          )}
          {r.steered != null && (
            <circle cx={x(r.steered)} cy={r.y} r={4.5} fill={r.color}>
              <title>
                {r.model} · {r.dim} · steered {r.steered.toFixed(1)}%
              </title>
            </circle>
          )}
        </g>
      ))}
    </svg>
  );
}

function Legend({ runs }: { runs: Run[] }): ReactElement {
  return (
    <div style={{ display: "flex", gap: 16, flexWrap: "wrap", margin: "4px 0 8px" }}>
      {runs.filter(scored).map((r, i) => (
        <span
          key={label(r)}
          style={{ display: "flex", alignItems: "center", gap: 6, fontSize: "0.88rem" }}
        >
          <span
            style={{
              width: 14,
              height: 3,
              background: palette[i % palette.length],
              display: "inline-block",
            }}
          />
          {label(r)}
          <span style={{ color: "var(--ifm-color-emphasis-600)" }}>({r.provider})</span>
        </span>
      ))}
      <span style={{ fontSize: "0.82rem", color: "var(--ifm-color-emphasis-600)" }}>
        open dot = without context · filled dot = with context
      </span>
    </div>
  );
}

/** The judged dimension is published only when the judge that produced it has
 *  measured agreement with human labels on record, under the same rubric, above
 *  the bar. Anything else would report the judge's opinion as the model's
 *  behavior. */
function judgeValidationFor(j: JudgeRecord): JudgeValidation | undefined {
  return validations.find(
    (v) => v.provider === j.provider && v.model === j.model && v.rubric_digest === j.rubric_digest,
  );
}
function judgePublishable(v: JudgeValidation | undefined): boolean {
  return v != null && v.kappa >= MIN_JUDGE_KAPPA && v.items >= MIN_JUDGE_ITEMS;
}

function ExperimentSection({ e }: { e: Experiment }): ReactElement {
  const plotted = e.runs.filter(scored);
  const judged = plotted.filter(
    (r) =>
      r.judge &&
      !r.judge.skipped_same_family &&
      !r.judge.skipped_same_language &&
      !r.judge.error &&
      r.judge.steered.scored > 0,
  );
  return (
    <section>
      <h2>
        Target {e.target}{" "}
        <span
          style={{ fontWeight: 400, fontSize: "0.85rem", color: "var(--ifm-color-emphasis-600)" }}
        >
          measured {e.date} · {e.corpus} · corpus <code>{e.digest}</code>
        </span>
      </h2>

      <LiftChart runs={e.runs} />
      <Legend runs={e.runs} />

      <table style={{ width: "100%", borderCollapse: "collapse", fontSize: "0.9rem" }}>
        <thead>
          <tr>
            <th style={left}>Model</th>
            {DIMENSIONS.map((d) => (
              <th key={d} style={cell}>
                {d} lift
              </th>
            ))}
            <th style={cell}>overall lift</th>
            <th style={cell}>context tok/pass</th>
            <th style={cell}>lift / 1k ctx tok</th>
            <th style={cell}>steered $/pass</th>
          </tr>
        </thead>
        <tbody>
          {plotted.map((r) => {
            const o = lift(overall(r));
            const per1k = liftPer1k(r);
            return (
              <tr key={label(r)}>
                <td style={left}>{label(r)}</td>
                {DIMENSIONS.map((dim) => {
                  const d = r.dimensions?.find((x) => x.dimension === dim);
                  const l = d ? lift(d) : null;
                  return (
                    <td
                      key={dim}
                      style={{ ...cell, color: l != null && l < 0 ? "#d65a5a" : undefined }}
                    >
                      {l != null ? pp(l) : "—"}
                    </td>
                  );
                })}
                <td style={{ ...cell, fontWeight: 600 }}>{o != null ? pp(o) : "—"}</td>
                <td style={cell}>{r.context_tokens ?? "—"}</td>
                <td style={cell}>{per1k != null ? pp(per1k) : "—"}</td>
                <td style={cell}>
                  {r.steered?.cost_usd ? usd(r.steered.cost_usd / Math.max(r.repeat, 1)) : "—"}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>

      <h3 style={{ marginTop: 24 }}>Per-trap breakdown</h3>
      <p style={{ fontSize: "0.9rem", color: "var(--ifm-color-emphasis-700)" }}>
        Adherence with context, by the kind of trap the fixture set. A model can hold the terms and
        still translate the product name; this is where that shows.
      </p>
      {plotted.map((r) => (
        <div key={label(r)} style={{ marginBottom: 20 }}>
          <h4 style={{ marginBottom: 4 }}>
            {label(r)}{" "}
            <span
              style={{
                fontWeight: 400,
                fontSize: "0.82rem",
                color: "var(--ifm-color-emphasis-600)",
              }}
            >
              {r.provider} · {r.repeat}× per variant
            </span>
          </h4>
          <table style={{ width: "100%", borderCollapse: "collapse", fontSize: "0.88rem" }}>
            <thead>
              <tr>
                <th style={left}>dimension / kind</th>
                <th style={cell}>checks</th>
                <th style={cell}>bare</th>
                <th style={cell}>steered</th>
                <th style={cell}>lift</th>
              </tr>
            </thead>
            <tbody>
              {(r.kinds ?? []).map((k) => {
                const b = rate(k.bare);
                const s = rate(k.steered);
                const l = lift(k);
                return (
                  <tr key={`${k.dimension}/${k.kind}`}>
                    <td style={left}>
                      {k.dimension} / {k.kind}
                    </td>
                    <td style={cell}>{k.steered.scored}</td>
                    <td style={cell}>{b != null ? `${b.toFixed(0)}%` : "—"}</td>
                    <td style={{ ...cell, color: s != null && s < 100 ? "#d65a5a" : undefined }}>
                      {s != null ? `${s.toFixed(0)}%` : "—"}
                    </td>
                    <td style={cell}>{l != null ? pp(l) : "—"}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
          {r.bare?.missing ||
          r.bare?.untranslated ||
          r.steered?.missing ||
          r.steered?.untranslated ? (
            <p
              style={{
                fontSize: "0.82rem",
                color: "var(--ifm-color-emphasis-700)",
                margin: "6px 0 0",
              }}
            >
              Excluded from adherence: bare {r.bare?.missing ?? 0} missing /{" "}
              {r.bare?.untranslated ?? 0} echoed, steered {r.steered?.missing ?? 0} missing /{" "}
              {r.steered?.untranslated ?? 0} echoed. An echoed segment would trivially
              &ldquo;pass&rdquo; every do-not-translate check, so it is counted here instead.
            </p>
          ) : null}
        </div>
      ))}

      {e.runs.some((r) => r.unmeasured) && (
        <p style={{ fontSize: "0.85rem", color: "var(--ifm-color-emphasis-700)" }}>
          {e.runs
            .filter((r) => r.unmeasured)
            .map((r) => label(r))
            .join(", ")}
          : the provider throttled the requests, so nothing was measured. Scored as a hole, not as
          0% adherence.
        </p>
      )}
      {e.runs.some((r) => r.error && !r.unmeasured) && (
        <p style={{ fontSize: "0.85rem", color: "#d65a5a" }}>
          {e.runs
            .filter((r) => r.error && !r.unmeasured)
            .map((r) => `${label(r)}: ${r.error}`)
            .join("; ")}
        </p>
      )}

      {judged.length > 0 && (
        <>
          <h3>Judged voice (register, naturalness, restraint)</h3>
          {judged.map((r) => {
            const j = r.judge!;
            const v = judgeValidationFor(j);
            if (!judgePublishable(v)) {
              const reason = v
                ? t(
                    "measured judge–human agreement (kappa {kappa} over {items} verdicts) is below the bar (kappa ≥ {minKappa} over ≥ {minItems})",
                    "why a judge's scores are not published",
                    {
                      kappa: v.kappa.toFixed(2),
                      items: v.items,
                      minKappa: MIN_JUDGE_KAPPA,
                      minItems: MIN_JUDGE_ITEMS,
                    },
                  )
                : t(
                    "this judge's agreement with human labels has not been measured for this rubric",
                    "why a judge's scores are not published",
                  );
              return (
                <p
                  key={label(r)}
                  style={{ fontSize: "0.9rem", color: "var(--ifm-color-emphasis-700)" }}
                >
                  <strong>{label(r)}</strong>: judged by {j.provider}:{j.model}, but the scores are{" "}
                  <strong>not published</strong> — {reason}. Publishing an unvalidated judge&rsquo;s
                  scores would report the judge&rsquo;s opinion as the model&rsquo;s behavior.
                </p>
              );
            }
            const b = rate(j.bare);
            const s = rate(j.steered);
            const l = b != null && s != null ? s - b : null;
            const validatedOn = v!.targets?.length
              ? t(", human-validated on {targets}", "judge validation note", {
                  targets: v!.targets.join(", "),
                })
              : "";
            const trustedOn =
              v!.targets?.length && !v!.targets.includes(r.target)
                ? t(" — trusted, not validated, on {target}", "judge validation note", {
                    target: r.target,
                  })
                : "";
            return (
              <p key={label(r)} style={{ fontSize: "0.9rem" }}>
                <strong>{label(r)}</strong>: bare {b?.toFixed(1)}% · steered {s?.toFixed(1)}%
                {l != null && (
                  <>
                    {" "}
                    · lift <strong>{pp(l)}</strong>
                  </>
                )}{" "}
                <span style={{ color: "var(--ifm-color-emphasis-600)" }}>
                  (judge {j.provider}:{j.model}, agreement kappa {v!.kappa.toFixed(2)} over{" "}
                  {v!.items} verdicts
                  {validatedOn}
                  {trustedOn})
                </span>
              </p>
            );
          })}
        </>
      )}
    </section>
  );
}

function OverTime(): ReactElement | null {
  // The over-time view lives inside one (target, digest) experiment — a trend
  // across corpora would compare different experiments and call it drift.
  const sections = experiments
    .map((e) => {
      const comparable = real.filter(
        (r) => r.target === e.target && r.corpus_digest === e.digest && scored(r),
      );
      const dates = [...new Set(comparable.map((r) => r.date))].sort();
      return { e, comparable, dates };
    })
    .filter((s) => s.dates.length > 1);
  if (!sections.length) return null;

  return (
    <>
      <h2>Over time</h2>
      <p>
        Overall lift by run date, per model, within one experiment. A model whose steerability
        regresses behind a stable alias shows up here — which is the point of keeping the record
        rather than measuring once.
      </p>
      {sections.map(({ e, comparable, dates }) => (
        <div key={e.target} style={{ marginBottom: 20 }}>
          <h3>Target {e.target}</h3>
          <table style={{ width: "100%", borderCollapse: "collapse", fontSize: "0.9rem" }}>
            <thead>
              <tr>
                <th style={left}>Model</th>
                {dates.map((d) => (
                  <th key={d} style={cell}>
                    {d}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {[...new Set(comparable.map(label))].map((m) => (
                <tr key={m}>
                  <td style={left}>{m}</td>
                  {dates.map((d) => {
                    const run = comparable.find((r) => label(r) === m && r.date === d);
                    const l = run ? lift(overall(run)) : null;
                    return (
                      <td key={d} style={cell}>
                        {l != null ? pp(l) : "—"}
                      </td>
                    );
                  })}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ))}
    </>
  );
}

export default function ContextEval(): ReactElement {
  const f = findings();
  const anyRuns = experiments.some((e) => e.runs.length > 0);
  return (
    <Layout
      title="Context eval"
      description="Whether models actually follow the context kapi injects — terms, voice profile, instruction — measured as lift: adherence with the context minus adherence without it, per model, per dimension, tracked over time."
    >
      <main style={{ maxWidth: 940, margin: "0 auto", padding: "2.5rem 1.25rem 4rem" }}>
        <h1>Context eval</h1>
        <p style={{ fontSize: "1.05rem", color: "var(--ifm-color-emphasis-700)" }}>
          kapi steers model output by injecting context: a terms store that mandates renderings, a
          voice guide, an instruction. This page measures whether each model actually follows that
          context — not whether it translates well, which is a different question. The sibling{" "}
          <Link to="/batch-eval">batch eval</Link> measures structural integrity and cost; this one
          measures obedience.
        </p>

        <h2>How it is measured</h2>
        <p>
          The core metric is a <strong>differential</strong>. An engineered corpus is translated
          twice through the production pipeline — once <em>bare</em> (no context) and once{" "}
          <em>steered</em> (terms + voice profile + instruction, injected exactly as production
          injects them) — and both passes are scored with kapi&rsquo;s own deterministic check tools
          (term-check, dnt-check, voice-vocab-check, pattern-check). Two numbers fall out per
          dimension: <strong>absolute adherence</strong> (did the steered output satisfy the
          requirement) and <strong>lift</strong> (steered minus bare — how much the context moved
          the model). Lift is the decision-relevant one: a model with high absolute adherence but no
          lift already &ldquo;knew&rdquo; it, and our context earns no credit. A model with high
          lift is genuinely <em>steerable</em>, which is what context injection is buying.
        </p>
        <p>
          Every fixture is a trap: a naive translation violates the context. A mandated term whose
          natural rendering differs from the mandate, a product name that reads like a common noun,
          casual English tempting an informal register, a source that ends in the exclamation mark
          the instruction forbids. There are distractors (a lowercase &ldquo;compass&rdquo; that is
          a real compass and must be translated) and declared-winner conflicts (the terms store pins
          a compound containing a forbidden word — the pin wins, and the scorer knows it). Results
          are reported <strong>per dimension</strong> — terminology, voice, instruction — never as
          one collapsed score: a model can be excellent at terminology and poor at voice, and the
          collapsed number would hide the thing you would act on.
        </p>
        <p>
          The deterministic checks are the backbone. The genuinely subjective remainder of voice —
          register, naturalness, restraint — is scored by a cross-family LLM judge under a fixed
          yes/no rubric, blind to which model and which variant produced a text, and its scores are
          published only once judge–human agreement has been measured above a stated bar. Until
          then, judged numbers stay in the record but off this page.
        </p>

        {!anyRuns ? (
          <p>
            <em>
              No real-model runs on record yet. Run <code>make context-eval-publish</code>.
            </em>
          </p>
        ) : (
          <>
            <h2>What the measurement found</h2>
            <p>
              {f.bestLift && (
                <>
                  The most steerable model measured is <strong>{f.bestLift.model}</strong> (
                  {f.bestLift.target}
                  ), which the context moved by <strong>{pp(f.bestLift.lift)}</strong> overall.{" "}
                </>
              )}
              {f.deadWeight.length > 0 ? (
                <>
                  For{" "}
                  {f.deadWeight.map((d, i) => (
                    <span key={`${d.model}-${d.target}`}>
                      {i > 0 ? ", " : ""}
                      <strong>{d.model}</strong> ({d.target})
                    </span>
                  ))}{" "}
                  the context produced no positive lift — on those models the injected guide is dead
                  weight at this corpus, a cost with no measured return.
                </>
              ) : (
                <>
                  Every measured model shows positive lift — the context earns its tokens
                  everywhere.
                </>
              )}
              {f.weakestDim && (
                <>
                  {" "}
                  The weakest dimension with context applied is{" "}
                  <strong>{f.weakestDim.dimension}</strong> at {f.weakestDim.rate.toFixed(1)}%
                  steered adherence — the ceiling context injection currently hits.
                </>
              )}
            </p>
            {experiments.map((e) => (
              <ExperimentSection key={e.target} e={e} />
            ))}
            <OverTime />
          </>
        )}

        <h2>Reproducing this</h2>
        <p>
          The harness is <code>scripts/contexteval</code>. A run against the built-in demo stub
          exercises the harness and measures nothing about any model; such runs are marked{" "}
          <code>simulated</code> and are excluded from every chart on this page. Adherence varies by
          target language, so the published sweep covers more than one.
        </p>
        <pre>
          <code>{`make context-eval                    # demo stub: proves the harness, measures nothing
make context-eval-publish            # the real sweep → this page's data`}</code>
        </pre>
      </main>
    </Layout>
  );
}
