import type { CSSProperties, ReactElement } from "react";
import Layout from "@theme/Layout";
import history from "./_batcheval.json";

// The batch-eval dashboard. kapi packs several blocks into one LLM call, and the
// ceiling it packs to (tools.MaxBlocksPerCall) was chosen from evidence about
// *adjacent* tasks — nobody has published a quality-versus-N curve for segment
// translation. This measures our own, and keeps measuring it: models and APIs
// move, so a ceiling that was right in July 2026 is not self-evidently right
// later. Regenerate with `make batch-eval-publish`.

interface Result {
  n: number;
  blocks: number;
  missing: number;
  placeholder_breaks: number;
  tag_breaks: number;
  untranslated: number;
  translated: number;
  failed?: boolean;
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
  corpus_digest: string;
  simulated?: boolean;
  results: Result[];
}
interface History {
  runs: Run[];
}

// The ceiling kapi actually ships. The chart draws it so the curve can be read
// against the decision it is supposed to inform.
const SHIPPED_CEILING = 16;

const h = history as unknown as History;

// A simulated run measures the harness, not a model. It must never reach a chart
// — a stub's flawless curve is exactly the number someone would quote.
const real = h.runs.filter((r) => !r.simulated);

// Runs are only comparable if they were measured on the same corpus. Rather than
// silently plotting two experiments as one trend, take the newest corpus as
// canonical and say plainly what that excluded.
const latestDigest = real.length
  ? real
      .map((r) => r.corpus_digest)
      .sort()
      .slice(-1)[0]
  : "";
const canonicalDigest = real.length > 0 ? real[real.length - 1].corpus_digest : latestDigest;
const comparable = real.filter((r) => r.corpus_digest === canonicalDigest);
const stale = real.length - comparable.length;

const dates = [...new Set(comparable.map((r) => r.date))].sort();
const latest = dates[dates.length - 1] ?? "";
const current = comparable.filter((r) => r.date === latest);

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

const label = (r: Run) => r.model || r.provider;

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

