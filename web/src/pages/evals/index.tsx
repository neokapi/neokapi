import { Fragment } from "react";
import type { CSSProperties, ReactElement } from "react";
import Layout from "@theme/Layout";
import Link from "@docusaurus/Link";
import index from "./_index.json";

// The /evals cover page: every measurement kapi publishes, grouped by the
// question it answers.
//
// Organised by question rather than by eval, and the questions are questions
// rather than claims, because a question can be answered badly. "Is authoring
// governed too" reads honestly when the answer is "not measured yet"; a heading
// asserting that it is would not.
//
// The rows that matter most are the absent ones. A gap a reader cannot see is a
// gap they will assume is covered, so an unbuilt eval carries the same card as
// a built one, minus its data, and says plainly what it would have measured.
//
// Regenerate with `go run ./scripts/evalindex`.

interface Question {
  id: string;
  ask: string;
  why: string;
  evals: string[];
}
interface Eval {
  id: string;
  title: string;
  method: "deterministic" | "labelled" | "judged" | "benchmark" | "comparative";
  status: "measured" | "partial" | "unvalidated" | "absent";
  spends?: boolean;
  corpus: string;
  covers: string;
  misses?: string;
  reproduce?: string;
  data?: string;
  page?: string;
  validation?: string;
  settings?: string;
}
interface Coverage {
  measured: number;
  partial: number;
  unvalidated: number;
  absent: number;
  questionsUnanswered: number;
}
interface Index {
  _note: string;
  questions: Question[];
  evals: Eval[];
  coverage: Coverage;
}

const data = index as Index;
const byID = new Map(data.evals.map((e) => [e.id, e]));

const mono = "var(--ifm-font-family-monospace)";

const styles: Record<string, CSSProperties> = {
  lede: { fontSize: "1.1rem", lineHeight: 1.6, maxWidth: "64ch" },
  prose: { maxWidth: "66ch" },
  strip: {
    display: "flex",
    flexWrap: "wrap",
    gap: "1.5rem",
    padding: "1.1rem 1.3rem",
    border: "1px solid var(--ifm-color-emphasis-300)",
    borderRadius: 8,
    background: "var(--ifm-background-surface-color)",
    margin: "1.8rem 0 1rem",
  },
  stat: { display: "flex", flexDirection: "column", gap: ".15rem" },
  statN: { fontSize: "1.5rem", fontWeight: 650, lineHeight: 1, fontVariantNumeric: "tabular-nums" },
  statL: {
    fontSize: ".72rem",
    letterSpacing: ".1em",
    textTransform: "uppercase",
    color: "var(--ifm-color-emphasis-700)",
  },
  question: {
    paddingTop: "2.5rem",
    marginTop: "2.5rem",
    borderTop: "1px solid var(--ifm-color-emphasis-300)",
  },
  ask: { fontSize: "1.4rem", fontWeight: 640, letterSpacing: "-.015em", margin: "0 0 .5rem" },
  why: { color: "var(--ifm-color-emphasis-800)", maxWidth: "66ch", margin: "0 0 1.3rem" },
  cards: { display: "grid", gap: "1rem" },
  card: {
    border: "1px solid var(--ifm-color-emphasis-300)",
    borderRadius: 8,
    padding: "1.1rem 1.25rem",
    background: "var(--ifm-background-surface-color)",
  },
  cardAbsent: {
    border: "1px dashed var(--ifm-color-emphasis-400)",
    borderRadius: 8,
    padding: "1.1rem 1.25rem",
    background: "transparent",
  },
  cardHead: {
    display: "flex",
    justifyContent: "space-between",
    alignItems: "baseline",
    gap: ".8rem",
    flexWrap: "wrap",
    marginBottom: ".5rem",
  },
  title: { fontWeight: 640, fontSize: "1.02rem" },
  pills: { display: "flex", gap: ".4rem", flexWrap: "wrap" },
  pill: {
    fontFamily: mono,
    fontSize: ".67rem",
    letterSpacing: ".08em",
    textTransform: "uppercase",
    padding: ".15rem .45rem",
    borderRadius: 4,
    whiteSpace: "nowrap",
  },
  row: {
    display: "grid",
    gridTemplateColumns: "6.5rem 1fr",
    gap: ".35rem 1rem",
    fontSize: ".9rem",
    lineHeight: 1.5,
    marginTop: ".5rem",
  },
  key: { color: "var(--ifm-color-emphasis-700)" },
  // Infima's `-contrast-foreground` swaps with the theme; `-darker` does not.
  // Misses in `--ifm-color-danger-darker` measured 3.2:1 on the dark card
  // surface, under the 4.5:1 floor, which would have made the honest half of
  // every card the hardest part to read for anyone browsing in dark mode.
  misses: { color: "var(--ifm-color-danger-contrast-foreground)" },
  cmd: { fontFamily: mono, fontSize: ".8rem", wordBreak: "break-all" },
};

