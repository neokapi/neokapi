import type { CSSProperties, ReactElement } from "react";
import Layout from "@theme/Layout";
import report from "./_conversioneval.json";
import { mono, tone, ageDays, STALE_AFTER_DAYS } from "../evals/_shared";

// The conversion dashboard. Regenerate with `make conversion-eval`.
//
// The per-extension tables lead, and the corpus-wide table is explicitly not a
// ranking: the tools read different file sets, so their overall numbers are
// computed over different documents. Only within one extension did every
// converter see the same files.

interface MissingWord {
  word: string;
  times: number;
}
interface FileResult {
  converter: string;
  file: string;
  ext: string;
  bytes: number;
  ms: number;
  truthWords: number;
  outputWords: number;
  matched: number;
  recall: number;
  missing?: MissingWord[];
  error?: string;
}
interface ExtScore {
  files: number;
  failed: number;
  recall: number;
  truthWords: number;
  matchedWords: number;
  meanRecall: number;
  medianRecall: number;
  medianMs: number;
  topFile?: string;
  topFileShare?: number;
}
interface ConverterResult {
  id: string;
  version: string;
  command: string;
  exts: string[];
  note: string;
  files: number;
  failed: number;
  recall: number;
  truthWords: number;
  matchedWords: number;
  meanRecall: number;
  medianRecall: number;
  tiny: number;
  perfect: number;
  medianMs: number;
  totalMs: number;
  worst?: FileResult[];
  byExt: Record<string, ExtScore>;
}
interface Report {
  date: string;
  corpus: {
    source: string;
    files: number;
    byExt: Record<string, number>;
    sampled: boolean;
    available: number;
  };
  method: string;
  limits: string;
  converters: ConverterResult[];
}

const r = report as unknown as Report;

