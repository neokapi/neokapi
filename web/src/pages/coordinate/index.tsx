import type { CSSProperties, ReactElement, ReactNode } from "react";
import Layout from "@theme/Layout";
import report from "./_coordinate.json";
import abReport from "./_abeval.json";

// The /coordinate report: what kapi reuses from a content memory when a source
// changes, and what it refuses to reuse.
//
// Every figure comes from running kapi's own code (the resolver, the matcher,
// the segmenter, the recycle tool and the prompt builder) with no model calls,
// so results are exact rather than sampled. scripts/coordinatereport's tests
// fail when the committed data stops matching the code.
//
// Regenerate with `go run ./scripts/coordinatereport`.

interface Point {
  slug: string;
  label: string;
  coordinates?: Record<string, string>;
  profile?: string;
  channel?: string;
  voiceFile?: string;
  voiceField?: string;
  termStore?: string;
  collections?: string[];
  default?: boolean;
}
interface Span {
  text: string;
  op: "same" | "cosmetic" | "added" | "removed";
}
interface Diff {
  kind: string;
  prior: Span[];
  current: Span[];
}
interface Version {
  source: string;
  target: string;
  fingerprint?: string;
  approvedAt: string;
  governed: boolean;
}
interface Chain {
  case: string;
  unit: string;
  point: string;
  inForce: string;
  currentText: string;
  versions: Version[];
  diff: Diff;
  offered?: Version;
  withheld?: string;
}
interface PromptSection {
  kind: string;
  origin: string;
  heading?: string;
  text: string;
}
interface PromptPair {
  case: string;
  source: string;
  without: PromptSection[];
  with: PromptSection[];
  digests: { without: string; with: string };
  withheld?: string;
}
interface LadderRung {
  edit: string;
  kind: string;
  text: string;
  diff: Diff;
  score: number;
  match?: string;
  classified: string;
  safeToFill: boolean;
  byScore: string;
  byKind: string;
  harm?: string;
  diverges?: boolean;
}
interface EditLadder {
  original: string;
  target: string;
  rungs: LadderRung[];
  fillFloor: number;
  wrongFills: number;
  missedFills: number;
  agreements: number;
}
interface SegmentRow {
  index: number;
  prior: string;
  current: string;
  diff: Diff;
  score: number;
  classified: string;
  filled: string;
}
interface SegmentSplit {
  engine: string;
  prior: string;
  current: string;
  approved: string;
  blockDiff: Diff;
  blockScore: number;
  blockClassified: string;
  blockFilled: string;
  blockFilledByScore: string;
  segments: SegmentRow[];
  reusable: number;
  moved: number;
}
interface Axes {
  products: string[];
  channels: string[];
  channelsByProduct: Record<string, string[]>;
}
interface Report {
  _note: string;
  recipe: string;
  points: Point[];
  axes: Axes;
  chains: Chain[];
  prompts: PromptPair[];
  ladder: EditLadder;
  segments: SegmentSplit;
}

interface AbVerdict {
  winner: "with" | "without" | "tie";
  reason?: string;
  shownFirst: string;
}
interface AbSample {
  without: string;
  with: string;
  keptWithout: boolean;
  keptWith: boolean;
  driftedWithout: boolean;
  driftedWith: boolean;
  judge?: AbVerdict;
}
interface AbCase {
  name: string;
  priorSource: string;
  source: string;
  priorTarget: string;
  keep: string[];
  drift: string[];
  why: string;
  withheld?: boolean;
  promptDiffers: boolean;
  samples: AbSample[];
  keptWith: number;
  keptWithout: number;
}
interface AbSentTurn {
  role: string;
  text: string;
}
interface AbArm {
  name: string;
  calls: number;
  inputTokens: number;
  cachedInputTokens: number;
  outputTokens: number;
  kept: number;
  drifted: number;
  translations: Record<string, string>;
  sent?: AbSentTurn[];
}
interface AbBatched {
  blocks: number;
  scored: number;
  termRulesAtCoordinate: number;
  termRulesSentBatched: number;
  arms: AbArm[];
}
interface AbReport {
  ranAt: string;
  model: string;
  judge: string;
  modelFamily: string;
  judgeFamily: string;
  repeat: number;
  samples: number;
  keptWith: number;
  keptWithout: number;
  judgeWith: number;
  judgeWithout: number;
  judgeTie: number;
  judgeValidated: boolean;
  cases: AbCase[];
  batched?: AbBatched;
}

const data = report as Report;
const ab = abReport as AbReport;

const mono = "var(--ifm-font-family-monospace)";

