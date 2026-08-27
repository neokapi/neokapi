import type { CSSProperties, ReactElement } from "react";
import Layout from "@theme/Layout";
import report from "./_authoringeval.json";
import { mono, tone, s as shared, ageDays, STALE_AFTER_DAYS } from "../evals/_shared";

// The authoring dashboard: the voice checks, voice-infer, and whether the voice
// guide steers writing or only improves it. Regenerate with `make authoring-eval`.
//
// Every number here is a per-document row you can open. A summary nobody can
// drill into is a claim, and the point of this page is that each one is checkable
// against the prose it came from.

interface Plant {
  text: string;
  rule: string;
  mechanism: string;
}
interface Doc {
  name: string;
  body: string;
  kind: string;
  plants?: Plant[];
}
interface Finding {
  category: string;
  severity: string;
  message: string;
  original_text: string;
}
interface DocOutcome {
  doc: string;
  kind: string;
  score: number;
  plants?: Plant[];
  findings?: Finding[];
  covered?: string[];
  missed?: Plant[];
  error?: string;
}
interface MechanismScore {
  plants: number;
  covered: number;
  recall: number;
}
interface CheckAccuracy {
  check: string;
  surface: string;
  plants: number;
  covered: number;
  recall: number;
  findings: number;
  byMechanism: Record<string, MechanismScore>;
  cleanDocs: number;
  cleanFalsePositives: number;
  cleanDocsWithFindings: number;
  outcomes: DocOutcome[];
  blocked?: string;
}
interface FieldRecovery {
  field: string;
  reference: string;
  draft: string;
  recovered: boolean;
  partial?: boolean;
  ratio?: number;
}
interface InferResult {
  surface: string;
  sources: number;
  draft?: string;
  fields?: FieldRecovery[];
  recovered: number;
  compared: number;
  rate: number;
  invented?: string[];
  blocked?: string;
}
interface SteerDoc {
  task: string;
  bare: string;
  steered: string;
  bareRef: number;
  steeredRef: number;
  bareContrast: number;
  steeredContrast: number;
  bareFindings?: Finding[];
  steeredFindings?: Finding[];
  error?: string;
}
interface SteerResult {
  provider: string;
  model: string;
  guide: string;
  docs: SteerDoc[];
  meanBareRef: number;
  meanSteeredRef: number;
  meanBareContrast: number;
  meanSteeredContrast: number;
  refGain: number;
  contrastGain: number;
  effect: number;
  scored: number;
  blocked?: string;
}
interface CorpusInfo {
  synthetic: boolean;
  disclosure: string;
  docs: number;
  onProfile: number;
  offProfile: number;
  plants: number;
  plantsByMechanism: Record<string, number>;
  profile: string;
  contrast: string;
  documents: Doc[];
}
interface Report {
  date: string;
  corpus: CorpusInfo;
  checks: CheckAccuracy[];
  infer?: InferResult;
  steer?: SteerResult;
}

const r = report as unknown as Report;

const s: Record<string, CSSProperties> = {
  wrap: { maxWidth: 1080, margin: "0 auto", padding: "2rem 1rem 4rem" },
  h2: { fontSize: "1.35rem", marginTop: "2.6rem", marginBottom: ".4rem" },
  sub: { color: "var(--ifm-color-emphasis-700)", maxWidth: "70ch", lineHeight: 1.6 },
  card: {
    border: "1px solid var(--ifm-color-emphasis-300)",
    borderRadius: 8,
    padding: "1.1rem 1.3rem",
    marginTop: "1rem",
    background: "var(--ifm-background-surface-color)",
  },
  bignum: {
    fontSize: "2rem",
    fontWeight: 700,
    fontVariantNumeric: "tabular-nums",
    lineHeight: 1.1,
  },
  label: {
    fontSize: ".72rem",
    textTransform: "uppercase",
    letterSpacing: ".06em",
    color: "var(--ifm-color-emphasis-600)",
  },
  code: {
    fontFamily: mono,
    fontSize: ".8rem",
    background: "var(--ifm-color-emphasis-100)",
    padding: ".15rem .35rem",
    borderRadius: 4,
  },
  pre: {
    fontFamily: mono,
    fontSize: ".78rem",
    lineHeight: 1.55,
    whiteSpace: "pre-wrap",
    margin: 0,
    padding: ".7rem .85rem",
    background: "var(--ifm-color-emphasis-100)",
    borderRadius: 6,
    overflowX: "auto",
  },
  scroll: { overflowX: "auto" },
  table: { width: "100%", borderCollapse: "collapse", fontSize: ".88rem" },
  num: { textAlign: "right", fontVariantNumeric: "tabular-nums" },
  two: {
    display: "grid",
    gridTemplateColumns: "repeat(auto-fit, minmax(280px, 1fr))",
    gap: "1rem",
  },
  banner: {
    borderLeft: "4px solid var(--ifm-color-warning)",
    background: "var(--ifm-color-warning-contrast-background)",
    color: "var(--ifm-color-warning-contrast-foreground)",
    padding: ".85rem 1.1rem",
    borderRadius: "0 6px 6px 0",
    marginTop: "1rem",
    lineHeight: 1.6,
  },
};

