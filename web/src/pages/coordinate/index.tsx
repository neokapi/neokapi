import type { CSSProperties, ReactElement } from "react";
import Layout from "@theme/Layout";
import report from "./_coordinate.json";

// The /coordinate dashboard. Content sits somewhere: at a brand, on a channel,
// in a collection, and what governs it is bound at that place. This page shows
// what follows from that, and shows it rather than scoring it: every claim is
// the input, the output, and the code's own verdict between them.
//
// It spends no model calls. Resolution, the version chain, the governance gate
// and the fill decision are deterministic, so their behaviour is a fact rather
// than a sample, and scripts/coordinatereport's companion test fails when the
// committed data stops matching the code. A page that hardcoded a finding would
// keep asserting it after the code stopped supporting it.
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
interface Report {
  _note: string;
  recipe: string;
  points: Point[];
  chains: Chain[];
  prompts: PromptPair[];
  ladder: EditLadder;
  segments: SegmentSplit;
}

const data = report as Report;

const mono = "var(--ifm-font-family-monospace)";

const styles: Record<string, CSSProperties> = {
  lede: { fontSize: "1.15rem", lineHeight: 1.6, maxWidth: "62ch" },
  section: { marginTop: "3.5rem" },
  prose: { maxWidth: "64ch" },
  grid: { display: "grid", gap: "1rem" },
  card: {
    border: "1px solid var(--ifm-color-emphasis-300)",
    borderRadius: 8,
    padding: "1rem 1.15rem",
    background: "var(--ifm-background-surface-color)",
  },
  slug: { fontFamily: mono, fontSize: ".8rem", color: "var(--ifm-color-emphasis-700)" },
  kv: { display: "grid", gridTemplateColumns: "8.5rem 1fr", gap: ".3rem 1rem", fontSize: ".9rem" },
  key: { color: "var(--ifm-color-emphasis-700)" },
  val: { fontFamily: mono, fontSize: ".85rem", wordBreak: "break-all" },
  sideBySide: { display: "grid", gridTemplateColumns: "1fr 1fr", gap: "1rem" },
  pane: {
    border: "1px solid var(--ifm-color-emphasis-300)",
    borderRadius: 8,
    padding: ".85rem 1rem",
    background: "var(--ifm-background-surface-color)",
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
  added: {
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
  diffText: { fontSize: ".92rem", lineHeight: 1.6 },
  outcome: { fontSize: ".85rem", lineHeight: 1.45 },
  untranslated: { color: "var(--ifm-color-emphasis-600)", fontStyle: "italic" },
  legend: {
    display: "flex",
    flexWrap: "wrap",
    gap: "1.1rem",
    fontSize: ".8rem",
    color: "var(--ifm-color-emphasis-700)",
    margin: "1rem 0",
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

/** One side of a diff, with every span marked as the classifier saw it. */
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

/** Whether a side carries a word that moved, which decides if it is worth drawing. */
function hasWordChange(spans: Span[]): boolean {
  return spans.some((s) => s.op === "added" || s.op === "removed");
}

/**
 * The edit as a reader should check it: the current source with what changed
 * marked, and the prior source above it only when a word actually vanished
 * (otherwise the two lines are the same sentence twice).
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

/** A target the tool wrote, or the absence of one. */
function Outcome({ text }: { text: string }): ReactElement {
  if (!text) {
    return <span style={{ ...styles.outcome, ...styles.untranslated }}>left for a translator</span>;
  }
  return <span style={styles.outcome}>{text}</span>;
}

function DiffLegend(): ReactElement {
  return (
    <div style={styles.legend}>
      <span>
        <span style={diffSpanStyle.added}>a word appeared</span>
      </span>
      <span>
        <span style={diffSpanStyle.removed}>a word went</span>
      </span>
      <span>
        <span style={diffSpanStyle.cosmetic}>punctuation, case or quote shape</span>: shown, and not
        compared
      </span>
    </div>
  );
}

/** Which sections one pane has that the other does not, by origin. */
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
          style={highlight.has(s.origin) ? { ...styles.sect, ...styles.added } : styles.sect}
        >
          <div style={styles.sectOrigin}>{s.origin}</div>
          {s.heading && <div style={{ fontWeight: 600 }}>{s.heading}</div>}
          <div style={styles.sectText}>{s.text}</div>
        </div>
      ))}
    </>
  );
}