const styles: Record<string, CSSProperties> = {
  lede: { fontSize: "1.1rem", lineHeight: 1.6, maxWidth: "62ch" },
  prose: { maxWidth: "66ch" },
  slug: { fontFamily: mono, fontSize: ".78rem", color: "var(--ifm-color-emphasis-700)" },
  val: { fontFamily: mono, fontSize: ".85rem" },

  // Each eval is a bordered card so a reader can see where one starts and the
  // next begins. A page of stacked tables under similar headings reads as one
  // long table.
  eval: {
    border: "1px solid var(--ifm-color-emphasis-300)",
    borderRadius: 10,
    padding: "1.5rem 1.6rem 1.8rem",
    marginTop: "2.5rem",
    background: "var(--ifm-background-surface-color)",
  },
  evalNumber: {
    fontFamily: mono,
    fontSize: ".75rem",
    letterSpacing: ".1em",
    textTransform: "uppercase",
    color: "var(--ifm-color-emphasis-600)",
  },
  evalBadge: {
    marginLeft: ".7rem",
    padding: ".05rem .4rem",
    borderRadius: 4,
    background: "var(--ifm-color-warning-contrast-background)",
    color: "var(--ifm-color-warning-darker)",
    letterSpacing: ".05em",
  },
  evalTitle: { margin: ".2rem 0 .1rem", fontSize: "1.45rem" },
  evalQuestion: {
    fontSize: ".95rem",
    color: "var(--ifm-color-emphasis-800)",
    marginBottom: "1.2rem",
    maxWidth: "66ch",
  },
  result: {
    borderLeft: "3px solid var(--ifm-color-primary)",
    paddingLeft: ".9rem",
    margin: "1.4rem 0 0",
    maxWidth: "66ch",
  },
  resultLabel: {
    fontSize: ".75rem",
    letterSpacing: ".1em",
    textTransform: "uppercase",
    color: "var(--ifm-color-emphasis-700)",
    display: "block",
    marginBottom: ".3rem",
  },

  facts: {
    border: "1px solid var(--ifm-color-emphasis-300)",
    borderRadius: 8,
    padding: ".2rem 1.15rem",
    background: "var(--ifm-background-surface-color)",
    maxWidth: "62rem",
  },
  factRow: {
    display: "grid",
    gridTemplateColumns: "12rem 1fr",
    gap: "1rem",
    padding: ".7rem 0",
    borderTop: "1px solid var(--ifm-color-emphasis-200)",
    fontSize: ".9rem",
    lineHeight: 1.5,
  },
  factRowFirst: {
    display: "grid",
    gridTemplateColumns: "12rem 1fr",
    gap: "1rem",
    padding: ".7rem 0",
    fontSize: ".9rem",
    lineHeight: 1.5,
  },
  factKey: { fontWeight: 600 },

  sideBySide: { display: "grid", gridTemplateColumns: "1fr 1fr", gap: "1rem" },
  pane: {
    border: "1px solid var(--ifm-color-emphasis-300)",
    borderRadius: 8,
    padding: ".85rem 1rem",
    background: "var(--ifm-background-color)",
    minWidth: 0,
  },
  paneHead: {
    fontSize: ".75rem",
    textTransform: "uppercase",
    letterSpacing: ".08em",
    color: "var(--ifm-color-emphasis-700)",
    marginBottom: ".6rem",
  },
  sect: { marginBottom: ".7rem", fontSize: ".82rem", lineHeight: 1.45 },
  sectOrigin: { fontFamily: mono, fontSize: ".7rem", color: "var(--ifm-color-emphasis-600)" },
  sectText: { whiteSpace: "pre-wrap", wordBreak: "break-word" },
  gained: {
    borderLeft: "3px solid var(--ifm-color-success)",
    paddingLeft: ".6rem",
    background: "var(--ifm-color-success-contrast-background)",
  },
  pill: {
    display: "inline-block",
    fontFamily: mono,
    fontSize: ".7rem",
    padding: ".1rem .45rem",
    borderRadius: 4,
    whiteSpace: "nowrap",
  },
  table: { width: "100%", fontSize: ".88rem", display: "table" },
  scroll: { overflowX: "auto" },
  subhead: {
    display: "block",
    fontWeight: 400,
    fontSize: ".76rem",
    color: "var(--ifm-color-emphasis-700)",
    marginTop: ".15rem",
  },
  sentBody: {
    fontSize: ".72rem",
    lineHeight: 1.5,
    maxHeight: "22rem",
    overflow: "auto",
    whiteSpace: "pre-wrap",
    wordBreak: "break-word",
    background: "var(--ifm-background-color)",
    border: "1px solid var(--ifm-color-emphasis-300)",
    borderLeftWidth: 1,
    borderRadius: 4,
    padding: ".7rem .85rem",
  },
  diffText: { fontSize: ".92rem", lineHeight: 1.6 },
  outcome: { fontSize: ".85rem", lineHeight: 1.45 },
  nothing: { color: "var(--ifm-color-emphasis-600)", fontStyle: "italic" },
  kept: {
    background: "var(--ifm-color-success-contrast-background)",
    color: "var(--ifm-color-success-contrast-foreground)",
    borderRadius: 3,
    padding: "0 .2rem",
  },
  drifted: {
    background: "var(--ifm-color-danger-contrast-background)",
    color: "var(--ifm-color-danger-contrast-foreground)",
    borderRadius: 3,
    padding: "0 .2rem",
  },
  legend: {
    display: "flex",
    flexWrap: "wrap",
    gap: "1.1rem",
    fontSize: ".8rem",
    color: "var(--ifm-color-emphasis-700)",
    margin: "1rem 0",
  },
  mapCell: {
    border: "1px solid var(--ifm-color-emphasis-300)",
    borderRadius: 6,
    padding: ".6rem .7rem",
    background: "var(--ifm-background-color)",
    fontSize: ".82rem",
    lineHeight: 1.45,
    minHeight: "5.4rem",
  },
  mapEmpty: {
    border: "1px dashed var(--ifm-color-emphasis-300)",
    borderRadius: 6,
    padding: ".6rem .7rem",
    fontSize: ".8rem",
    color: "var(--ifm-color-emphasis-600)",
    minHeight: "5.4rem",
    display: "flex",
    alignItems: "center",
  },
  mapAbsent: {
    border: "1px solid transparent",
    borderRadius: 6,
    minHeight: "5.4rem",
    background:
      "repeating-linear-gradient(45deg, transparent, transparent 5px, var(--ifm-color-emphasis-200) 5px, var(--ifm-color-emphasis-200) 6px)",
  },
  mapSwatch: {
    display: "inline-block",
    width: "1.6rem",
    height: ".9rem",
    minHeight: 0,
    verticalAlign: "-.1rem",
    marginRight: ".35rem",
    padding: 0,
  },
  mapAxis: {
    fontSize: ".8rem",
    fontWeight: 600,
    color: "var(--ifm-color-emphasis-800)",
    alignSelf: "center",
  },
};