function Pill({ text, t }: { text: string; t: keyof typeof tone }): ReactElement {
  return (
    <span
      style={{
        ...tone[t],
        fontFamily: mono,
        fontSize: ".72rem",
        padding: ".1rem .45rem",
        borderRadius: 4,
        whiteSpace: "nowrap",
      }}
    >
      {text}
    </span>
  );
}

const pct = (v: number) => `${Math.round(100 * v)}%`;
const signed = (v: number) => `${v > 0 ? "+" : ""}${v.toFixed(1)}`;

// A gain is good on the profile the guide came from and bad on the one it did
// not, so the same number reads in opposite directions depending on the column.
function gainTone(v: number, wantUp: boolean): keyof typeof tone {
  if (v === 0) return "flat";
  return v > 0 === wantUp ? "ok" : "gap";
}

function Freshness(): ReactElement | null {
  const age = ageDays(r);
  if (age === undefined) return null;
  const stale = age > STALE_AFTER_DAYS;
  return (
    <Pill
      text={age === 0 ? "measured today" : `measured ${age}d ago`}
      t={stale ? "warn" : "flat"}
    />
  );
}

// ---------------------------------------------------------------------------

function CorpusSection(): ReactElement {
  const c = r.corpus;
  return (
    <section>
      <h2 style={s.h2}>The corpus</h2>
      <div style={s.banner}>
        <strong>Synthesized, and disclosed in the data.</strong> {c.disclosure}
      </div>
      <div style={{ ...s.card, ...s.two }}>
        <div>
          <div style={s.label}>documents</div>
          <div style={s.bignum}>{c.docs}</div>
          <div style={s.sub}>
            {c.onProfile} written to the profile, {c.offProfile} written against it
          </div>
        </div>
        <div>
          <div style={s.label}>marked violations</div>
          <div style={s.bignum}>{c.plants}</div>
          <div style={s.sub}>
            {Object.entries(c.plantsByMechanism)
              .sort()
              .map(([k, v]) => `${v} ${k}`)
              .join(" · ")}
          </div>
        </div>
      </div>
      <details style={s.card}>
        <summary>
          The reference profile — what the on-profile half was written from, and what an inferred
          draft is compared against
        </summary>
        <pre style={{ ...s.pre, marginTop: ".8rem" }}>{c.profile}</pre>
      </details>
      <details style={s.card}>
        <summary>The contrast profile — the control for the steering measurement</summary>
        <p style={s.sub}>
          It wants the opposite on every axis the schema allows. Steering toward the reference
          should push a document away from this one; guidance that only says &ldquo;write
          well&rdquo; would raise both.
        </p>
        <pre style={s.pre}>{c.contrast}</pre>
      </details>
      <details style={s.card}>
        <summary>All {c.docs} documents, with their marked violations</summary>
        {c.documents.map((d) => (
          <div key={d.name} style={{ marginTop: "1rem" }}>
            <div style={{ display: "flex", gap: ".5rem", alignItems: "center" }}>
              <code style={s.code}>{d.name}</code>
              <Pill text={d.kind} t={d.kind === "on-profile" ? "ok" : "warn"} />
              {d.plants?.length ? (
                <span style={s.sub}>{d.plants.length} marked</span>
              ) : (
                <span style={s.sub}>nothing marked — the right answer is silence</span>
              )}
            </div>
            <pre style={{ ...s.pre, marginTop: ".4rem" }}>{d.body}</pre>
            {d.plants?.length ? (
              <ul style={{ marginTop: ".4rem", fontSize: ".85rem" }}>
                {d.plants.map((p) => (
                  <li key={p.text}>
                    <code style={s.code}>{p.text}</code> — {p.rule}{" "}
                    <Pill text={p.mechanism} t="flat" />
                  </li>
                ))}
              </ul>
            ) : null}
          </div>
        ))}
      </details>
    </section>
  );
}