/** Intact% against N, one line per model. Log-x, because N doubles. */
function Curve({ runs }: { runs: Run[] }): ReactElement | null {
  if (!runs.length) return null;

  const W = 720;
  const H = 320;
  const pad = { top: 16, right: 16, bottom: 44, left: 48 };
  const ns = [...new Set(runs.flatMap((r) => r.results.map((x) => x.n)))].sort((a, b) => a - b);
  if (!ns.length) return null;

  const lo = Math.log2(ns[0]);
  const hi = Math.log2(ns[ns.length - 1]);
  const x = (n: number) =>
    pad.left + (hi === lo ? 0.5 : (Math.log2(n) - lo) / (hi - lo)) * (W - pad.left - pad.right);
  const y = (v: number) => pad.top + (1 - v / 100) * (H - pad.top - pad.bottom);

  return (
    <svg
      viewBox={`0 0 ${W} ${H}`}
      style={{ width: "100%", height: "auto", overflow: "visible" }}
      role="img"
      aria-label="Structural integrity against blocks per call, one line per model"
    >
      {[0, 25, 50, 75, 100].map((g) => (
        <g key={g}>
          <line
            x1={pad.left}
            x2={W - pad.right}
            y1={y(g)}
            y2={y(g)}
            stroke="var(--ifm-color-emphasis-300)"
            strokeWidth={1}
            strokeDasharray={g === 100 ? undefined : "3 3"}
          />
          <text
            x={pad.left - 8}
            y={y(g) + 4}
            textAnchor="end"
            fontSize={11}
            fill="var(--ifm-color-emphasis-600)"
          >
            {g}%
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

      {runs.map((r, i) => {
        const color = palette[i % palette.length];
        const pts = r.results.filter((p) => p.blocks > 0);
        const d = pts.map((p, j) => `${j ? "L" : "M"}${x(p.n)},${y(intact(p))}`).join(" ");
        return (
          <g key={label(r)}>
            <path d={d} fill="none" stroke={color} strokeWidth={2} />
            {pts.map((p) => (
              <circle
                key={p.n}
                cx={x(p.n)}
                cy={y(intact(p))}
                r={p.failed ? 5 : 3.5}
                fill={p.failed ? "#d65a5a" : color}
              >
                <title>
                  {label(r)} · N={p.n} · {intact(p).toFixed(1)}% intact
                  {p.failed ? ` · failed: ${p.error ?? ""}` : ""}
                </title>
              </circle>
            ))}
          </g>
        );
      })}
    </svg>
  );
}

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
  return (
    <Layout
      title="Batch eval"
      description="What batching costs: structural integrity of translation against the number of blocks packed into one LLM call, measured per model and tracked over time."
    >
      <main style={{ maxWidth: 940, margin: "0 auto", padding: "2.5rem 1.25rem 4rem" }}>
        <h1>Batch eval</h1>
        <p style={{ fontSize: "1.05rem", color: "var(--ifm-color-emphasis-700)" }}>
          kapi packs several blocks into a single LLM call, because one call per string is slow and
          expensive. How many is safe? The published evidence covers <em>adjacent</em> tasks — batch
          prompting on classification, BatchGEMBA on MT evaluation — and no quality-versus-N curve
          has been published for segment translation. So kapi&rsquo;s ceiling started life as an
          inference. This is the measurement that replaces it, and it is re-run as models change: a
          ceiling that was right for one generation of models is not self-evidently right for the
          next.
        </p>

        <h2>What is scored, and why it is not &ldquo;quality&rdquo;</h2>
        <p>
          Batching does not primarily make wording clumsier. It makes segments{" "}
          <strong>disappear, merge, and get renumbered</strong>, and it mangles the placeholders and
          inline markup inside them. In a localization pipeline that is not a style complaint but a
          correctness failure: a translation that lost its <code>{"{0}"}</code> cannot be written
          back into the source file at all. So the metric is structural integrity — did every
          segment come back, under the id it was sent, with its placeholders and tags intact? That
          needs no reference translations, which is why it can be measured for any model in any
          language pair.
        </p>

        {current.length === 0 ? (
          <p>
            <em>No real-model runs on record yet. Run `make batch-eval-publish`.</em>
          </p>
        ) : (
          <>
            <h2>Structural integrity against batch size</h2>
            <p style={{ color: "var(--ifm-color-emphasis-700)", fontSize: "0.92rem" }}>
              Measured {latest} · target {current[0].target} · {current[0].corpus} · corpus{" "}
              <code>{canonicalDigest}</code>
              {stale > 0 && (
                <>
                  {" "}
                  · {stale} older run{stale === 1 ? "" : "s"} hidden: measured on a different
                  corpus, and therefore not comparable
                </>
              )}
            </p>
            <Curve runs={current} />
            <Legend runs={current} />

            <h2>What it costs, and what you give up</h2>
            <p>
              Batching is generally assumed to be a cost lever: pack the same content into fewer
              calls, repeat the instructions fewer times, bill fewer input tokens.{" "}
              <strong>On these models, it is not.</strong> Going from one block per call to
              thirty-two roughly halves input tokens — and roughly doubles output tokens, because a
              batched reply has to carry an id and a JSON envelope for every segment. Output is
              priced around six times input. The two effects cancel, and cost per 1,000 words comes
              out flat, or slightly worse than translating one string at a time.
            </p>
            <p>
              What batching actually buys is <strong>throughput</strong>: two to three times the
              words per second, because you are waiting on a handful of round trips instead of
              thirty. That is a real and worthwhile benefit. It is simply not the one it is usually
              sold as, and it is worth knowing which lever you are pulling.
            </p>
            <p>
              The unit is <strong>USD per 1,000 source words</strong>, because content budgets are
              denominated in words and a token count is a fact about a vendor&rsquo;s tokenizer
              rather than about the work. Nothing here is specific to translation: any AI pass over
              content — a review, a check, terminology, entity extraction — has the same shape, and
              the same economics. Translation is simply the pass this harness drives, because it is
              the one that batches.
            </p>
            <p style={{ fontSize: "0.92rem", color: "var(--ifm-color-emphasis-700)" }}>
              One block per call, against kapi&rsquo;s shipped ceiling of {SHIPPED_CEILING}:
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
                  <th style={cell}>N=1</th>
                  <th style={cell}>N={SHIPPED_CEILING}</th>
                  <th style={cell}>Δ</th>
                  <th style={cell}>N=1</th>
                  <th style={cell}>N={SHIPPED_CEILING}</th>
                  <th style={cell}>Δ</th>
                </tr>
              </thead>
              <tbody>
                {current.map((r) => {
                  const one = r.results.find((p) => p.n === 1);
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
                      <td style={cell}>{c1 != null ? usd(c1) : "—"}</td>
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
              Read the two Δ columns together: batching to {SHIPPED_CEILING} does not reduce cost,
              and roughly doubles or triples throughput, at no measured cost to structural
              integrity. That is the trade it actually offers.
            </p>
            <p style={{ fontSize: "0.85rem", color: "var(--ifm-color-emphasis-700)" }}>
              Two honest caveats about these numbers.{" "}
              <strong>The Anthropic models show no cost</strong>, because this sweep reached them
              through the claude-code subscription, which is not billed per token — and whose token
              counts do not describe an API call either: the CLI wraps every request in its own
              agent system prompt and reports it as cache creation, so it recorded 240 input tokens
              across sixty calls. A dollar figure built on that would be fiction in whichever
              direction happened to flatter the conclusion, so the column is blank. Cost for those
              models needs a sweep against the metered Anthropic API.{" "}
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
                            color: intact(p) < 100 ? "#d65a5a" : undefined,
                            fontWeight: intact(p) < 100 ? 600 : undefined,
                          }}
                        >
                          {p.failed ? "failed" : `${intact(p).toFixed(1)}%`}
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
          The harness is <code>scripts/batcheval</code>. It translates a fixed corpus — long prose,
          placeholders, inline markup, and the same source text under two different keys — at each
          N, and scores what came back. A run against the built-in demo stub exercises the harness
          and measures nothing about any model; such runs are marked <code>simulated</code> and are
          excluded from every chart on this page.
        </p>
        <pre>
          <code>{`make batch-eval                     # demo stub: proves the harness, measures nothing
make batch-eval-publish             # the real sweep → this page's data`}</code>
        </pre>
      </main>
    </Layout>
  );
}