export default function Coordinate(): ReactElement {
  const offered = data.chains.filter((c) => c.offered).length;
  const withheld = data.chains.length - offered;
  const ladder = data.ladder;
  const seg = data.segments;

  return (
    <Layout
      title="Coordinates"
      description="What governs content at each point, which prior answers are reused, and what the recycle tool writes when an author edits."
    >
      <main className="container margin-vert--lg">
        <h1>Where content sits, and what governs it there</h1>
        <p style={styles.lede}>
          Content sits somewhere: at a product, on a channel, in a collection. What governs it is
          bound at that place. Everything below is computed by calling the resolver, the corpus, the
          segmenter and the recycle tool themselves, and every claim is shown as its input, its
          output, and the verdict between them. No model was asked anything, which is why these are
          facts rather than samples.
        </p>

        <section style={styles.section}>
          <h2>What resolves at each point</h2>
          <p style={styles.prose}>
            One recipe, {data.points.length} points. Each card is the <em>complete</em> governance
            resolved there. A partial answer would look identical whether a binding was correct or
            merely broader than intended.
          </p>
          <div
            style={{ ...styles.grid, gridTemplateColumns: "repeat(auto-fit, minmax(19rem, 1fr))" }}
          >
            {data.points.map((p) => (
              <div key={p.slug} style={styles.card}>
                <div style={{ display: "flex", justifyContent: "space-between", gap: ".5rem" }}>
                  <strong>{p.label}</strong>
                  {p.default && pill("default", "flat")}
                </div>
                <div style={{ ...styles.slug, marginBottom: ".7rem" }}>{p.slug}</div>
                <div style={styles.kv}>
                  <span style={styles.key}>voice</span>
                  <span style={styles.val}>{p.voiceFile ?? "—"}</span>
                  <span style={styles.key}>from</span>
                  <span style={styles.val}>{p.voiceField ?? "—"}</span>
                  <span style={styles.key}>terms</span>
                  <span style={styles.val}>{p.termStore || "the project's own"}</span>
                  <span style={styles.key}>content</span>
                  <span style={styles.val}>{(p.collections ?? []).join(", ") || "—"}</span>
                </div>
              </div>
            ))}
          </div>
        </section>

        <section style={styles.section}>
          <h2>What an author&rsquo;s edit puts in the file</h2>
          <p style={styles.prose}>
            One approved sentence, edited eleven ways, each run twice through the real recycle tool:
            once under a policy that reads the match percentage against a {ladder.fillFloor}% floor,
            once under the one that reads the edit. The last two columns are what each run actually
            wrote into the Norwegian file.
          </p>
          <div style={{ ...styles.card, marginBottom: "1rem" }}>
            <div style={styles.paneHead}>the approved pair</div>
            <div style={{ marginBottom: ".35rem" }}>{ladder.original}</div>
            <div style={{ color: "var(--ifm-color-emphasis-700)" }}>{ladder.target}</div>
          </div>
          <DiffLegend />
          <div style={styles.scroll}>
            <table style={styles.table}>
              <thead>
                <tr>
                  <th style={{ minWidth: "22rem" }}>The edit, as the classifier compares it</th>
                  <th>Score</th>
                  <th>Verdict</th>
                  <th style={{ minWidth: "14rem" }}>A percentage writes</th>
                  <th style={{ minWidth: "14rem" }}>The edit kind writes</th>
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
                          {r.harm}
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
          <p style={{ marginTop: "1.2rem", ...styles.prose }}>
            The two policies agree on {ladder.agreements} of these {ladder.rungs.length} and part
            company on {ladder.wrongFills}. Both of those are meaning changes the percentage fills,
            and they are not near the floor: they score{" "}
            {ladder.rungs
              .filter((r) => r.diverges && !r.safeToFill)
              .map((r) => r.score)
              .join(" and ")}
            , above every edit the same policy correctly refuses. That is what makes it a ranking
            problem rather than a tuning one. Moving the floor to catch them refuses the harmless
            edits first, and a full stop on a button label scores 91.
          </p>
          <p style={styles.prose}>
            A kind does not soften with length. A word appearing is a word appearing in a two-word
            label and in a paragraph, which is why the right-hand column sorts these the way a
            translator would.
          </p>
        </section>

        <section style={styles.section}>
          <h2>One number for a paragraph is one number too few</h2>
          <p style={styles.prose}>
            The same measurement at a coarser grain. A paragraph is one block, so one edited
            sentence gives the whole thing a single score and a single verdict, and at{" "}
            {seg.blockScore}% a percentage fills it, writing the old billing terms back into the
            file. Segmenting it with the {seg.engine} engine lets each sentence answer for itself:{" "}
            {seg.reusable} of {seg.segments.length} keep their approved wording and {seg.moved} is
            all anyone has to look at.
          </p>
          <div style={styles.sideBySide}>
            <div style={styles.pane}>
              <div style={styles.paneHead}>as one block · scores {seg.blockScore}</div>
              <EditView diff={seg.blockDiff} />
              <div style={{ marginTop: ".8rem", ...styles.kv }}>
                <span style={styles.key}>a percentage</span>
                <span style={styles.outcome}>{seg.blockFilledByScore}</span>
                <span style={styles.key}>the edit kind</span>
                <span>
                  <Outcome text={seg.blockFilled} />
                </span>
              </div>
            </div>
            <div style={styles.pane}>
              <div style={styles.paneHead}>the approved paragraph</div>
              <div style={{ color: "var(--ifm-color-emphasis-700)", fontSize: ".92rem" }}>
                {seg.approved}
              </div>
              <p style={{ ...styles.slug, marginTop: ".8rem", marginBottom: 0 }}>
                The fill above is this paragraph unchanged. Its middle sentence still says{" "}
                <em>dagen før</em>, the day before, and the English now says three days.
              </p>
            </div>
          </div>
          <div style={{ ...styles.scroll, marginTop: "1.5rem" }}>
            <table style={styles.table}>
              <thead>
                <tr>
                  <th>#</th>
                  <th style={{ minWidth: "24rem" }}>Sentence</th>
                  <th>Score</th>
                  <th>Verdict</th>
                  <th style={{ minWidth: "16rem" }}>What the tool wrote</th>
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
          <p style={{ marginTop: "1.2rem", ...styles.prose }}>
            Both grains are worth having. The block score answers &ldquo;has this paragraph
            changed&rdquo;; only the sentence split answers &ldquo;which part, and what still
            stands&rdquo;. The sentence that moved is also a better thing to hand a model: it
            arrives with its neighbours already approved beside it rather than as an
            undifferentiated lump of prose.
          </p>
        </section>

        <section style={styles.section}>
          <h2>Which prior answers are reused</h2>
          <p style={styles.prose}>
            A block&rsquo;s earlier approved answers are reference for translating it again, but
            only while the rules they were approved under still hold. The gate is asked here exactly
            as a producer asks it: {pill(`${offered} offered`, "ok")}{" "}
            {pill(`${withheld} withheld`, "no")}
          </p>
          <p style={styles.prose}>
            Note what the gate does <em>not</em> read: the wording column. Governance decides this,
            so an answer whose source barely moved is withheld when the rules moved under it, and
            one whose source moved a lot is offered when they did not.
          </p>
          <div style={styles.scroll}>
            <table style={styles.table}>
              <thead>
                <tr>
                  <th>Case</th>
                  <th style={{ minWidth: "18rem" }}>How the source moved</th>
                  <th>Approved under</th>
                  <th>In force now</th>
                  <th>Verdict</th>
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
                    <td style={styles.val}>{c.versions[0]?.fingerprint || "—"}</td>
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
          <p style={{ marginTop: "1rem", ...styles.prose }}>
            Two of these are worth reading twice. A profile whose <em>version</em> moved but whose
            guidance did not is still offered, because the fingerprint covers what actually reached
            the model, not a version number. And a term rule added about words this block does not
            contain withholds it anyway: the fingerprint covers every rule at the coordinate, which
            over-invalidates on purpose, because a staleness gate would rather re-check than miss.
          </p>
        </section>

        <section style={styles.section}>
          <h2>The prompt, both ways</h2>
          <p style={styles.prose}>
            The same block, rendered by the real prompt builder with and without the reference. The
            highlighted section is what the model gains, and the cache key beneath each pane must
            move with it, or a translation produced under one prior version would be served after
            the chain moved.
          </p>
          {data.prompts.map((p) => {
            const gained = addedOrigins(p.without, p.with);
            return (
              <div key={p.case} style={{ marginTop: "2rem" }}>
                <h3 style={{ marginBottom: ".2rem" }}>{p.case}</h3>
                <div style={{ ...styles.slug, marginBottom: ".8rem" }}>
                  translating “{p.source}”{p.withheld && `, reference withheld: ${p.withheld}`}
                </div>
                <div style={styles.sideBySide}>
                  <div style={styles.pane}>
                    <div style={styles.paneHead}>
                      without the prior answer · key {p.digests.without || "none"}
                    </div>
                    <SectionList sections={p.without} highlight={new Set()} />
                  </div>
                  <div style={styles.pane}>
                    <div style={styles.paneHead}>with it · key {p.digests.with || "none"}</div>
                    <SectionList sections={p.with} highlight={gained} />
                  </div>
                </div>
              </div>
            );
          })}
        </section>

        <section style={styles.section}>
          <h2>The recipe this is built from</h2>
          <p style={styles.prose}>
            Two profiles share a channel name, one declares a terms store and the other does not,
            and one collection binds to nothing. Those are the shapes that break resolvers.
          </p>
          <pre style={{ fontSize: ".8rem" }}>
            <code>{data.recipe}</code>
          </pre>
        </section>
      </main>
    </Layout>
  );
}
