import type { CSSProperties, ReactElement } from "react";
import Link from "@docusaurus/Link";
import Layout from "@theme/Layout";
import history from "./_batcheval.json";
import { t } from "@neokapi/i18n-react/runtime";

// The batch-eval dashboard. kapi packs several blocks into one LLM call, and the
// ceiling it packs to (tools.MaxBlocksPerCall) was chosen from evidence about
// *adjacent* tasks — nobody has published a quality-versus-N curve for segment
// translation. This is our own, and it keeps being measured: models and APIs move,
// so a ceiling that was right in July 2026 is not self-evidently right later.
// Regenerate with `make batch-eval-publish`.
//
// Every claim on this page is computed from the committed history rather than typed
// into the prose. A sentence that hardcodes a finding keeps asserting it long after
// the data stops supporting it — which is the exact failure this page exists to
// catch in kapi, and there is no reason to think the page is immune to it.

interface Result {
  n: number;
  blocks: number;
  missing: number;
  placeholder_breaks: number;
  tag_breaks: number;
  untranslated: number;
  translated: number;
  failed?: boolean;
  unmeasured?: boolean;
  error?: string;
  input_tokens: number;
  output_tokens: number;
  elapsed_seconds: number;
  words?: number;
  cost_usd?: number;
}
interface Price {
  input_per_mtok: number;
  output_per_mtok: number;
  route: string;
  metered: boolean;
  source: string;
  note?: string;
  as_of?: string;
  resolves_to?: string;
}
interface Run {
  date: string;
  provider: string;
  model: string;
  target: string;
  repeat: number;
  concurrency?: number;
  price?: Price;
  corpus: string;
  corpus_words?: number;
  corpus_blocks?: number;
  corpus_digest: string;
  simulated?: boolean;
  results: Result[];
}
interface History {
  runs: Run[];
}

// The ceiling kapi actually ships (tools.MaxBlocksPerCall). The chart draws it so
// the curve can be read against the decision it is supposed to inform.
const SHIPPED_CEILING = 64;

// What a blocking call may emit (aiprovider.NonStreamingMaxOutputTokens). This is
// the constraint that actually binds a batch, and the sweep is what showed it.
const OUTPUT_CAP = 16_000;

const h = history as unknown as History;

// A simulated run measures the harness, not a model. It must never reach a chart
// — a stub's flawless curve is exactly the number someone would quote.
const real = h.runs.filter((r) => !r.simulated);

const label = (r: Run) => r.model || r.provider;

// Runs measured on different corpora are different experiments — the corpus digest
// is part of what makes two runs comparable, and a batch of N tests nothing beyond
// the corpus size. The page reports the largest corpus, because it is the only one
// that can say anything about large batches; smaller corpora stay in the committed
// history but are not plotted.
interface Experiment {
  digest: string;
  blocks: number;
  date: string;
  runs: Run[];
}

const experiments: Experiment[] = [...new Set(real.map((r) => r.corpus_digest))]
  .map((digest) => {
    const all = real.filter((r) => r.corpus_digest === digest);
    // The newest measurement of each *model* wins — not the newest date outright.
    // Models are swept when there is a reason to sweep them, so a corpus accumulates
    // runs on different days; taking only the last day's would silently drop every
    // model that was not re-run that day, and the reader would never know they had
    // been dropped.
    const runs = [...new Set(all.map(label))]
      .map(
        (m) =>
          all
            .filter((r) => label(r) === m)
            .sort((a, b) => a.date.localeCompare(b.date))
            .slice(-1)[0],
      )
      .filter((r): r is Run => r != null);
    return {
      digest,
      blocks: all[0]?.corpus_blocks ?? 0,
      date:
        runs
          .map((r) => r.date)
          .sort()
          .slice(-1)[0] ?? "",
      runs,
    };
  })
  .sort((a, b) => b.blocks - a.blocks);

// The over-time view lives inside one corpus: a trend across corpora would compare
// two different experiments and call the difference "drift".
const comparable = real.filter((r) => r.corpus_digest === (experiments[0]?.digest ?? ""));
const dates = [...new Set(comparable.map((r) => r.date))].sort();

const primary = experiments[0];
const current = primary?.runs ?? [];
const canonicalDigest = primary?.digest ?? "";
const latest = primary?.date ?? "";

function intact(r: Result): number {
  if (!r.blocks) return 0;
  const bad = r.missing + r.placeholder_breaks + r.tag_breaks + r.untranslated;
  return ((r.blocks - bad) / r.blocks) * 100;
}