const s: Record<string, CSSProperties> = {
  wrap: { maxWidth: 1080, margin: "0 auto", padding: "2rem 1rem 4rem" },
  lede: { fontSize: "1.05rem", lineHeight: 1.6, maxWidth: "68ch" },
  h2: { fontSize: "1.35rem", marginTop: "2.6rem", marginBottom: ".4rem" },
  sub: { color: "var(--ifm-color-emphasis-700)", maxWidth: "72ch", lineHeight: 1.6 },
  card: {
    border: "1px solid var(--ifm-color-emphasis-300)",
    borderRadius: 8,
    padding: "1.1rem 1.3rem",
    marginTop: "1rem",
    background: "var(--ifm-background-surface-color)",
  },
  code: {
    fontFamily: mono,
    fontSize: ".8rem",
    background: "var(--ifm-color-emphasis-100)",
    padding: ".15rem .35rem",
    borderRadius: 4,
  },
  scroll: { overflowX: "auto" },
  table: { width: "100%", borderCollapse: "collapse", fontSize: ".88rem" },
  num: { textAlign: "right", fontVariantNumeric: "tabular-nums" },
  th: { textAlign: "left", fontSize: ".76rem", textTransform: "uppercase", letterSpacing: ".05em" },
  row: { borderTop: "1px solid var(--ifm-color-emphasis-200)" },
  note: { fontSize: ".8rem", color: "var(--ifm-color-emphasis-600)" },
  banner: {
    borderLeft: "4px solid var(--ifm-color-emphasis-400)",
    background: "var(--ifm-color-emphasis-100)",
    padding: ".85rem 1.1rem",
    borderRadius: "0 6px 6px 0",
    marginTop: "1rem",
    lineHeight: 1.6,
    maxWidth: "72ch",
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

// Mirrors dominationThreshold in scripts/conversioneval. A third of a
// format's words in one document is enough that the word-weighted column is
// substantially about that file.
const DOMINATION_THRESHOLD = 0.3;

const pct = (v: number) => `${(100 * v).toFixed(1)}%`;
const ms = (v: number) => (v >= 1000 ? `${(v / 1000).toFixed(1)}s` : `${v}ms`);

// A bar behind the number, so the reader sees the spread before reading a
// single figure. Scaled from 80% rather than 0, because every converter is
// above 80 and a bar from zero would make them look identical.
function RecallCell({ v }: { v: number }): ReactElement {
  const floor = 0.8;
  const width = Math.max(0, Math.min(1, (v - floor) / (1 - floor)));
  return (
    <div
      style={{ display: "flex", alignItems: "center", gap: ".5rem", justifyContent: "flex-end" }}
    >
      <div
        style={{
          width: 70,
          height: 6,
          background: "var(--ifm-color-emphasis-200)",
          borderRadius: 3,
          overflow: "hidden",
        }}
      >
        <div
          style={{
            width: `${100 * width}%`,
            height: "100%",
            background: v >= 0.99 ? tone.ok.fg : v >= 0.95 ? tone.warn.fg : tone.gap.fg,
          }}
        />
      </div>
      <span style={{ fontVariantNumeric: "tabular-nums", minWidth: "3.5rem", textAlign: "right" }}>
        {pct(v)}
      </span>
    </div>
  );
}

function ExtTable({ ext }: { ext: string }): ReactElement {
  const rows = r.converters
    .filter((c) => c.byExt[ext])
    .map((c) => ({ c, e: c.byExt[ext] }))
    .sort((a, b) => b.e.recall - a.e.recall);
  const words = rows[0]?.e.truthWords ?? 0;
  // Every converter in this table read the same files, so the top-file share is
  // the same for all of them; the first row's is the format's.
  const first = rows[0]?.e;
  const top =
    first?.topFile && first.topFileShare
      ? { file: first.topFile, share: first.topFileShare }
      : undefined;
  return (
    <div style={s.card}>
      <div style={{ display: "flex", gap: ".6rem", alignItems: "baseline", flexWrap: "wrap" }}>
        <strong>
          <code style={s.code}>{ext}</code>
        </strong>
        <span style={s.note}>
          {rows[0]?.e.files ?? 0} documents · {words.toLocaleString()} words of ground truth · every
          converter below read the same files. A mean well under the typical file means a few
          documents went badly wrong.
        </span>
      </div>
      {top && top.share > DOMINATION_THRESHOLD ? (
        <div style={{ ...s.banner, marginTop: ".7rem" }}>
          <code style={s.code}>{top.file}</code> carries {Math.round(100 * top.share)}% of this
          format&rsquo;s ground truth, so the words-kept column is mostly a score about that one
          document. The typical-file and mean columns weight every document equally.
        </div>
      ) : null}
      <div style={{ ...s.scroll, marginTop: ".8rem" }}>
        <table style={s.table}>
          <thead>
            <tr>
              <th style={s.th}>converter</th>
              <th style={{ ...s.th, textAlign: "right" }}>words kept</th>
              <th style={{ ...s.th, textAlign: "right" }}>typical file</th>
              <th style={{ ...s.th, textAlign: "right" }}>mean file</th>
              <th style={{ ...s.th, textAlign: "right" }}>failed</th>
              <th style={{ ...s.th, textAlign: "right" }}>median time</th>
            </tr>
          </thead>
          <tbody>
            {rows.map(({ c, e }) => (
              <tr key={c.id} style={s.row}>
                <td>
                  <strong>{c.id}</strong> <span style={s.note}>{c.version}</span>
                </td>
                <td style={s.num}>
                  <RecallCell v={e.recall} />
                </td>
                <td style={s.num}>{pct(e.medianRecall)}</td>
                <td
                  style={{
                    ...s.num,
                    // A mean far below the median is the signal a few
                    // documents went badly wrong while most were fine, which
                    // neither number says on its own.
                    color: e.medianRecall - e.meanRecall > 0.1 ? tone.gap.fg : undefined,
                  }}
                >
                  {pct(e.meanRecall)}
                </td>
                <td style={s.num}>
                  {e.failed > 0 ? <Pill text={String(e.failed)} t="gap" /> : "0"}
                </td>
                <td style={s.num}>{ms(e.medianMs)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function WorstFiles({ c }: { c: ConverterResult }): ReactElement | null {
  if (!c.worst?.length) return null;
  return (
    <details style={s.card}>
      <summary>
        <strong>{c.id}</strong>: the documents it lost the most on
      </summary>
      <p style={s.note}>{c.note}</p>
      <code style={s.code}>{c.command}</code>
      <div style={{ ...s.scroll, marginTop: ".8rem" }}>
        <table style={s.table}>
          <thead>
            <tr>
              <th style={s.th}>document</th>
              <th style={{ ...s.th, textAlign: "right" }}>kept</th>
              <th style={{ ...s.th, textAlign: "right" }}>words</th>
              <th style={s.th}>what went missing</th>
            </tr>
          </thead>
          <tbody>
            {c.worst.map((f) => (
              <tr key={f.file} style={s.row}>
                <td>
                  <code style={s.code}>{f.file}</code>
                </td>
                <td style={s.num}>{f.error ? <Pill text="failed" t="gap" /> : pct(f.recall)}</td>
                <td style={s.num}>{f.truthWords}</td>
                <td style={{ fontSize: ".82rem" }}>
                  {f.error ? (
                    <span style={{ color: tone.gap.fg }}>{f.error}</span>
                  ) : (
                    (f.missing ?? [])
                      .map((m) => (m.times > 1 ? `${m.word} ×${m.times}` : m.word))
                      .join(", ")
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </details>
  );
}

function Freshness(): ReactElement | null {
  const age = ageDays(r);
  if (age === undefined) return null;
  return (
    <Pill
      text={age === 0 ? "measured today" : `measured ${age}d ago`}
      t={age > STALE_AFTER_DAYS ? "warn" : "flat"}
    />
  );
}

export default function ConversionEval(): ReactElement {
  const exts = Object.keys(r.corpus.byExt).sort();
  return (
    <Layout
      title="Conversion eval"
      description="How much of a document's text each converter keeps, scored against the documents themselves"
    >
      <main style={s.wrap}>
        <h1 style={{ marginBottom: ".3rem" }}>Conversion</h1>
        <div style={{ display: "flex", gap: ".5rem", alignItems: "center", marginBottom: ".8rem" }}>
          <Freshness />
          <code style={s.code}>make conversion-eval</code>
        </div>
        <p style={s.lede}>
          How much of a document&rsquo;s text each converter keeps, and how long it takes. The
          answer comes from the documents rather than from any converter: OOXML designates which
          elements carry text, so each file states its own contents and no tool&rsquo;s output has
          to stand in for the truth.
        </p>

        <div style={s.banner}>
          <strong>Corpus.</strong> {r.corpus.source}{" "}
          {r.corpus.sampled ? (
            <>
              This run read {r.corpus.files.toLocaleString()} of{" "}
              {r.corpus.available.toLocaleString()} available.
            </>
          ) : (
            <>All {r.corpus.files.toLocaleString()} of them.</>
          )}
        </div>

        <h2 style={s.h2}>By format</h2>
        <p style={s.sub}>
          These are the comparable numbers. Within one extension every converter read the same
          documents, so the same ground truth is behind each row.
        </p>
        {exts.map((e) => (
          <ExtTable key={e} ext={e} />
        ))}

        <h2 style={s.h2}>Where the losses are</h2>
        <p style={s.sub}>
          A percentage with no example under it cannot be acted on. Each converter&rsquo;s worst
          documents are listed with the words that did not survive.
        </p>
        {r.converters.map((c) => (
          <WorstFiles key={c.id} c={c} />
        ))}

        <h2 style={s.h2}>Across everything each tool accepted</h2>
        <p style={s.sub}>
          Not a ranking. The tools read different file sets: textutil takes Word documents only,
          LibreOffice has no text target for Impress, pandoc does not read spreadsheets. So these
          numbers are computed over different documents and the column cannot be compared top to
          bottom.
        </p>
        <div style={{ ...s.card, ...s.scroll }}>
          <table style={s.table}>
            <thead>
              <tr>
                <th style={s.th}>converter</th>
                <th style={s.th}>reads</th>
                <th style={{ ...s.th, textAlign: "right" }}>documents</th>
                <th style={{ ...s.th, textAlign: "right" }}>words kept</th>
                <th style={{ ...s.th, textAlign: "right" }}>lost nothing</th>
                <th style={{ ...s.th, textAlign: "right" }}>failed</th>
              </tr>
            </thead>
            <tbody>
              {r.converters.map((c) => (
                <tr key={c.id} style={s.row}>
                  <td>
                    <strong>{c.id}</strong> <span style={s.note}>{c.version}</span>
                  </td>
                  <td style={s.note}>{c.exts.join(" ")}</td>
                  <td style={s.num}>{c.files}</td>
                  <td style={s.num}>
                    {pct(c.recall)} <span style={s.note}>of {c.truthWords.toLocaleString()}</span>
                  </td>
                  <td style={s.num}>
                    {c.perfect}/{c.files - c.failed}
                  </td>
                  <td style={s.num}>
                    {c.failed > 0 ? <Pill text={String(c.failed)} t="gap" /> : "0"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        <h2 style={s.h2}>How it is scored</h2>
        <div style={s.banner}>{r.method}</div>
        <div style={s.banner}>
          <strong>What this does not establish.</strong> {r.limits}
        </div>
      </main>
    </Layout>
  );
}