const diffSpanStyle: Record<Span["op"], CSSProperties> = {
  same: {},
  cosmetic: {
    background: "var(--ifm-color-emphasis-200)",
    borderRadius: 3,
    boxShadow: "inset 0 -1px 0 var(--ifm-color-emphasis-500)",
  },
  added: {
    background: "var(--ifm-color-success-contrast-background)",
    color: "var(--ifm-color-success-contrast-foreground)",
    fontWeight: 600,
    borderRadius: 3,
    padding: "0 .1rem",
  },
  removed: {
    background: "var(--ifm-color-danger-contrast-background)",
    color: "var(--ifm-color-danger-contrast-foreground)",
    textDecoration: "line-through",
    borderRadius: 3,
    padding: "0 .1rem",
  },
};

function pill(text: string, tone: "ok" | "no" | "flat"): ReactElement {
  const bg =
    tone === "ok"
      ? "var(--ifm-color-success-contrast-background)"
      : tone === "no"
        ? "var(--ifm-color-warning-contrast-background)"
        : "var(--ifm-color-emphasis-200)";
  const fg =
    tone === "ok"
      ? "var(--ifm-color-success-darker)"
      : tone === "no"
        ? "var(--ifm-color-warning-darker)"
        : "var(--ifm-color-emphasis-800)";
  return <span style={{ ...styles.pill, background: bg, color: fg }}>{text}</span>;
}

function DiffText({ spans }: { spans: Span[] }): ReactElement {
  return (
    <span style={styles.diffText}>
      {spans.map((s, i) => (
        <span key={i} style={diffSpanStyle[s.op]}>
          {s.text}
        </span>
      ))}
    </span>
  );
}

function hasWordChange(spans: Span[]): boolean {
  return spans.some((s) => s.op === "added" || s.op === "removed");
}

/**
 * The old text above the new one, but only when a word actually vanished.
 * Otherwise the two lines would be the same sentence printed twice.
 */
function EditView({ diff }: { diff: Diff }): ReactElement {
  return (
    <>
      {hasWordChange(diff.prior) && (
        <div style={{ marginBottom: ".2rem", opacity: 0.75 }}>
          <DiffText spans={diff.prior} />
        </div>
      )}
      <div>
        <DiffText spans={diff.current} />
      </div>
    </>
  );
}

/** What the recycle tool wrote into the target file, or nothing. */
function Outcome({ text }: { text: string }): ReactElement {
  if (!text) {
    return (
      <span style={{ ...styles.outcome, ...styles.nothing }}>nothing (goes to a translator)</span>
    );
  }
  return <span style={styles.outcome}>{text}</span>;
}

function DiffLegend(): ReactElement {
  return (
    <div style={styles.legend}>
      <span>
        <span style={diffSpanStyle.added}>word added</span>
      </span>
      <span>
        <span style={diffSpanStyle.removed}>word removed</span>
      </span>
      <span>
        <span style={diffSpanStyle.cosmetic}>punctuation, capitals, quote marks</span> (not
        compared)
      </span>
    </div>
  );
}

/** Column header with a smaller line of explanation under it. */
function Th({ label, sub, width }: { label: string; sub?: string; width?: string }): ReactElement {
  return (
    <th style={width ? { minWidth: width } : undefined}>
      {label}
      {sub && <span style={styles.subhead}>{sub}</span>}
    </th>
  );
}

const evalCount = 5;

/** One eval, boxed and numbered so it has a visible start and end. */
function Eval({
  n,
  title,
  question,
  children,
  result,
  badge,
}: {
  n: number;
  title: string;
  question: string;
  children: ReactNode;
  result: ReactNode;
  badge?: string;
}): ReactElement {
  return (
    <section style={styles.eval}>
      <div style={styles.evalNumber}>
        Eval {n} of {evalCount}
        {badge && <span style={styles.evalBadge}>{badge}</span>}
      </div>
      <h2 style={styles.evalTitle}>{title}</h2>
      <p style={styles.evalQuestion}>{question}</p>
      {children}
      <div style={styles.result}>
        <span style={styles.resultLabel}>Result</span>
        {result}
      </div>
    </section>
  );
}

/**
 * One A/B case: the wording under test, then every sample's two translations
 * side by side. The Norwegian is the evidence, so it is shown in full rather
 * than summarised into a rate.
 */