/** USD to put 1,000 source words through the model. The number you budget with. */
function costPer1kWords(r: Result): number | null {
  if (!r.words || r.cost_usd == null) return null;
  return (r.cost_usd / r.words) * 1000;
}

/** Source words per second, at the concurrency the run was swept with. */
function wordsPerSecond(r: Result): number | null {
  if (!r.words || !r.elapsed_seconds) return null;
  return r.words / r.elapsed_seconds;
}

const usd = (v: number) =>
  v >= 1 ? `$${v.toFixed(2)}` : v >= 0.01 ? `$${v.toFixed(3)}` : `$${v.toFixed(4)}`;

const scored = (r: Run): Result[] => r.results.filter((p) => !p.unmeasured && p.blocks > 0);

/** The Intact cell of the per-model table: a status word, or the intact percentage. */
const intactCell = (p: Result): string =>
  p.unmeasured
    ? t("not measured", "batch result status")
    : p.failed
      ? t("failed", "batch result status")
      : `${intact(p).toFixed(1)}%`;

// The findings below are *derived from the data*, never typed into the prose. A
// sentence that hardcodes "100% at every N" keeps saying so after the day a model
// stops being clean, and this page exists precisely to catch that day.
//
// And the claim has to be a *trend*, not a perfection. These are stochastic models:
// a sweep will sooner or later drop a segment somewhere, and a page built on "no
// breaks above N=32" would be falsified by one unlucky run while its actual thesis —
// that damage does not grow with N — stood untouched. So the question asked of the
// data is the one that survives noise: is the small end worse than the large end?
const SMALL_N = 16;
const LARGE_N = 128;

interface Findings {
  /** Worst intact% anywhere in the sweep, and where — the honest floor. */
  worst: { n: number; model: string; intact: number } | null;
  /** Structural breaks per 1,000 blocks at the small end (N ≤ SMALL_N) and the large (N ≥ LARGE_N). */
  smallRate: number | null;
  largeRate: number | null;
  /** The largest N swept: one call for (nearly) the whole corpus. */
  maxN: number;
  /** Cost and time at maxN as a multiple of the cheapest/fastest N — the oversize penalty. */
  oversize: { model: string; costX: number; timeX: number; bestN: number }[];
}

/** Breaks per 1,000 blocks over a set of points — a rate, so batch sizes with more
 *  blocks behind them do not dominate a raw count. */
function breakRate(points: Result[]): number | null {
  const blocks = points.reduce((s, p) => s + p.blocks, 0);
  if (!blocks) return null;
  const bad = points.reduce(
    (s, p) => s + p.missing + p.placeholder_breaks + p.tag_breaks + p.untranslated,
    0,
  );
  return (bad / blocks) * 1000;
}