// Callers place pills as JSX siblings, never as an array, so the pill owns no
// key. Deriving one from its text collided the moment a card's method and its
// status were the same word, which model-cost (measured/measured) is.
function pill(text: string, tone: "ok" | "warn" | "gap" | "flat"): ReactElement {
  const bg = {
    ok: "var(--ifm-color-success-contrast-background)",
    warn: "var(--ifm-color-warning-contrast-background)",
    gap: "var(--ifm-color-danger-contrast-background)",
    flat: "var(--ifm-color-emphasis-200)",
  }[tone];
  // Paired deliberately: each `-contrast-background` has a matching
  // `-contrast-foreground`, and both sides of the pair invert together in dark
  // mode. Mixing in a `-darker` foreground puts mid-red on dark red.
  const fg = {
    ok: "var(--ifm-color-success-contrast-foreground)",
    warn: "var(--ifm-color-warning-contrast-foreground)",
    gap: "var(--ifm-color-danger-contrast-foreground)",
    flat: "var(--ifm-color-emphasis-800)",
  }[tone];
  return <span style={{ ...styles.pill, background: bg, color: fg }}>{text}</span>;
}

const statusTone = {
  measured: "ok",
  partial: "warn",
  unvalidated: "warn",
  absent: "gap",
} as const;

// The status id is "absent", which is the right word for a registry and the
// wrong one for a reader: it invites "absent from what?". Every surface shows
// "not measured" instead, and one map keeps the pill, the legend and the
// summary strip from drifting apart on it.
const statusLabel: Record<Eval["status"], string> = {
  measured: "measured",
  partial: "partial",
  unvalidated: "unvalidated",
  absent: "not measured",
};

/** What a status means, said once here rather than assumed. */
const statusMeans: Record<Eval["status"], string> = {
  measured: "runs, data committed, numbers can be read as they stand",
  partial: "runs, and covers less than the question needs",
  unvalidated: "produces numbers that should not yet be relied on",
  absent: "nothing measures this",
};

function EvalCard({ e }: { e: Eval }): ReactElement {
  const absent = e.status === "absent";
  return (
    <div style={absent ? styles.cardAbsent : styles.card}>
      <div style={styles.cardHead}>
        <span style={styles.title}>{e.page ? <Link to={e.page}>{e.title}</Link> : e.title}</span>
        <span style={styles.pills}>
          {pill(e.method, "flat")}
          {e.spends && pill("spends", "flat")}
          {pill(statusLabel[e.status], statusTone[e.status])}
        </span>
      </div>
      <div style={styles.row}>
        <span style={styles.key}>Covers</span>
        <span>{e.covers}</span>
        {e.misses && (
          <>
            <span style={styles.key}>Misses</span>
            <span style={styles.misses}>{e.misses}</span>
          </>
        )}
        <span style={styles.key}>Corpus</span>
        <span>{e.corpus}</span>
        {e.settings && (
          <>
            <span style={styles.key}>Settings</span>
            <span style={e.settings.startsWith("Not recorded") ? styles.misses : undefined}>
              {e.settings}
            </span>
          </>
        )}
        {e.validation && (
          <>
            <span style={styles.key}>Validation</span>
            <span>{e.validation}</span>
          </>
        )}
        {e.reproduce && (
          <>
            <span style={styles.key}>Reproduce</span>
            <code style={styles.cmd}>{e.reproduce}</code>
          </>
        )}
      </div>
    </div>
  );
}