function CheckSection({ a }: { a: CheckAccuracy }): ReactElement {
  return (
    <div style={s.card}>
      <div style={{ display: "flex", gap: ".6rem", alignItems: "baseline", flexWrap: "wrap" }}>
        <strong>{a.check}</strong>
        {a.blocked ? <Pill text="not measured" t="gap" /> : null}
      </div>
      <code style={{ ...s.code, display: "inline-block", marginTop: ".4rem" }}>{a.surface}</code>
      {a.blocked ? (
        <div style={s.banner}>{a.blocked}</div>
      ) : (
        <>
          <div style={{ ...s.two, marginTop: "1rem" }}>
            <div>
              <div style={s.label}>recall over marked violations</div>
              <div style={s.bignum}>{pct(a.recall)}</div>
              <div style={s.sub}>
                {a.covered} of {a.plants} found
              </div>
            </div>
            <div>
              <div style={s.label}>false positives on clean prose</div>
              <div style={s.bignum}>{a.cleanFalsePositives}</div>
              <div style={s.sub}>
                across {a.cleanDocsWithFindings} of {a.cleanDocs} documents where the right answer
                is silence
              </div>
            </div>
          </div>
          <div style={{ ...s.scroll, marginTop: "1rem" }}>
            <table style={s.table}>
              <thead>
                <tr>
                  <th style={{ textAlign: "left" }}>how the profile states the rule</th>
                  <th style={s.num}>found</th>
                  <th style={s.num}>marked</th>
                  <th style={s.num}>recall</th>
                </tr>
              </thead>
              <tbody>
                {Object.entries(a.byMechanism)
                  .sort()
                  .map(([k, m]) => (
                    <tr key={k} style={{ borderTop: "1px solid var(--ifm-color-emphasis-200)" }}>
                      <td>
                        <code style={s.code}>{k}</code>
                      </td>
                      <td style={s.num}>{m.covered}</td>
                      <td style={s.num}>{m.plants}</td>
                      <td style={s.num}>
                        <Pill
                          text={pct(m.recall)}
                          t={m.recall === 1 ? "ok" : m.recall === 0 ? "gap" : "warn"}
                        />
                      </td>
                    </tr>
                  ))}
              </tbody>
            </table>
          </div>
          <details style={{ marginTop: "1rem" }}>
            <summary>Every document, with what the check said about it</summary>
            {a.outcomes.map((o) => (
              <div key={o.doc} style={{ marginTop: ".9rem" }}>
                <div
                  style={{ display: "flex", gap: ".5rem", alignItems: "center", flexWrap: "wrap" }}
                >
                  <code style={s.code}>{o.doc}</code>
                  <Pill text={o.kind} t={o.kind === "on-profile" ? "ok" : "warn"} />
                  <span style={s.sub}>score {o.score}</span>
                  {o.missed?.length ? <Pill text={`${o.missed.length} missed`} t="gap" /> : null}
                </div>
                {o.error ? <div style={s.banner}>{o.error}</div> : null}
                {o.missed?.length ? (
                  <div style={{ fontSize: ".85rem", marginTop: ".3rem" }}>
                    missed:{" "}
                    {o.missed.map((m) => (
                      <code key={m.text} style={{ ...s.code, marginRight: ".3rem" }}>
                        {m.text}
                      </code>
                    ))}
                  </div>
                ) : null}
                {o.findings?.length ? (
                  <ul style={{ fontSize: ".85rem", marginTop: ".3rem" }}>
                    {o.findings.map((f, i) => (
                      <li key={`${f.original_text}-${i}`}>
                        <code style={s.code}>{f.original_text}</code> — {f.message}{" "}
                        {o.covered?.[i] ? (
                          <Pill text={`on “${o.covered[i]}”`} t="ok" />
                        ) : (
                          <Pill text="not marked" t="flat" />
                        )}
                      </li>
                    ))}
                  </ul>
                ) : (
                  <div style={{ ...s.sub, fontSize: ".85rem" }}>no findings</div>
                )}
              </div>
            ))}
          </details>
        </>
      )}
    </div>
  );
}