function AbCaseView({ c }: { c: AbCase }): ReactElement {
  const n = c.samples.length;
  return (
    <div style={{ ...styles.pane, marginTop: "1.2rem" }}>
      <div
        style={{ display: "flex", justifyContent: "space-between", gap: "1rem", flexWrap: "wrap" }}
      >
        <strong>{c.name}</strong>
        <span>
          {c.withheld
            ? pill("control: no reference given", "flat")
            : pill(`kept ${c.keptWith}/${n} with, ${c.keptWithout}/${n} without`, "ok")}
        </span>
      </div>
      <p style={{ ...styles.slug, margin: ".4rem 0 .8rem" }}>{c.why}</p>
      <div style={{ ...styles.factRowFirst, padding: 0, marginBottom: ".3rem" }}>
        <span style={styles.factKey}>English now</span>
        <span>{c.source}</span>
      </div>
      <div style={{ ...styles.factRowFirst, padding: 0, marginBottom: ".8rem" }}>
        <span style={styles.factKey}>Approved before</span>
        <span>
          {c.priorTarget}{" "}
          <span style={styles.slug}>
            (for &ldquo;{c.priorSource}&rdquo;
            {c.withheld && ", withheld from the model in this case"})
          </span>
        </span>
      </div>
      <div style={styles.scroll}>
        <table style={styles.table}>
          <thead>
            <tr>
              <Th label="#" />
              <Th label="Without the reference" width="18rem" />
              <Th label="With the reference" width="18rem" />
            </tr>
          </thead>
          <tbody>
            {c.samples.map((s, i) => (
              <tr key={i}>
                <td style={styles.val}>{i + 1}</td>
                <td>
                  <span style={s.keptWithout ? styles.kept : styles.drifted}>{s.without}</span>
                </td>
                <td>
                  <span style={s.keptWith ? styles.kept : styles.drifted}>{s.with}</span>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <div style={{ ...styles.legend, marginBottom: 0 }}>
        <span>
          <span style={styles.kept}>green</span> keeps the approved wording (
          {c.keep.slice(0, 2).join(", ")})
        </span>
        <span>
          <span style={styles.drifted}>red</span> uses another word (
          {c.drift.slice(0, 2).join(", ")})
        </span>
      </div>
    </div>
  );
}

/**
 * One arm's prompt, exactly as the model received it.
 *
 * Captured during the run rather than re-rendered here: a page that renders its
 * own idea of the prompt is the failure this whole report exists to avoid.
 */
function SentPrompt({ turns }: { turns: AbSentTurn[] }): ReactElement {
  return (
    <>
      {turns.map((turn, i) => (
        <div key={i} style={{ marginBottom: ".8rem" }}>
          <div style={styles.paneHead}>{turn.role}</div>
          <pre style={{ ...styles.sentBody, margin: 0 }}>
            <code>{turn.text}</code>
          </pre>
        </div>
      ))}
    </>
  );
}

function addedOrigins(a: PromptSection[], b: PromptSection[]): Set<string> {
  const seen = new Set(a.map((s) => s.origin));
  return new Set(b.filter((s) => !seen.has(s.origin)).map((s) => s.origin));
}

function SectionList({
  sections,
  highlight,
}: {
  sections: PromptSection[];
  highlight: Set<string>;
}): ReactElement {
  return (
    <>
      {sections.map((s, i) => (
        <div
          key={`${s.origin}-${i}`}
          style={highlight.has(s.origin) ? { ...styles.sect, ...styles.gained } : styles.sect}
        >
          <div style={styles.sectOrigin}>{s.origin}</div>
          {s.heading && <div style={{ fontWeight: 600 }}>{s.heading}</div>}
          <div style={styles.sectText}>{s.text}</div>
        </div>
      ))}
    </>
  );
}

/**
 * A path short enough to read and long enough to tell apart. Every profile's
 * voice file is named voice.yaml, so a basename alone would show two different
 * files under one name.
 */
const shortPath = (p?: string): string => (p ? p.replace(/^\.kapi\//, "") : "");

/**
 * Products down the side, channels across the top, and what governs the content
 * at each crossing. Three states: content here, a channel the product declares
 * with nothing in it yet, and a crossing the recipe never declared. A list of
 * cards can only show the first.
 */
function CoordinateMap({ points, axes }: { points: Point[]; axes: Axes }): ReactElement {
  const placed = points.filter((p) => p.coordinates?.product && p.coordinates?.channel);
  const at = (product: string, channel: string): Point | undefined =>
    placed.find((p) => p.coordinates!.product === product && p.coordinates!.channel === channel);
  const declares = (product: string, channel: string): boolean =>
    (axes.channelsByProduct[product] ?? []).includes(channel);

  return (
    <div style={styles.scroll}>
      <div
        style={{
          display: "grid",
          gridTemplateColumns: `6rem repeat(${axes.channels.length}, minmax(12rem, 1fr))`,
          gap: ".5rem",
          minWidth: `${6 + axes.channels.length * 12}rem`,
        }}
      >
        <div style={{ ...styles.slug, alignSelf: "center" }}>product / channel</div>
        {axes.channels.map((c) => (
          <div key={c} style={{ ...styles.mapAxis, textAlign: "center" }}>
            {c}
          </div>
        ))}
        {axes.products.map((prod) => (
          <div key={prod} style={{ display: "contents" }}>
            <div style={styles.mapAxis}>{prod}</div>
            {axes.channels.map((chan) => {
              if (!declares(prod, chan)) {
                return <div key={`${prod}-${chan}`} style={styles.mapAbsent} />;
              }
              const p = at(prod, chan);
              if (!p) {
                return (
                  <div key={`${prod}-${chan}`} style={styles.mapEmpty}>
                    declared, no content yet
                  </div>
                );
              }
              return (
                <div key={`${prod}-${chan}`} style={styles.mapCell}>
                  <div style={{ fontWeight: 600, marginBottom: ".35rem" }}>
                    {(p.collections ?? []).join(", ")}
                  </div>
                  <div style={styles.slug}>voice: {shortPath(p.voiceFile)}</div>
                  <div style={styles.slug}>
                    terms: {p.termStore ? shortPath(p.termStore) : "project default"}
                  </div>
                </div>
              );
            })}
          </div>
        ))}
      </div>
      <div style={{ ...styles.legend, marginTop: ".8rem" }}>
        <span>
          <span style={{ ...styles.mapCell, ...styles.mapSwatch }} /> content, governed here
        </span>
        <span>
          <span style={{ ...styles.mapEmpty, ...styles.mapSwatch }} /> channel declared, no content
        </span>
        <span>
          <span style={{ ...styles.mapAbsent, ...styles.mapSwatch }} /> not a channel of this
          product
        </span>
      </div>
    </div>
  );
}

export default function Coordinate(): ReactElement {
  const offeredCount = data.chains.filter((c) => c.offered).length;
  const withheldCount = data.chains.length - offeredCount;
  const ladder = data.ladder;
  const seg = data.segments;
  const fallback = data.points.find((p) => p.default);
  const wrongScores = ladder.rungs
    .filter((r) => r.diverges && !r.safeToFill)
    .map((r) => `${r.score}%`)
    .join(" and ");

  return (
    <Layout
      title="Reuse report"
      description="What kapi reuses from a content memory when a source changes, and what it refuses to reuse."
    >
      <main className="container margin-vert--lg">
        <h1>Translation reuse report</h1>
        <p style={styles.lede}>
          When an author edits an English sentence, kapi decides whether the approved Norwegian
          still applies. Four evals run that decision on real cases and show what the tool wrote
          each time.
        </p>

        <h2>How this is measured</h2>
        <div style={styles.facts}>
          <div style={styles.factRowFirst}>
            <span style={styles.factKey}>AI models used</span>
            <span>
              Evals 1 to 4 use none. Every figure in them comes from running kapi&rsquo;s own
              resolver, matcher, segmenter, recycle tool and prompt builder, so the results are
              exact rather than sampled. Eval 5 calls <code>{ab.model}</code> and is the only one
              that does; its data is committed rather than regenerated, so the rest of the page
              stays deterministic.
            </span>
          </div>
          <div style={styles.factRow}>
            <span style={styles.factKey}>Pass bar</span>
            <span>
              Tests in <code>scripts/coordinatereport</code> fail the build if the data in evals 1
              to 4 stops matching the code, if the two reuse rules stop disagreeing, or if a case is
              added whose hand-written label disagrees with kapi&rsquo;s own verdict. Eval 5 has no
              build gate: it samples a model, so a threshold on it would fail the build on a bad
              day. Its consistency half is a deterministic string check and its judged half is
              reported as unvalidated.
            </span>
          </div>
          <div style={styles.factRow}>
            <span style={styles.factKey}>Not measured here</span>
            <span>
              Whether the judge in eval 5 agrees with a person. Until that agreement is measured on
              labelled examples, the judged win-loss numbers are reported and not relied on. The
              consistency numbers beside them need no such caveat.
            </span>
          </div>
        </div>

        <h2 style={{ marginTop: "2.5rem" }}>Terms used below</h2>
        <div style={styles.facts}>
          <div style={styles.factRowFirst}>
            <span style={styles.factKey}>Match score</span>
            <span>
              How similar the new English is to the English that was approved, as a percentage.
              kapi&rsquo;s matcher compares the two texts character by character, so the score falls
              as more characters differ. The same one-character edit therefore scores higher in a
              long sentence than in a short one.
            </span>
          </div>
          <div style={styles.factRow}>
            <span style={styles.factKey}>Verdict</span>
            <span>
              kapi&rsquo;s classification of the edit, from comparing the two texts word by word
              rather than character by character. <code>none</code>: identical.{" "}
              <code>cosmetic</code>: the same words in the same order, differing only in
              punctuation, capitals, spacing or quote shape. <code>substantive</code>: a word was
              added, removed or changed. Sentence length does not affect it.
            </span>
          </div>
          <div style={styles.factRow}>
            <span style={styles.factKey}>Reuse</span>
            <span>
              Writing the approved translation into the target file as a draft for review. The
              alternative is leaving the target empty, so the sentence goes to a translator or a
              model instead.
            </span>
          </div>
          <div style={styles.factRow}>
            <span style={styles.factKey}>Old and current rule</span>
            <span>
              The old rule reuses whenever the match score is {ladder.fillFloor}% or better. The
              current rule reuses whenever the verdict is <code>none</code> or <code>cosmetic</code>
              , and ignores the score.
            </span>
          </div>
        </div>

        <Eval
          n={1}
          title="Edited sentences"
          question="An author edits one approved English sentence eleven ways. Does each rule reuse the approved Norwegian when it should, and leave it alone when it should not?"
          result={
            <>
              <p style={styles.prose}>
                The two rules agree on {ladder.agreements} of the {ladder.rungs.length} edits and
                disagree on {ladder.wrongFills}, highlighted above. In both, the meaning of the
                English changed and the old rule still wrote the old Norwegian into the file.
              </p>
              <p style={styles.prose}>
                Raising the threshold would not fix it. Those two edits score {wrongScores}, higher
                than every edit the old rule correctly declined. A full stop added to a short button
                label scores 91%. Any threshold that catches the meaning changes also rejects
                harmless punctuation edits, and any threshold that keeps the punctuation edits lets
                the meaning changes through. The match score puts them in the wrong order, so no
                threshold sorts them correctly.
              </p>
              <p style={styles.prose}>
                The current rule compares words instead. Adding &ldquo;not&rdquo; is a word change
                in a two-word label and in a paragraph alike, so sentence length stops affecting the
                decision.
              </p>
            </>
          }
        >
          <div style={{ ...styles.facts, marginBottom: "1rem" }}>
            <div style={styles.factRowFirst}>
              <span style={styles.factKey}>Approved English</span>
              <span>{ladder.original}</span>
            </div>
            <div style={styles.factRow}>
              <span style={styles.factKey}>Approved Norwegian</span>
              <span>{ladder.target}</span>
            </div>
          </div>
          <DiffLegend />
          <div style={styles.scroll}>
            <table style={styles.table}>
              <thead>
                <tr>
                  <Th label="The author's edit" width="22rem" />
                  <Th label="Match" sub="score %" />
                  <Th label="Verdict" sub="word comparison" />
                  <Th label="Old rule wrote" sub={`reuse at ${ladder.fillFloor}%+`} width="14rem" />
                  <Th label="Current rule wrote" sub="reuse if words unchanged" width="14rem" />
                </tr>
              </thead>
              <tbody>
                {ladder.rungs.map((r) => (
                  <tr
                    key={r.edit}
                    style={
                      r.diverges
                        ? { background: "var(--ifm-color-warning-contrast-background)" }
                        : {}
                    }
                  >
                    <td>
                      <div style={{ ...styles.slug, marginBottom: ".25rem" }}>{r.edit}</div>
                      <EditView diff={r.diff} />
                      {r.harm && (
                        <div
                          style={{
                            ...styles.slug,
                            marginTop: ".4rem",
                            color: "var(--ifm-color-danger-darker)",
                          }}
                        >
                          Reusing here is wrong: {r.harm}
                        </div>
                      )}
                    </td>
                    <td style={styles.val}>{r.score}</td>
                    <td>{r.safeToFill ? pill(r.classified, "ok") : pill(r.classified, "no")}</td>
                    <td>
                      <Outcome text={r.byScore} />
                    </td>
                    <td>
                      <Outcome text={r.byKind} />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </Eval>

        <Eval
          n={2}
          title="A paragraph against its sentences"
          question="A paragraph is stored as one block. When one sentence in it changes, what does each rule do, and what changes if the paragraph is split into sentences first?"
          result={
            <p style={styles.prose}>
              Scored as one block the paragraph is {seg.blockScore}%, so the old rule reused it and
              wrote the outdated billing terms back into the file. Split into sentences,{" "}
              {seg.reusable} of {seg.segments.length} keep their approved Norwegian and {seg.moved}{" "}
              goes to a translator. Both measurements are useful: the block score answers whether
              the paragraph changed, the sentence split answers which part changed and what still
              stands. The changed sentence is also a better unit to send to a model, because it
              arrives with its two approved neighbours as context.
            </p>
          }
        >
          <div style={styles.sideBySide}>
            <div style={styles.pane}>
              <div style={styles.paneHead}>scored as one block: {seg.blockScore}%</div>
              <EditView diff={seg.blockDiff} />
              <div style={{ ...styles.factRowFirst, marginTop: ".9rem", padding: 0 }}>
                <span style={styles.factKey}>Old rule wrote</span>
                <span style={styles.outcome}>{seg.blockFilledByScore}</span>
              </div>
              <div style={{ ...styles.factRow, paddingBottom: 0 }}>
                <span style={styles.factKey}>Current rule wrote</span>
                <span>
                  <Outcome text={seg.blockFilled} />
                </span>
              </div>
            </div>
            <div style={styles.pane}>
              <div style={styles.paneHead}>the approved Norwegian</div>
              <div style={{ color: "var(--ifm-color-emphasis-700)", fontSize: ".92rem" }}>
                {seg.approved}
              </div>
              <p style={{ ...styles.slug, marginTop: ".9rem", marginBottom: 0 }}>
                What the old rule wrote is this paragraph unchanged. Its middle sentence still says{" "}
                <em>dagen f&oslash;r</em> (the day before) while the English now says three days.
              </p>
            </div>
          </div>
          <p style={{ ...styles.prose, marginTop: "1.5rem", marginBottom: ".6rem" }}>
            The same paragraph split by the {seg.engine} sentence segmenter, each sentence scored
            and decided on its own:
          </p>
          <div style={styles.scroll}>
            <table style={styles.table}>
              <thead>
                <tr>
                  <Th label="#" />
                  <Th label="Sentence" width="24rem" />
                  <Th label="Match" sub="score %" />
                  <Th label="Verdict" sub="word comparison" />
                  <Th label="Current rule wrote" width="16rem" />
                </tr>
              </thead>
              <tbody>
                {seg.segments.map((s) => (
                  <tr key={s.index}>
                    <td style={styles.val}>{s.index}</td>
                    <td>
                      <EditView diff={s.diff} />
                    </td>
                    <td style={styles.val}>{s.score}</td>
                    <td>
                      {s.classified === "substantive"
                        ? pill(s.classified, "no")
                        : pill(s.classified, "ok")}
                    </td>
                    <td>
                      <Outcome text={s.filled} />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </Eval>

        <Eval
          n={3}
          title="Offering an old translation as reference"
          question="A sentence was translated before and the English has since changed. Can that old translation be shown to the model as reference, given the voice profile and term rules may also have changed?"
          result={
            <>
              <p style={styles.prose}>
                {offeredCount} offered, {withheldCount} withheld. The decision comes from the rules,
                not the wording: rows 1 and 5 have the same wording change and opposite outcomes,
                because row 5&rsquo;s translation was approved for a different product and channel.
              </p>
              <p style={styles.prose}>
                Two rows are worth a second look. A voice profile whose version number was bumped
                without any change to its guidance is still offered, because the fingerprint covers
                the text that reached the model rather than a version number. And a term rule added
                about words this sentence does not contain withholds it anyway, because the
                fingerprint covers every rule at the coordinate. That over-invalidates deliberately:
                a staleness check should re-verify rather than miss.
              </p>
            </>
          }
        >
          <p style={{ ...styles.prose, marginBottom: ".8rem" }}>
            kapi stores a fingerprint of the voice profile and term rules that were in force when a
            translation was approved, and compares it with the fingerprint in force now.
          </p>
          <div style={styles.scroll}>
            <table style={styles.table}>
              <thead>
                <tr>
                  <Th label="Case" />
                  <Th label="How the English changed" width="18rem" />
                  <Th label="Rules then" sub="fingerprint" />
                  <Th label="Rules now" sub="fingerprint" />
                  <Th label="Decision" />
                </tr>
              </thead>
              <tbody>
                {data.chains.map((c) => (
                  <tr key={c.case}>
                    <td>
                      {c.case}
                      <div style={{ ...styles.slug, marginTop: ".25rem" }}>
                        approved: &ldquo;{c.versions[0]?.target}&rdquo;
                      </div>
                    </td>
                    <td>
                      <EditView diff={c.diff} />
                    </td>
                    <td style={styles.val}>{c.versions[0]?.fingerprint || "none"}</td>
                    <td style={styles.val}>{c.inForce}</td>
                    <td>
                      {c.offered ? (
                        pill("offered", "ok")
                      ) : (
                        <>
                          {pill("withheld", "no")}
                          <div style={{ ...styles.slug, marginTop: ".2rem" }}>{c.withheld}</div>
                        </>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </Eval>

        <Eval
          n={4}
          title="What reaches the model"
          question="When an old translation is offered, does it actually appear in the prompt, and does the prompt cache key change so a cached answer is not served after the reference changes?"
          result={
            <p style={styles.prose}>
              In each of the {offeredCount} offered cases the prompt gains a section and the cache
              key changes. In each of the {withheldCount} withheld cases the two prompts are
              identical and the cache key is unchanged. A test asserts both directions, because a
              reference that reached the model without moving the cache key would be correct once
              and stale afterwards.
            </p>
          }
        >
          {data.prompts.map((p) => {
            const gained = addedOrigins(p.without, p.with);
            return (
              <div key={p.case} style={{ marginTop: "1.5rem" }}>
                <h3 style={{ marginBottom: ".2rem", fontSize: "1.05rem" }}>{p.case}</h3>
                <div style={{ ...styles.slug, marginBottom: ".8rem" }}>
                  translating “{p.source}”{p.withheld && `, reference withheld: ${p.withheld}`}
                </div>
                <div style={styles.sideBySide}>
                  <div style={styles.pane}>
                    <div style={styles.paneHead}>
                      without the old translation · cache key {p.digests.without || "none"}
                    </div>
                    <SectionList sections={p.without} highlight={new Set()} />
                  </div>
                  <div style={styles.pane}>
                    <div style={styles.paneHead}>
                      with it · cache key {p.digests.with || "none"}
                    </div>
                    <SectionList sections={p.with} highlight={gained} />
                  </div>
                </div>
              </div>
            );
          })}
        </Eval>

        <Eval
          n={5}
          title="Does the old translation change what the model writes"
          question="Everything above is deterministic. This one calls a model. Each edited sentence is translated twice, once with the previously approved translation in the prompt and once without, and the outputs are compared."
          badge="spends model calls"
          result={
            <>
              <p style={styles.prose}>
                The approved wording survived {ab.keptWith} of {ab.samples} times with the reference
                and {ab.keptWithout} of {ab.samples} without it. The effect is concentrated: in
                three cases the model picks the house wording either way, and in two it never does
                without being shown it. Those two go from 0 of 3 to 3 of 3.
              </p>
              <p style={styles.prose}>
                The control is what makes that readable. It is the same sentence as the first case,
                run with the reference withheld, and it produces the drifted wording in both arms,
                every time. So the difference above comes from the reference and not from the case.
              </p>
              <p style={styles.prose}>
                The blind judge said {ab.judgeWith} for the reference arm, {ab.judgeWithout} against
                and {ab.judgeTie} ties, which is not evidence of anything yet: agreement with a
                human has not been measured, and the judge is a small local model. It is also asked
                the wrong question for this purpose. It sees the English and the two candidates, not
                the approved wording, so on the subscription case it preferred <em>plan</em> over{" "}
                <em>abonnement</em> — a defensible translation of the English and the wrong word for
                this product. Consistency is not visible to a judge that does not know what was
                agreed, which is why the measurement above it is a string check rather than an
                opinion.
              </p>
            </>
          }
        >
          <div style={styles.facts}>
            <div style={styles.factRowFirst}>
              <span style={styles.factKey}>Model under test</span>
              <span>
                <code>{ab.model}</code> ({ab.modelFamily}), {ab.repeat} samples per case per arm
              </span>
            </div>
            <div style={styles.factRow}>
              <span style={styles.factKey}>Judge</span>
              <span>
                <code>{ab.judge}</code> ({ab.judgeFamily}). Refused at startup if it shares a family
                with the model under test, because a model grading its own family prefers itself
                measurably. Blind: it never sees which candidate had the reference, nor the
                reference, and the display order alternates.
              </span>
            </div>
            <div style={styles.factRow}>
              <span style={styles.factKey}>Consistency check</span>
              <span>
                A whole-word search for the approved wording and for the word a model reaches for
                instead. Deterministic, so this half cannot be wrong about itself. It matches whole
                words rather than substrings, because Norwegian <em>handlekurven</em> contains{" "}
                <em>kurven</em>, and the first version of this check scored every drift as a match.
              </span>
            </div>
            <div style={styles.factRow}>
              <span style={styles.factKey}>Run</span>
              <span>
                {ab.ranAt}. Committed rather than regenerated per build, so the rest of this page
                stays deterministic.
              </span>
            </div>
          </div>
          {ab.batched && (
            <div style={{ marginTop: "1.4rem" }}>
              <h3 style={{ marginTop: 0 }}>The path production runs</h3>
              <p style={styles.prose}>
                Everything above translates one sentence per call. Production batches, and the two
                build materially different prompts: a batch sends a JSON payload of many segments
                and a schema-constrained reply. So the same question is asked again over a document
                of {ab.batched.blocks} blocks, {ab.batched.scored} of which carry a wording under
                test, in three arms.
              </p>
              <div style={styles.scroll}>
                <table style={styles.table}>
                  <thead>
                    <tr>
                      <Th label="Arm" width="20rem" />
                      <Th label="Kept" sub="approved wording" />
                      <Th label="Calls" />
                      <Th label="Input" sub="tokens" />
                      <Th label="Output" sub="tokens" />
                    </tr>
                  </thead>
                  <tbody>
                    {ab.batched.arms.map((a) => {
                      const total = a.kept + a.drifted;
                      return (
                        <tr key={a.name}>
                          <td>{a.name}</td>
                          <td>
                            {a.kept === total
                              ? pill(`${a.kept}/${total}`, "ok")
                              : pill(`${a.kept}/${total}`, "no")}
                          </td>
                          <td style={styles.val}>{a.calls}</td>
                          <td style={styles.val}>{a.inputTokens.toLocaleString()}</td>
                          <td style={styles.val}>{a.outputTokens.toLocaleString()}</td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
              <p style={{ ...styles.slug, marginTop: ".6rem" }}>
                Input counts include the part a provider served from its prompt cache: cheaper per
                token, still prompt the model reads.
              </p>
              <h3>What each call actually sent</h3>
              <p style={styles.prose}>
                The first call of each arm, captured as it went to the model. The difference is one{" "}
                <code>prior</code> object per segment that has history: what that segment said
                before, and the translation approved for it then. Everything else is identical,
                including the terminology &mdash; {ab.batched.termRulesSentBatched} rules of the{" "}
                {ab.batched.termRulesAtCoordinate} governing this collection, because those are the
                ones this text could use.
              </p>
              <div style={styles.sideBySide}>
                {ab.batched.arms
                  .filter((a) => a.sent)
                  .map((a) => (
                    <div key={a.name} style={styles.pane}>
                      <div style={{ ...styles.paneHead, marginBottom: ".8rem" }}>
                        {a.name} &middot; {a.kept}/{a.kept + a.drifted} kept
                      </div>
                      <SentPrompt turns={a.sent ?? []} />
                    </div>
                  ))}
              </div>

              <p style={{ ...styles.prose, marginTop: "1rem" }}>
                Two things follow. Batching does not cost the effect &mdash; the batched arm with
                references matches the one-call-per-block ceiling exactly, so the worry that a model
                juggling many references would apply each less reliably did not materialise here.
                And the references are nearly free:{" "}
                {(ab.batched.arms[1].inputTokens - ab.batched.arms[0].inputTokens).toLocaleString()}{" "}
                more input tokens across the document, for {ab.batched.arms[0].kept} of{" "}
                {ab.batched.arms[0].kept + ab.batched.arms[0].drifted} becoming{" "}
                {ab.batched.arms[1].kept} of {ab.batched.arms[1].kept + ab.batched.arms[1].drifted}.
              </p>
              <p style={styles.prose}>
                The third arm is what the old rule would have cost. A block with history used to be
                translated alone, because the batch payload had nowhere to put a per-segment
                reference &mdash; {ab.batched.arms[2].calls} calls and{" "}
                {ab.batched.arms[2].inputTokens.toLocaleString()} input tokens to reach the same
                answer the batched arm reached in {ab.batched.arms[1].calls}.
              </p>
              <p style={styles.prose}>
                One caveat on the first row: the arm with no references is the one that varies
                between runs, scoring 2 of 5 and 3 of 5 on two runs of this document. The arm with
                references scored 5 of 5 both times. That is the shape you would expect if the
                reference is doing the work — but it is two runs, not a distribution.
              </p>
              <p style={styles.prose}>
                Terminology is scoped to each call the same way: this collection is governed by{" "}
                {ab.batched.termRulesAtCoordinate} term rules and a call carries the{" "}
                {ab.batched.termRulesSentBatched} its own text could use. Tokens are the smaller
                saving; the larger is that the rules which do bite are not buried among the ones
                that cannot.
              </p>
            </div>
          )}
          {ab.cases.map((c) => (
            <AbCaseView key={c.name} c={c} />
          ))}
        </Eval>

        <h2 style={{ marginTop: "3.5rem" }}>Which rules apply where</h2>
        <p style={styles.prose}>
          Content sits at a coordinate: a product and a channel. The voice profile and the terms
          that govern it are attached to that coordinate rather than to the file, and eval 3
          compares fingerprints of exactly those. The test project declares content at three of the
          four crossings.
        </p>
        <CoordinateMap points={data.points} axes={data.axes} />
        {fallback && (
          <p style={{ marginTop: "1rem", ...styles.prose }}>
            Content that declares no coordinate falls back to the project defaults:{" "}
            <code>{shortPath(fallback.voiceFile)}</code> and the project&rsquo;s own terms. Here
            that is the <code>{(fallback.collections ?? []).join(", ")}</code> collection.
          </p>
        )}

        <h2 style={{ marginTop: "3.5rem" }}>The test project</h2>
        <p style={styles.prose}>
          The recipe every figure above is computed against. It is deliberately awkward: two
          products share a channel name, only one declares a terms store, and one collection
          declares no coordinate at all.
        </p>
        <pre style={{ fontSize: ".8rem" }}>
          <code>{data.recipe}</code>
        </pre>
      </main>
    </Layout>
  );
}