export default function Evals(): ReactElement {
  const c = data.coverage;
  const total = c.measured + c.partial + c.unvalidated + c.absent;

  return (
    <Layout
      title="Tests and Evals"
      description="Every measurement kapi publishes, grouped by the question it answers, including the questions nothing answers yet."
    >
      <main className="container margin-vert--lg">
        <h1>Tests and Evals</h1>
        <p style={styles.lede}>
          What kapi is measured on, what those measurements say, and where they stop. Grouped by the
          question a reader might arrive with rather than by the tool that answers it.
        </p>

        <div style={styles.strip}>
          <div style={styles.stat}>
            <span style={styles.statN}>{c.measured}</span>
            <span style={styles.statL}>measured</span>
          </div>
          <div style={styles.stat}>
            <span style={styles.statN}>{c.partial}</span>
            <span style={styles.statL}>partial</span>
          </div>
          <div style={styles.stat}>
            <span style={styles.statN}>{c.unvalidated}</span>
            <span style={styles.statL}>unvalidated</span>
          </div>
          <div style={styles.stat}>
            <span style={styles.statN}>{c.absent}</span>
            <span style={styles.statL}>{statusLabel.absent}</span>
          </div>
          <div style={{ ...styles.stat, marginLeft: "auto" }}>
            <span style={styles.statN}>
              {c.questionsUnanswered}/{data.questions.length}
            </span>
            <span style={styles.statL}>questions with no answer</span>
          </div>
        </div>

        <p style={styles.prose}>
          Of {total} evals, {c.absent} are not built. Those are the most useful rows on this page: a
          gap a reader cannot see is a gap they will assume is covered. Each unbuilt eval carries
          the same card as a built one and says what it would have measured.
        </p>
        <p style={styles.prose}>
          A card states what an eval <strong>misses</strong> as well as what it covers, names the
          corpus it ran over, and gives the command that reproduces it. That last one is not
          courtesy: across more than 50,000 published evaluation records surveyed in 2026, 96.5%
          were missing at least one field needed to re-run them. A number nobody can reproduce is an
          assertion with a table around it.
        </p>
        <p style={styles.prose}>
          By that standard three cards here fall short today, and their <strong>Settings</strong>{" "}
          row says so. Every eval that calls a hosted model runs at whatever sampling the API
          defaults to, and no harness writes the value down, so re-running one of those commands
          gives you the method rather than the number.
        </p>

        <h2 style={{ marginTop: "2.5rem" }}>What the statuses mean</h2>
        <div style={{ ...styles.row, gridTemplateColumns: "8rem 1fr", maxWidth: "62rem" }}>
          {(Object.keys(statusMeans) as Eval["status"][]).map((s) => (
            <Fragment key={s}>
              <span>{pill(statusLabel[s], statusTone[s])}</span>
              <span>{statusMeans[s]}</span>
            </Fragment>
          ))}
        </div>

        {data.questions.map((q) => {
          const list = q.evals.map((id) => byID.get(id)).filter((e): e is Eval => Boolean(e));
          const unanswered = list.every((e) => e.status === "absent");
          return (
            <section key={q.id} style={styles.question}>
              <h2 style={styles.ask}>
                {q.ask} {unanswered && pill("no answer yet", "gap")}
              </h2>
              <p style={styles.why}>{q.why}</p>
              <div style={styles.cards}>
                {list.map((e) => (
                  <EvalCard key={e.id} e={e} />
                ))}
              </div>
            </section>
          );
        })}
      </main>
    </Layout>
  );
}