function InferSection(): ReactElement | null {
  const i = r.infer;
  if (!i) return null;
  return (
    <section>
      <h2 style={s.h2}>Does an inferred profile recover the one the corpus was written from?</h2>
      <p style={s.sub}>
        The on-profile half was written from the reference profile, so recovery is checkable field
        by field rather than by asking a judge.
      </p>
      <div style={s.card}>
        <code style={s.code}>{i.surface}</code>
        {i.blocked ? (
          <div style={s.banner}>{i.blocked}</div>
        ) : (
          <>
            <div style={{ marginTop: "1rem" }}>
              <div style={s.label}>fields recovered</div>
              <div style={s.bignum}>{pct(i.rate)}</div>
              <div style={s.sub}>
                {i.recovered} of {i.compared}, from {i.sources} documents
              </div>
            </div>
            <div style={{ ...s.scroll, marginTop: "1rem" }}>
              <table style={s.table}>
                <thead>
                  <tr>
                    <th style={{ textAlign: "left" }}>field</th>
                    <th style={{ textAlign: "left" }}>reference</th>
                    <th style={{ textAlign: "left" }}>draft</th>
                  </tr>
                </thead>
                <tbody>
                  {i.fields?.map((f) => (
                    <tr
                      key={f.field}
                      style={{ borderTop: "1px solid var(--ifm-color-emphasis-200)" }}
                    >
                      <td>
                        <code style={s.code}>{f.field}</code>{" "}
                        <Pill
                          text={f.recovered ? "ok" : f.partial ? pct(f.ratio ?? 0) : "miss"}
                          t={f.recovered ? "ok" : f.partial ? "warn" : "gap"}
                        />
                      </td>
                      <td>{f.reference}</td>
                      <td>{f.draft || "—"}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            {i.draft ? (
              <details style={{ marginTop: "1rem" }}>
                <summary>The draft, verbatim</summary>
                <pre style={s.pre}>{i.draft}</pre>
              </details>
            ) : null}
          </>
        )}
      </div>
    </section>
  );
}

function SteerSection(): ReactElement | null {
  const t = r.steer;
  if (!t) return null;
  return (
    <section>
      <h2 style={s.h2}>Does the voice guide steer writing, or only improve it?</h2>
      <p style={s.sub}>
        Each brief is written twice by {t.provider}:{t.model} — once with nothing, once with{" "}
        <code style={s.code}>kapi voice guide</code> as the system turn. The user turn is identical.
        Both versions are scored against the profile the guide came from and against a profile that
        wants the opposite, because any competent writing guidance raises any reasonable score. The
        measurement is the difference between the two gains.
      </p>
      {t.blocked ? (
        <div style={s.banner}>{t.blocked}</div>
      ) : (
        <>
          <div style={{ ...s.card, ...s.two }}>
            <div>
              <div style={s.label}>toward the profile the guide came from</div>
              <div style={{ ...s.bignum, color: tone.ok.fg }}>{signed(t.refGain)}</div>
              <div style={s.sub}>
                {t.meanBareRef.toFixed(1)} → {t.meanSteeredRef.toFixed(1)} mean score
              </div>
            </div>
            <div>
              <div style={s.label}>toward the profile it did not</div>
              <div style={{ ...s.bignum, color: tone.gap.fg }}>{signed(t.contrastGain)}</div>
              <div style={s.sub}>
                {t.meanBareContrast.toFixed(1)} → {t.meanSteeredContrast.toFixed(1)} mean score
              </div>
            </div>
            <div>
              <div style={s.label}>effect</div>
              <div style={s.bignum}>{signed(t.effect)}</div>
              <div style={s.sub}>
                the difference of the two, over {t.scored} documents. Generic polish would move both
                gains the same way and land near zero.
              </div>
            </div>
          </div>

          <div style={{ ...s.scroll, marginTop: "1rem" }}>
            <table style={s.table}>
              <thead>
                <tr>
                  <th style={{ textAlign: "left" }}>brief</th>
                  <th style={s.num}>reference</th>
                  <th style={s.num}>contrast</th>
                </tr>
              </thead>
              <tbody>
                {t.docs.map((d) => (
                  <tr key={d.task} style={{ borderTop: "1px solid var(--ifm-color-emphasis-200)" }}>
                    <td style={{ maxWidth: "42ch" }}>{d.task}</td>
                    <td style={s.num}>
                      {d.bareRef} → {d.steeredRef}{" "}
                      <Pill
                        text={signed(d.steeredRef - d.bareRef)}
                        t={gainTone(d.steeredRef - d.bareRef, true)}
                      />
                    </td>
                    <td style={s.num}>
                      {d.bareContrast} → {d.steeredContrast}{" "}
                      <Pill
                        text={signed(d.steeredContrast - d.bareContrast)}
                        t={gainTone(d.steeredContrast - d.bareContrast, false)}
                      />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          <details style={s.card}>
            <summary>The guide, verbatim — the whole measurement is about this string</summary>
            <pre style={s.pre}>{t.guide}</pre>
          </details>

          <details style={s.card}>
            <summary>Every brief, both versions side by side</summary>
            {t.docs.map((d) => (
              <div key={d.task} style={{ marginTop: "1.4rem" }}>
                <div style={{ fontWeight: 600, fontSize: ".9rem" }}>{d.task}</div>
                {d.error ? <div style={s.banner}>{d.error}</div> : null}
                <div style={{ ...s.two, marginTop: ".5rem" }}>
                  <div>
                    <div style={s.label}>bare — no guide</div>
                    <pre style={s.pre}>{d.bare}</pre>
                    <Findings list={d.bareFindings} />
                  </div>
                  <div>
                    <div style={s.label}>guided</div>
                    <pre style={s.pre}>{d.steered}</pre>
                    <Findings list={d.steeredFindings} />
                  </div>
                </div>
              </div>
            ))}
          </details>
        </>
      )}
    </section>
  );
}

function Findings({ list }: { list?: Finding[] }): ReactElement {
  if (!list?.length) {
    return (
      <div style={{ ...s.sub, fontSize: ".82rem", marginTop: ".3rem" }}>
        <Pill text="no findings" t="ok" />
      </div>
    );
  }
  return (
    <ul style={{ fontSize: ".82rem", marginTop: ".3rem" }}>
      {list.map((f, i) => (
        <li key={`${f.original_text}-${i}`}>
          <code style={s.code}>{f.original_text}</code> — {f.message}
        </li>
      ))}
    </ul>
  );
}

export default function AuthoringEval(): ReactElement {
  return (
    <Layout
      title="Authoring eval"
      description="Whether the voice checks find real violations, and whether the voice guide steers writing toward its profile"
    >
      <main style={s.wrap}>
        <h1 style={{ marginBottom: ".3rem" }}>Authoring</h1>
        <div style={{ display: "flex", gap: ".5rem", alignItems: "center", marginBottom: ".8rem" }}>
          <Freshness />
          <code style={s.code}>make authoring-eval</code>
        </div>
        <p style={shared.lede}>
          Three questions about the authoring side, measured on one corpus: do the voice checks find
          the violations a profile declares, does an inferred profile recover the one the material
          was written from, and does the voice guide steer writing toward its profile rather than
          merely improving it.
        </p>

        <CorpusSection />

        <section>
          <h2 style={s.h2}>Do the voice checks find what the profile declares?</h2>
          <p style={s.sub}>
            Recall is measured over the documents written against the profile, where every violation
            is marked. False positives are measured over the documents written to it, where the
            right answer is silence. Neither half can substitute for the other: an off-profile
            document contains violations beyond the marked ones, so counting unmarked findings there
            would measure how complete the marking is rather than how good the check is.
          </p>
          {r.checks.map((a) => (
            <CheckSection key={a.check} a={a} />
          ))}
        </section>

        <InferSection />
        <SteerSection />
      </main>
    </Layout>
  );
}
