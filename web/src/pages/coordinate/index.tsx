import type { CSSProperties, ReactElement } from "react";
import Layout from "@theme/Layout";
import report from "./_coordinate.json";

// The /coordinate dashboard. Content sits somewhere — at a brand, on a channel,
// in a collection — and what governs it is bound at that place. This page shows
// three things that follow from that, side by side, and every one of them is
// computed rather than described.
//
// It spends no model calls. Resolution, the version chain and the governance
// gate are deterministic, so their behaviour is a fact rather than a sample, and
// scripts/coordinatereport's companion test fails when the committed data stops
// matching the code. A page that hardcoded a finding would keep asserting it
// after the code stopped supporting it.
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
interface Report {
  _note: string;
  recipe: string;
  points: Point[];
  chains: Chain[];
  prompts: PromptPair[];
}

const data = report as Report;

const mono = "var(--ifm-font-family-monospace)";

const styles: Record<string, CSSProperties> = {
  lede: { fontSize: "1.15rem", lineHeight: 1.6, maxWidth: "60ch" },
  section: { marginTop: "3.5rem" },
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
  },
  table: { width: "100%", fontSize: ".88rem" },
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

  return (
    <Layout
      title="Coordinates"
      description="What governs content at each point, which prior answers are reused, and the prompt each produces."
    >
      <main className="container margin-vert--lg">
        <h1>Where content sits, and what governs it there</h1>
        <p style={styles.lede}>
          Content sits somewhere — at a product, on a channel, in a collection — and what governs it
          is bound at that place. Everything below is computed by calling the resolver, the corpus
          and the prompt builder themselves. No model was asked anything, which is why these are
          facts rather than samples.
        </p>

        <section style={styles.section}>
          <h2>What resolves at each point</h2>
          <p>
            One recipe, {data.points.length} points. Each card is the <em>complete</em> governance
            resolved there — a partial answer would look identical whether a binding was correct or
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
          <h2>Which prior answers are reused</h2>
          <p>
            A block's earlier approved answers are reference for translating it again — but only
            while the rules they were approved under still hold. The gate is asked here exactly as a
            producer asks it: {pill(`${offered} offered`, "ok")}{" "}
            {pill(`${withheld} withheld`, "no")}
          </p>
          <table style={styles.table}>
            <thead>
              <tr>
                <th>Case</th>
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
                    <div style={styles.slug}>
                      “{c.versions[0]?.source}” → “{c.versions[0]?.target}”
                    </div>
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
          <p style={{ marginTop: "1rem", maxWidth: "62ch" }}>
            Two of these are worth reading twice. A profile whose <em>version</em> moved but whose
            guidance did not is still offered — the fingerprint covers what actually reached the
            model, not a version number. And a term rule added about words this block does not
            contain withholds it anyway: the fingerprint covers every rule at the coordinate, which
            over-invalidates on purpose, because a staleness gate would rather re-check than miss.
          </p>
        </section>

        <section style={styles.section}>
          <h2>The prompt, both ways</h2>
          <p>
            The same block, rendered by the real prompt builder with and without the reference. The
            highlighted section is what the model gains — and the cache key beneath each pane must
            move with it, or a translation produced under one prior version would be served after
            the chain moved.
          </p>
          {data.prompts.map((p) => {
            const gained = addedOrigins(p.without, p.with);
            return (
              <div key={p.case} style={{ marginTop: "2rem" }}>
                <h3 style={{ marginBottom: ".2rem" }}>{p.case}</h3>
                <div style={{ ...styles.slug, marginBottom: ".8rem" }}>
                  translating “{p.source}”{p.withheld && ` — reference withheld: ${p.withheld}`}
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
          <p>
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