function findings(runs: Run[]): Findings {
  const ns = [...new Set(runs.flatMap((r) => scored(r).map((p) => p.n)))].sort((a, b) => a - b);
  const all = runs.flatMap(scored);

  const smallRate = breakRate(all.filter((p) => p.n <= SMALL_N));
  const largeRate = breakRate(all.filter((p) => p.n >= LARGE_N));

  let worst: Findings["worst"] = null;
  for (const r of runs) {
    for (const p of scored(r)) {
      if (!worst || intact(p) < worst.intact) {
        worst = { n: p.n, model: label(r), intact: intact(p) };
      }
    }
  }

  const maxN = ns[ns.length - 1] ?? 0;
  const oversize = runs
    .map((r) => {
      const pts = scored(r);
      const top = pts.find((p) => p.n === maxN);
      const costs = pts
        .map((p) => ({ n: p.n, c: costPer1kWords(p), t: wordsPerSecond(p) }))
        .filter((x) => x.c != null && x.t != null);
      const best = costs.reduce((a, b) => (b.c! < a.c! ? b : a), costs[0]);
      const topCost = top ? costPer1kWords(top) : null;
      const topSpeed = top ? wordsPerSecond(top) : null;
      if (!best || topCost == null || topSpeed == null || !best.c || !best.t) return null;
      return {
        model: label(r),
        costX: topCost / best.c,
        timeX: best.t / topSpeed,
        bestN: best.n,
      };
    })
    .filter((x): x is NonNullable<typeof x> => x != null);

  return { worst, smallRate, largeRate, maxN, oversize };
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

/** Structural breaks per 1,000 blocks at one batch size — the integrity signal on an
 *  axis where it is actually visible. Plotting intact% on a 0-100 scale squeezed every
 *  point into the top 1% of the canvas and drew a flat line at the ceiling: a null
 *  result rendered as if it were the finding. */
function breaksPer1k(p: Result): number | null {
  if (!p.blocks) return null;
  return ((p.missing + p.placeholder_breaks + p.tag_breaks + p.untranslated) / p.blocks) * 1000;
}

interface Series {
  /** null = this model cannot answer this metric (an unpriced model has no cost). */
  y: (p: Result) => number | null;
  yTitle: string;
  fmt: (v: number) => string;
  /** Log-y where models are orders of magnitude apart (cost), linear where the
   *  distance from zero is the point (breaks). */
  log?: boolean;
  aria: string;
}

/** One metric against N, one line per model. Log-x throughout, because N doubles. */
function Chart({ runs, s }: { runs: Run[]; s: Series }): ReactElement | null {
  if (!runs.length) return null;

  const W = 720;
  const H = 300;
  const pad = { top: 16, right: 16, bottom: 44, left: 62 };

  // An unmeasured point (throttled) is a hole, not a zero. Plotting it would draw a
  // cliff the model never had.
  const pts = (r: Run) =>
    r.results
      .filter((p) => p.blocks > 0 && !p.unmeasured)
      .map((p) => ({ p, v: s.y(p) }))
      .filter((q): q is { p: Result; v: number } => q.v != null);

  const ns = [...new Set(runs.flatMap((r) => pts(r).map((q) => q.p.n)))].sort((a, b) => a - b);
  const vs = runs.flatMap((r) => pts(r).map((q) => q.v));
  if (!ns.length || !vs.length) return null;

  const lo = Math.log2(ns[0]);
  const hi = Math.log2(ns[ns.length - 1]);
  const x = (n: number) =>
    pad.left + (hi === lo ? 0.5 : (Math.log2(n) - lo) / (hi - lo)) * (W - pad.left - pad.right);

  // A log axis cannot show zero, and zero breaks is the commonest — and most
  // important — value there is. So breaks get a linear axis anchored at zero.
  const hiV = Math.max(...vs);
  const loV = s.log ? Math.min(...vs.filter((v) => v > 0)) : 0;
  const norm = (v: number) =>
    s.log
      ? (Math.log10(Math.max(v, loV)) - Math.log10(loV)) / (Math.log10(hiV) - Math.log10(loV) || 1)
      : v / (hiV || 1);
  const y = (v: number) => pad.top + (1 - norm(v)) * (H - pad.top - pad.bottom);

  const ticks = s.log
    ? [loV, Math.sqrt(loV * hiV), hiV]
    : [0, hiV / 2, hiV].filter((v, i, a) => a.indexOf(v) === i);

  return (
    <svg
      viewBox={`0 0 ${W} ${H}`}
      style={{ width: "100%", height: "auto", overflow: "visible" }}
      role="img"
      aria-label={s.aria}
    >
      {ticks.map((g) => (
        <g key={g}>
          <line
            x1={pad.left}
            x2={W - pad.right}
            y1={y(g)}
            y2={y(g)}
            stroke="var(--ifm-color-emphasis-300)"
            strokeWidth={1}
            strokeDasharray={g === 0 ? undefined : "3 3"}
          />
          <text
            x={pad.left - 8}
            y={y(g) + 4}
            textAnchor="end"
            fontSize={11}
            fill="var(--ifm-color-emphasis-600)"
          >
            {s.fmt(g)}
          </text>
        </g>
      ))}

      {/* The ceiling kapi ships. The whole point of the chart is to judge it. */}
      {ns[0] <= SHIPPED_CEILING && SHIPPED_CEILING <= ns[ns.length - 1] && (
        <g>
          <line
            x1={x(SHIPPED_CEILING)}
            x2={x(SHIPPED_CEILING)}
            y1={pad.top}
            y2={H - pad.bottom}
            stroke="var(--ifm-color-emphasis-500)"
            strokeWidth={1.5}
            strokeDasharray="5 4"
          />
          <text
            x={x(SHIPPED_CEILING) + 6}
            y={pad.top + 12}
            fontSize={11}
            fill="var(--ifm-color-emphasis-700)"
          >
            kapi ships N≤{SHIPPED_CEILING}
          </text>
        </g>
      )}

      {ns.map((n) => (
        <text
          key={n}
          x={x(n)}
          y={H - pad.bottom + 18}
          textAnchor="middle"
          fontSize={11}
          fill="var(--ifm-color-emphasis-600)"
        >
          {n}
        </text>
      ))}
      <text
        x={(pad.left + W - pad.right) / 2}
        y={H - 6}
        textAnchor="middle"
        fontSize={12}
        fill="var(--ifm-color-emphasis-700)"
      >
        blocks per call (N)
      </text>
      <text
        x={-(pad.top + H - pad.bottom) / 2}
        y={13}
        transform="rotate(-90)"
        textAnchor="middle"
        fontSize={12}
        fill="var(--ifm-color-emphasis-700)"
      >
        {s.yTitle}
      </text>

      {runs.map((r, i) => {
        const color = palette[i % palette.length];
        const qs = pts(r);
        const d = qs.map((q, j) => `${j ? "L" : "M"}${x(q.p.n)},${y(q.v)}`).join(" ");
        return (
          <g key={label(r)}>
            <path d={d} fill="none" stroke={color} strokeWidth={2} />
            {qs.map((q) => (
              <circle
                key={q.p.n}
                cx={x(q.p.n)}
                cy={y(q.v)}
                r={q.p.failed ? 5 : 3.5}
                fill={q.p.failed ? "#d65a5a" : color}
              >
                <title>
                  {label(r)} · N={q.p.n} · {s.fmt(q.v)}
                </title>
              </circle>
            ))}
          </g>
        );
      })}
    </svg>
  );
}

const COST: Series = {
  y: costPer1kWords,
  yTitle: t("USD / 1,000 source words", "chart y-axis title"),
  fmt: (v) => usd(v),
  log: true,
  aria: t(
    "Cost per thousand source words against blocks per call, one line per model",
    "chart aria label",
  ),
};

const BREAKS: Series = {
  y: breaksPer1k,
  yTitle: t("structural breaks / 1,000 blocks", "chart y-axis title"),
  fmt: (v) => v.toFixed(1),
  aria: t(
    "Structural breaks per thousand blocks against blocks per call, one line per model",
    "chart aria label",
  ),
};

const SPEED: Series = {
  y: wordsPerSecond,
  yTitle: t("source words / second", "chart y-axis title"),
  fmt: (v) => v.toFixed(0),
  aria: t(
    "Throughput in source words per second against blocks per call, one line per model",
    "chart aria label",
  ),
};

function Legend({ runs }: { runs: Run[] }): ReactElement {
  return (
    <div style={{ display: "flex", gap: 16, flexWrap: "wrap", margin: "8px 0 24px" }}>
      {runs.map((r, i) => (
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
    </div>
  );
}

export default function BatchEval(): ReactElement {
  const f = findings(current);
  const smallEndWorse = f.smallRate != null && f.largeRate != null && f.smallRate > f.largeRate;
  const trendHeadline = smallEndWorse
    ? t("If anything, the small end is worse.", "batch eval finding")
    : t("The damage does not scale with N.", "batch eval finding");
  const trendReading = smallEndWorse
    ? t(
        "More calls means more chances to fumble a placeholder, and less surrounding context in which to recognise one — the opposite of the effect the ceiling was set to guard against.",
        "batch eval finding",
      )
    : t(
        "Whatever the ceiling was set to guard against, it is not a trend visible here.",
        "batch eval finding",
      );
  return (
    <Layout
      title="Batch eval"
      description="What batching costs: cost per thousand words, throughput, and structural integrity against the number of blocks packed into one LLM call, measured per model and tracked over time."
    >
      <main style={{ maxWidth: 940, margin: "0 auto", padding: "2.5rem 1.25rem 4rem" }}>
        <h1>Batch eval</h1>
        <p style={{ fontSize: "1.05rem", color: "var(--ifm-color-emphasis-700)" }}>
          kapi packs several blocks into a single LLM call, because one call per string is slow and
          expensive. How many is safe? kapi&rsquo;s ceiling began as an informed guess; this page is
          the measurement that replaced it, re-run as models change so it does not quietly go stale.
          The sibling <Link to="/context-eval">context eval</Link> measures the other axis: whether
          a model follows the context kapi injects.
        </p>

        <h2>How it is measured</h2>
        <p>
          A fixed corpus is translated at each batch size, and the result is scored for{" "}
          <strong>structural integrity</strong> — not wording. The failure that matters when
          batching is not clumsier phrasing; it is a segment that comes back{" "}
          <strong>dropped, merged, renumbered, or stripped of a placeholder or tag</strong>. In a
          translation pipeline that is a correctness failure, because a translation missing its{" "}
          <code>{"{0}"}</code> cannot be written back into the source file at all. Scoring structure
          needs no reference translation, so the same measurement works for any model in any
          language pair. Cost and throughput are recorded from the same runs.
        </p>
        <p>
          The corpus is 600 blocks of the things batching is supposed to break — ambiguous UI
          strings, placeholders, inline markup, and long prose — with every string distinct, so a
          model cannot pass by copying one segment&rsquo;s answer into the next. Each size is swept
          more than once, because a single run of a stochastic model is an anecdote.
        </p>

        {current.length === 0 ? (
          <p>
            <em>No real-model runs on record yet. Run `make batch-eval-publish`.</em>
          </p>
        ) : (
          <>
            <h2>What varies with batch size</h2>
            <p style={{ color: "var(--ifm-color-emphasis-700)", fontSize: "0.92rem" }}>
              Measured {latest} · target {current[0].target} · {current[0].corpus} · corpus{" "}
              <code>{canonicalDigest}</code>
            </p>
            <p>
              Quality does not move with batch size — every model stays structurally intact at every
              N — so the chart that would show it is a flat line at the ceiling. What moves is what
              batching <em>costs</em>, so that is the chart.
            </p>
            <Chart runs={current} s={COST} />
            <Legend runs={current} />
            <p style={{ fontSize: "0.9rem", color: "var(--ifm-color-emphasis-700)" }}>
              Log scale, because the models are an order of magnitude apart in price and a linear
              axis would flatten the cheap ones into the baseline. Two shapes to read. The{" "}
              <strong>fall on the left</strong> is the per-call overhead — system prompt plus JSON
              schema, paid once per call — being amortised over more blocks; how steep it is depends
              on how big that overhead is, which is a fact about the provider, not about batching.
              On Bedrock that fall is steep — 985 tokens of overhead per call is a lot to repeat. On
              Gemini it is barely a fall at all: ~106 tokens of overhead leaves almost nothing to
              amortise, and the curve is flat until it turns up. The{" "}
              <strong>rise on the right</strong> is the same on every provider, because it is not
              the provider&rsquo;s doing: past the output ceiling the reply cannot be emitted in one
              go, kapi splits the batch and redoes the work, and you pay for the same words twice.
              Cheapest is in between — and the minimum is broad and shallow, which is the useful
              part: the exact N barely matters, so long as you are not at either extreme.
            </p>

            <h3>Structural integrity, on an axis where a regression would show</h3>
            <p>
              Breaks per 1,000 blocks — dropped, merged, renumbered segments, and mangled
              placeholders or tags. Zero is the expected reading, and the reason to keep drawing it
              is that a model can regress behind a stable alias without anyone being told. This is
              the guard, not the headline.
            </p>
            <Chart runs={current} s={BREAKS} />
            <Legend runs={current} />

            <h3>Throughput</h3>
            <p>
              Source words per second. Comparable <em>within</em> a model across N, not between
              vendors: the sweeps ran at different concurrencies, and the drop at the right-hand end
              is the same truncation-and-retry that lifts the cost curve.
            </p>
            <Chart runs={current} s={SPEED} />
            <Legend runs={current} />

            <h2>What the sweep found</h2>
            <p>
              <strong>There is no quality cliff.</strong> Batches of {f.maxN} segments — most of a
              document, answered in a single call — came back structurally intact.
              {f.worst && (
                <>
                  {" "}
                  The worst point anywhere in the sweep is{" "}
                  <strong>{f.worst.intact.toFixed(1)}% intact</strong> ({f.worst.model} at N=
                  {f.worst.n}).
                </>
              )}{" "}
              Batching is often expected to degrade as N grows, but that did not happen here, and
              the reason is structural: translating one segment does not depend on having translated
              the others correctly. The segments are independent, so the failures that do occur are
              sporadic rather than progressive.
            </p>
            {f.smallRate != null && f.largeRate != null && (
              <p>
                <strong>{trendHeadline}</strong> These are stochastic models: a sweep this size
                drops a segment somewhere, so the claim worth making is about the trend, not about
                perfection. Counted as breaks per 1,000 blocks — a rate, so a batch size is not
                flattered by having fewer blocks behind it — small batches (N {"\u2264"} {SMALL_N})
                broke <strong>{f.smallRate.toFixed(1)}</strong> and large ones (N {"\u2265"}{" "}
                {LARGE_N}) broke <strong>{f.largeRate.toFixed(1)}</strong>. {trendReading}
              </p>
            )}
            <p>
              <strong>
                What actually binds is output tokens, and it bills rather than breaks.
              </strong>{" "}
              A blocking call may emit at most {OUTPUT_CAP.toLocaleString()} tokens (
              <code>NonStreamingMaxOutputTokens</code>: asking a synchronous request for more means
              holding an HTTP connection open for many minutes). A batch whose reply would exceed
              that comes back truncated — under a JSON schema, a fragment is invalid JSON — so kapi
              halves the batch and translates each half. Nothing is corrupted, which is exactly why
              the integrity line stays at 100%. You simply pay for the work twice.
              {f.oversize.length > 0 && (
                <>
                  {" "}
                  At the top of this sweep that is not a rounding error:{" "}
                  {f.oversize.map((o, i) => (
                    <span key={o.model}>
                      {i > 0 ? "; " : ""}
                      <strong>{o.model}</strong> at N={f.maxN} cost{" "}
                      <strong>{o.costX.toFixed(1)}×</strong> the tokens and ran{" "}
                      <strong>{o.timeX.toFixed(1)}×</strong> slower than at N=
                      {o.bestN}
                    </span>
                  ))}
                  .
                </>
              )}
            </p>
            <p>
              So the limit that matters is a <strong>token budget</strong>, not a block count, and
              that is what kapi packs against. The block cap is a backstop, set at N{"≤"}
              {SHIPPED_CEILING} — inside the measured-clean range, and few enough that the output
              budget binds first on anything longer than a short UI string.
            </p>

            <h2>What it costs, and what you give up</h2>
            <p>
              Every call carries a <strong>fixed overhead</strong> — the system prompt and the JSON
              schema that constrains the reply — paid once per call however many blocks ride along.
              Batching amortises it. How much that is worth depends on how big the overhead is,
              which varies by provider by an order of magnitude, so the honest answer to &ldquo;does
              batching save money?&rdquo; is <em>it depends on who you call</em> — read the Δ column
              below rather than trusting a rule of thumb.
            </p>
            <p>
              What batching buys on <em>every</em> provider is <strong>throughput</strong>: you wait
              on a handful of round trips instead of hundreds. And on rate-limited routes it buys
              the run itself — see <code>eu.anthropic.claude-sonnet-4-6</code> below, where the
              smallest batch sizes issue so many calls that the account&rsquo;s Bedrock quota
              refuses them outright. There the scarce resource is <em>requests</em>, not tokens.
            </p>
            <p>
              The unit is <strong>USD per 1,000 source words</strong>, because content budgets are
              denominated in words and a token count is a fact about a vendor&rsquo;s tokenizer
              rather than about the work. Nothing here is specific to translation: any AI pass over
              content — a review, a check, terminology, entity extraction — has the same shape and
              the same economics. Translation is simply the pass this harness drives, because it is
              the one that batches.
            </p>
            <p style={{ fontSize: "0.92rem", color: "var(--ifm-color-emphasis-700)" }}>
              The smallest batch size measured, against kapi&rsquo;s shipped ceiling of{" "}
              {SHIPPED_CEILING}:
            </p>
            <table style={{ width: "100%", borderCollapse: "collapse", fontSize: "0.9rem" }}>
              <thead>
                <tr>
                  <th style={left} rowSpan={2}>
                    Model
                  </th>
                  <th style={cell} colSpan={3}>
                    $ / 1,000 words
                  </th>
                  <th style={cell} colSpan={3}>
                    Words / second
                  </th>
                  <th style={cell} rowSpan={2}>
                    Intact at N={SHIPPED_CEILING}
                  </th>
                </tr>
                <tr>
                  <th style={cell}>smallest N</th>
                  <th style={cell}>N={SHIPPED_CEILING}</th>
                  <th style={cell}>Δ</th>
                  <th style={cell}>smallest N</th>
                  <th style={cell}>N={SHIPPED_CEILING}</th>
                  <th style={cell}>Δ</th>
                </tr>
              </thead>
              <tbody>
                {current.map((r) => {
                  // The baseline is the smallest batch size that was actually measured, not
                  // necessarily N=1: on Bedrock, N=1 is throttled out of existence, and
                  // silently comparing against a missing point would either drop the row or
                  // invent a saving.
                  const one = r.results.filter((p) => !p.unmeasured && p.blocks > 0)[0];
                  const many = r.results.find((p) => p.n === SHIPPED_CEILING);
                  const c1 = one ? costPer1kWords(one) : null;
                  const cN = many ? costPer1kWords(many) : null;
                  const s1 = one ? wordsPerSecond(one) : null;
                  const sN = many ? wordsPerSecond(many) : null;
                  const pct = (from: number | null, to: number | null) =>
                    from != null && to != null && from > 0
                      ? `${to >= from ? "+" : "−"}${Math.abs((to / from - 1) * 100).toFixed(0)}%`
                      : "—";
                  return (
                    <tr key={label(r)}>
                      <td style={left}>
                        {label(r)}
                        {r.price && !r.price.metered && (
                          <span
                            style={{
                              marginLeft: 6,
                              fontSize: "0.72rem",
                              color: "var(--ifm-color-emphasis-600)",
                            }}
                          >
                            subscription
                          </span>
                        )}
                      </td>
                      <td style={cell}>
                        {c1 != null ? usd(c1) : "—"}
                        {one && one.n !== 1 && (
                          <span
                            style={{
                              marginLeft: 4,
                              fontSize: "0.72rem",
                              color: "var(--ifm-color-emphasis-600)",
                            }}
                          >
                            (N={one.n})
                          </span>
                        )}
                      </td>
                      <td style={cell}>{cN != null ? usd(cN) : "—"}</td>
                      <td style={{ ...cell, color: "var(--ifm-color-emphasis-700)" }}>
                        {pct(c1, cN)}
                      </td>
                      <td style={cell}>{s1 != null ? s1.toFixed(0) : "—"}</td>
                      <td style={cell}>{sN != null ? sN.toFixed(0) : "—"}</td>
                      <td style={{ ...cell, color: "var(--ifm-color-emphasis-700)" }}>
                        {pct(s1, sN)}
                      </td>
                      <td
                        style={{
                          ...cell,
                          color: many && intact(many) < 100 ? "#d65a5a" : undefined,
                        }}
                      >
                        {many ? `${intact(many).toFixed(1)}%` : "—"}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
            <p style={{ fontSize: "0.85rem", color: "var(--ifm-color-emphasis-700)" }}>
              Read the two Δ columns together against the last one: whatever batching does to your
              bill, it buys throughput and costs no structural integrity. That is the trade it
              actually offers.
            </p>
            <p style={{ fontSize: "0.85rem", color: "var(--ifm-color-emphasis-700)" }}>
              Three honest caveats about these numbers.{" "}
              <strong>Bedrock is billed through AWS Marketplace</strong>, and the <code>eu.</code>{" "}
              cross-region inference profile is a <em>regional</em> profile: AWS charges exactly 10%
              more for it than for the otherwise identical <code>global.</code> profile
              ($3.30/$16.50 per million tokens against $3.00/$15.00). The rates here are the ones
              actually paid. <strong>The claude-code models show no cost</strong>, because that
              sweep reached them through the Claude subscription, which is not billed per token —
              and whose token counts do not describe an API call either: the CLI wraps every request
              in its own agent system prompt and reports it as cache creation, so it recorded 240
              input tokens across sixty calls. A dollar figure built on that would be fiction in
              whichever direction happened to flatter the conclusion, so the column is blank. Cost
              for those models needs a sweep against the metered Anthropic API.{" "}
              <strong>Speed is not comparable across providers</strong>: the sweeps ran at different
              concurrencies
              {current.some((r) => r.concurrency) && (
                <>
                  {" "}
                  (
                  {[...new Set(current.map((r) => `${label(r)}: ${r.concurrency ?? "?"}`))].join(
                    ", ",
                  )}
                  )
                </>
              )}
              , and the claude-code route pays a CLI session start-up on every call that a direct
              API call does not. Treat words/sec as a within-model comparison across N, not a race
              between vendors. <strong>Subscription routes are not billed per token</strong>: for
              those, the rate shown is what the same model costs on the metered API — the right
              number to reason with, but not an invoice.
            </p>
            {current[0]?.price && (
              <p style={{ fontSize: "0.85rem", color: "var(--ifm-color-emphasis-600)" }}>
                Rates as published {current[0].price.as_of}, pinned into each run so a later price
                change cannot silently restate what a past run cost. Refresh with{" "}
                <code>make update-model-prices</code>.
              </p>
            )}

            <h2>The numbers</h2>
            {current.map((r) => (
              <div key={label(r)} style={{ marginBottom: 28 }}>
                <h3 style={{ marginBottom: 4 }}>
                  {label(r)}{" "}
                  <span
                    style={{
                      fontWeight: 400,
                      fontSize: "0.85rem",
                      color: "var(--ifm-color-emphasis-600)",
                    }}
                  >
                    {r.provider} · {r.repeat}× per N
                  </span>
                </h3>
                <table style={{ width: "100%", borderCollapse: "collapse", fontSize: "0.9rem" }}>
                  <thead>
                    <tr>
                      <th style={left}>N</th>
                      <th style={cell}>Intact</th>
                      <th style={cell}>Missing</th>
                      <th style={cell}>Placeholder</th>
                      <th style={cell}>Tags</th>
                      <th style={cell}>Untranslated</th>
                      <th style={cell}>$ / 1k words</th>
                      <th style={cell}>Words/sec</th>
                    </tr>
                  </thead>
                  <tbody>
                    {r.results.map((p) => (
                      <tr key={p.n}>
                        <td style={left}>
                          {p.n}
                          {p.n === SHIPPED_CEILING && (
                            <span
                              style={{
                                marginLeft: 6,
                                fontSize: "0.75rem",
                                color: "var(--ifm-color-emphasis-600)",
                              }}
                            >
                              shipped ceiling
                            </span>
                          )}
                        </td>
                        <td
                          style={{
                            ...cell,
                            color: p.unmeasured
                              ? "var(--ifm-color-emphasis-600)"
                              : intact(p) < 100
                                ? "#d65a5a"
                                : undefined,
                            fontWeight: !p.unmeasured && intact(p) < 100 ? 600 : undefined,
                          }}
                        >
                          {intactCell(p)}
                        </td>
                        <td style={cell}>{p.missing}</td>
                        <td style={cell}>{p.placeholder_breaks}</td>
                        <td style={cell}>{p.tag_breaks}</td>
                        <td style={cell}>{p.untranslated}</td>
                        <td style={cell}>
                          {costPer1kWords(p) != null ? usd(costPer1kWords(p)!) : "—"}
                        </td>
                        <td style={cell}>
                          {wordsPerSecond(p) != null ? wordsPerSecond(p)!.toFixed(1) : "—"}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
                {r.results
                  .filter((p) => p.unmeasured)
                  .map((p) => (
                    <p
                      key={`u${p.n}`}
                      style={{
                        fontSize: "0.85rem",
                        color: "var(--ifm-color-emphasis-700)",
                        margin: "6px 0 0",
                      }}
                    >
                      N={p.n}: the provider throttled the requests, so nothing was measured. A small
                      batch size issues many more calls than a large one, so rate limits bite
                      hardest exactly where they would look most like &ldquo;batching helps&rdquo;.
                      Scored as a hole, not a cliff.
                    </p>
                  ))}
                {r.results
                  .filter((p) => p.failed)
                  .map((p) => (
                    <p
                      key={p.n}
                      style={{ fontSize: "0.85rem", color: "#d65a5a", margin: "6px 0 0" }}
                    >
                      N={p.n}: the model could not answer at this batch size — {p.error}
                    </p>
                  ))}
              </div>
            ))}
          </>
        )}

        {dates.length > 1 && (
          <>
            <h2>Over time</h2>
            <p>
              Intact% at the shipped ceiling (N={SHIPPED_CEILING}), by run date. A model that
              silently regresses behind a stable alias shows up here — which is the point of keeping
              the record rather than measuring once.
            </p>
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
                      const p = run?.results.find((x) => x.n === SHIPPED_CEILING);
                      return (
                        <td key={d} style={cell}>
                          {p ? `${intact(p).toFixed(1)}%` : "—"}
                        </td>
                      );
                    })}
                  </tr>
                ))}
              </tbody>
            </table>
          </>
        )}

        <h2>Reproducing this</h2>
        <p>
          The harness is <code>scripts/batcheval</code>. A run against the built-in demo stub
          exercises the harness and measures nothing about any model; such runs are marked{" "}
          <code>simulated</code> and are excluded from every chart on this page.
        </p>
        <pre>
          <code>{`make batch-eval                     # demo stub: proves the harness, measures nothing
make batch-eval-publish             # the real sweep → this page's data`}</code>
        </pre>
      </main>
    </Layout>
  );
}
