import type { CSSProperties, ReactElement } from "react";
import Layout from "@theme/Layout";
import report from "./_eval.json";

// The check-eval dashboard. Measures the content checks the way parity measures
// faithfulness: a labeled corpus → precision/recall/F1 per check → a regression
// gate (scripts/checkeval + its test). Regenerate with `go run ./scripts/checkeval`.

interface Metric {
  check: string;
  cases: number;
  tp: number;
  fp: number;
  fn: number;
  precision: number;
  recall: number;
  f1: number;
}
interface CaseResult {
  id: string;
  check: string;
  expect: string[];
  got: string[];
  fp: number;
  fn: number;
  score: number;
  expect_score?: number;
  score_ok: boolean;
  note: string;
}
interface Report {
  total: Metric;
  by_check: Metric[];
  cases: CaseResult[];
}

const r = report as unknown as Report;

const cell: CSSProperties = {
  padding: "7px 12px",
  borderBottom: "1px solid var(--ifm-table-border-color)",
  textAlign: "right",
  fontVariantNumeric: "tabular-nums",
};
const left: CSSProperties = { ...cell, textAlign: "left" };
const pct = (v: number) => `${(v * 100).toFixed(0)}%`;

function verdict(c: CaseResult): { label: string; color: string } {
  if (c.fp > 0) return { label: "false positive", color: "#d65a5a" };
  if (c.fn > 0) return { label: "missed", color: "#d65a5a" };
  return { label: "ok", color: "#1f9e84" };
}

export default function CheckEval(): ReactElement {
  return (
    <Layout
      title="Check eval"
      description="Precision/recall of the content checks against a labeled corpus, gated against regressions and grown from real corrections."
    >
      <main style={{ maxWidth: 940, margin: "0 auto", padding: "2.5rem 1.25rem 4rem" }}>
        <h1>Check eval</h1>
        <p style={{ fontSize: "1.05rem", color: "var(--ifm-color-emphasis-700)" }}>
          A check is only useful if it stays accurate as it gets real usage. This measures the
          content checks the way the <a href="/parity">parity</a> harness measures format
          faithfulness: a labeled corpus of inputs with the findings a correct check should produce,
          scored as precision and recall, and gated so a change that introduces a false positive or
          a missed finding fails the build. The corpus is seeded from the checkers' unit and
          adversarial cases and grows from real corrections (issue #759).
        </p>

        <div
          style={{
            display: "flex",
            gap: 16,
            margin: "1.5rem 0",
            flexWrap: "wrap",
          }}
        >
          {[
            { k: "Cases", v: String(r.total.cases) },
            { k: "Precision", v: pct(r.total.precision) },
            { k: "Recall", v: pct(r.total.recall) },
            { k: "F1", v: r.total.f1.toFixed(2) },
            { k: "False positives", v: String(r.total.fp), bad: r.total.fp > 0 },
            { k: "Missed", v: String(r.total.fn), bad: r.total.fn > 0 },
          ].map((s) => (
            <div
              key={s.k}
              style={{
                border: "1px solid var(--ifm-color-emphasis-200)",
                borderRadius: 10,
                padding: "12px 18px",
                minWidth: 120,
              }}
            >
              <div style={{ fontSize: "0.8rem", color: "var(--ifm-color-emphasis-600)" }}>{s.k}</div>
              <div
                style={{
                  fontSize: "1.5rem",
                  fontWeight: 700,
                  color: s.bad ? "#d65a5a" : undefined,
                }}
              >
                {s.v}
              </div>
            </div>
          ))}
        </div>

        <h2>By check</h2>
        <table style={{ width: "100%", borderCollapse: "collapse", fontSize: "0.92rem" }}>
          <thead>
            <tr>
              <th style={left}>Check</th>
              <th style={cell}>Cases</th>
              <th style={cell}>Precision</th>
              <th style={cell}>Recall</th>
              <th style={cell}>F1</th>
              <th style={cell}>FP</th>
              <th style={cell}>Missed</th>
            </tr>
          </thead>
          <tbody>
            {r.by_check.map((m) => (
              <tr key={m.check}>
                <td style={left}>
                  <code>{m.check}</code>
                </td>
                <td style={cell}>{m.cases}</td>
                <td style={cell}>{pct(m.precision)}</td>
                <td style={cell}>{pct(m.recall)}</td>
                <td style={cell}>{m.f1.toFixed(2)}</td>
                <td style={{ ...cell, color: m.fp > 0 ? "#d65a5a" : undefined }}>{m.fp}</td>
                <td style={{ ...cell, color: m.fn > 0 ? "#d65a5a" : undefined }}>{m.fn}</td>
              </tr>
            ))}
          </tbody>
        </table>

        <h2>Cases</h2>
        <table style={{ width: "100%", borderCollapse: "collapse", fontSize: "0.9rem" }}>
          <thead>
            <tr>
              <th style={left}>Case</th>
              <th style={left}>Check</th>
              <th style={left}>Expected</th>
              <th style={left}>Got</th>
              <th style={cell}>Score</th>
              <th style={left}>Result</th>
            </tr>
          </thead>
          <tbody>
            {r.cases.map((c) => {
              const v = verdict(c);
              const scoreDrift = c.expect_score != null && !c.score_ok;
              return (
                <tr key={c.id}>
                  <td style={left}>
                    <code>{c.id}</code>
                  </td>
                  <td style={left}>{c.check}</td>
                  <td style={left}>{c.expect.length ? c.expect.join(", ") : "—"}</td>
                  <td style={left}>{c.got.length ? c.got.join(", ") : "—"}</td>
                  <td style={{ ...cell, color: scoreDrift ? "#d65a5a" : undefined }}>
                    {c.score}
                    {c.expect_score != null && scoreDrift ? ` (≠${c.expect_score})` : ""}
                  </td>
                  <td style={{ ...left, color: v.color, fontWeight: 600 }}>{v.label}</td>
                </tr>
              );
            })}
          </tbody>
        </table>
        <p style={{ fontSize: "0.85rem", color: "var(--ifm-color-emphasis-600)" }}>
          The <strong>Score</strong> column is the rolled-up compliance score (0–100) for the case.
          Calibrated cases pin this value, so a change to the severity weights (neutral 0 / minor 1 /
          major 5 / critical 25) or to a checker&rsquo;s severity choice is caught as score drift,
          not just a finding change (issue #758).
        </p>

        <h2>How this grows</h2>
        <p>
          The seed corpus is small and gold-labeled, so the deterministic checks score a perfect
          F1 and any regression is caught immediately. The plan (issue #759) extends it with public
          datasets (XFORMAL for formality, MQM-annotated MT, a placeholder/DNT error set) and, most
          importantly, with a real <strong>correction stream</strong>: every human correction
          is a labeled example — the check should have flagged the original and should not flag the
          fix — so the eval set, and the calibration of thresholds like <code>--voice-min</code>,
          improve precisely where real content exercises them. Calibration curves and the ML proxy
          checks are tracked next.
        </p>
      </main>
    </Layout>
  );
}
