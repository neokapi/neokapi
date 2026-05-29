import type { CSSProperties, ReactNode, ReactElement } from "react";
import Layout from "@theme/Layout";
import report from "./_benchmark.json";

// The ML-benchmark dashboard. Measures what a user actually pays to run a
// small-model checker in-process: the model download, cold-start (session
// load), per-sentence inference, and resident memory. Numbers come from
// onnxruntime — the runtime the kapi-check plugin embeds — so they transfer.
// Regenerate with `python3 scripts/ml-benchmark.py`.

interface ModelRow {
  key: string;
  repo: string;
  role: string;
  license: string;
  task: string;
  download_mb: number | null;
  onnx_available: boolean;
  load_ms?: number;
  infer_ms_mean?: number;
  peak_rss_mb?: number;
}

interface Report {
  platform: string;
  python: string;
  onnxruntime: string;
  runtime_note: string;
  models: ModelRow[];
}

const r = report as unknown as Report;

const cell: CSSProperties = {
  padding: "8px 12px",
  borderBottom: "1px solid var(--ifm-table-border-color)",
  textAlign: "right",
  fontVariantNumeric: "tabular-nums",
};
const left: CSSProperties = { ...cell, textAlign: "left" };
const num = (v: number | null | undefined, suffix = "") =>
  v == null ? "—" : `${v}${suffix}`;

// Heavier-than-comfortable for a CLI: flag big downloads / footprints.
const heavy = (mb: number | null | undefined) => (mb ?? 0) >= 300;

function Pill({ ok, children }: { ok: boolean; children: ReactNode }) {
  return (
    <span
      style={{
        background: ok ? "rgba(37,194,160,0.12)" : "rgba(244,114,114,0.12)",
        color: ok ? "#1f9e84" : "#d65a5a",
        borderRadius: 6,
        padding: "1px 8px",
        fontSize: "0.8rem",
        fontWeight: 600,
      }}
    >
      {children}
    </span>
  );
}

export default function MLBenchmark(): ReactElement {
  return (
    <Layout
      title="ML model benchmark"
      description="What it costs to run kapi's small-model content checkers in-process: download size, load time, inference latency, and memory — and what that means for running checks standalone versus server-side."
    >
      <main style={{ maxWidth: 980, margin: "0 auto", padding: "2.5rem 1.25rem 4rem" }}>
        <h1>ML model benchmark</h1>
        <p style={{ fontSize: "1.05rem", color: "var(--ifm-color-emphasis-700)" }}>
          kapi's subjective checks (voice/style similarity, register, do-not-translate by
          entity) are served by small, open, multilingual models run in-process through the
          same ONNX runtime the segmenter uses. The cost a user pays is not the per-sentence
          inference — that is cheap — but the <strong>model download</strong> and the{" "}
          <strong>resident memory</strong>. This page measures both, so the choice between
          running a check on your machine and running it server-side is grounded in numbers.
        </p>

        <h2>Measured cost per model</h2>
        <div style={{ overflowX: "auto" }}>
          <table style={{ width: "100%", borderCollapse: "collapse", fontSize: "0.92rem" }}>
            <thead>
              <tr>
                <th style={left}>Model / variant</th>
                <th style={left}>What it checks</th>
                <th style={cell}>Download</th>
                <th style={cell}>Load</th>
                <th style={cell}>Inference</th>
                <th style={cell}>Peak memory</th>
                <th style={cell}>License</th>
              </tr>
            </thead>
            <tbody>
              {r.models.map((m) => (
                <tr key={m.key}>
                  <td style={left}>
                    <code>{m.key}</code>
                    {!m.onnx_available && (
                      <>
                        {" "}
                        <Pill ok={false}>no ONNX yet</Pill>
                      </>
                    )}
                  </td>
                  <td style={left}>{m.role}</td>
                  <td style={cell}>
                    {m.download_mb == null ? (
                      "—"
                    ) : (
                      <span style={{ color: heavy(m.download_mb) ? "#d65a5a" : undefined }}>
                        {m.download_mb} MB
                      </span>
                    )}
                  </td>
                  <td style={cell}>{num(m.load_ms, " ms")}</td>
                  <td style={cell}>{m.infer_ms_mean ? `${m.infer_ms_mean} ms` : "—"}</td>
                  <td style={cell}>
                    {m.peak_rss_mb == null ? (
                      "—"
                    ) : (
                      <span style={{ color: heavy(m.peak_rss_mb) ? "#d65a5a" : undefined }}>
                        {m.peak_rss_mb} MB
                      </span>
                    )}
                  </td>
                  <td style={cell}>{m.license}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <p style={{ fontSize: "0.85rem", color: "var(--ifm-color-emphasis-600)" }}>
          {r.platform} · Python {r.python} · onnxruntime {r.onnxruntime}. Inference is the mean
          over repeated runs on short multilingual sentences. {r.runtime_note}. Size-only rows
          are not yet wired for in-process inference (GLiNER's zero-shot input differs; the
          formality ranker ships PyTorch weights that need an ONNX export). “Peak memory” is the
          resident set the loaded session adds.
        </p>

        <h2>What the numbers say</h2>
        <ul>
          <li>
            <strong>Per-sentence inference is not the cost.</strong> A loaded embedding model
            scores a sentence in single-digit milliseconds — fast enough to run on every block
            in a pipeline.
          </li>
          <li>
            <strong>The full-precision footprint is the cost.</strong> The fp32 export of a
            118M-parameter embedding model is a ~465 MB download and over a gigabyte of resident
            memory — too heavy to load casually inside a CLI that runs next to your editor and
            your build.
          </li>
          <li>
            <strong>Quantization changes the verdict.</strong> The int8 export of the same model
            is a ~129 MB download and ~40 MB resident — and slightly faster. That is small
            enough to ship as an explicitly-installed plugin and cache, which makes a single
            small-model checker viable to run on your machine.
          </li>
          <li>
            <strong>Some models stay heavy even quantized.</strong> The generalist NER model is
            ~1.1 GB at full precision and still ~330 MB int8 — defensible as an opt-in download,
            but a poor default for a laptop, and a natural fit for a server that hosts it once.
          </li>
        </ul>

        <h2>Standalone, or server-side?</h2>
        <p>
          The deterministic checks — terminology, do-not-translate by string, placeholder and
          tag integrity, register by lexicon — have no model and no download; they always run
          locally and free. The question is only where the <em>model-backed</em> checks run.
          Three options:
        </p>
        <h3>Option A — small model local, heavy models server-side (recommended)</h3>
        <p>
          Ship the int8 embedding model as an optional plugin the user explicitly installs
          (~129 MB, ~40 MB resident) for voice/style similarity and register; run the generalist NER and
          any LLM-deep check server-side in bowrain, where the model is hosted once and amortized
          across the team and across large batches. Keeps the CLI lean and offline-capable for
          the common case, without asking every user to download a gigabyte.
        </p>
        <h3>Option B — all model-backed checks server-side</h3>
        <p>
          kapi stays purely deterministic offline; every ML-backed check is a call to bowrain.
          Simplest CLI and smallest install, at the cost of the offline subjective checks and a
          network dependency for them.
        </p>
        <h3>Option C — all models local (quantized)</h3>
        <p>
          Ship int8 exports of every checker (embedding ~129 MB + NER ~330 MB + register).
          Maximal offline capability and no server needed, at the cost of a few hundred MB of
          one-time downloads and a heavier resident footprint when several run together.
        </p>
        <p>
          The data points to <strong>Option A</strong>: int8 makes one small model cheap enough
          to live in the CLI, while the heavy generalist model earns its keep server-side — which
          is also where batch volume (tens of thousands of strings across many languages) is most
          economical to process.
        </p>

        <h2>How the model is acquired</h2>
        <p>
          A checker that needs a model should acquire it <strong>explicitly</strong>, never by a
          surprise download in the middle of a <code>kapi check</code>. Consumer ML tools
          (Hugging Face <code>transformers</code>, Whisper) lazy-download on first use, which is
          convenient but hangs the first run and fails in airgapped or CI environments. Developer
          tools make it explicit and pinnable — <code>vale sync</code>, <code>spacy download</code>,
          <code>ollama pull</code> — and kapi already follows that model for its native deps
          (<code>kapi plugin install okapi-bridge</code>). The model-backed checker is the same: an
          opt-in plugin you install (its model bundled in the release tarball, the way the
          segmenter bundles the ONNX runtime, or pulled by an explicit step), so the download is a
          deliberate, cacheable, offline-after-install action with a known version.
        </p>
        <p>
          When the plugin or its model is absent, <code>kapi check</code> still runs every
          deterministic check and reports the model-backed check as unavailable with the one
          command that enables it — fail-closed with guidance, not a silent network call. In CI,
          the install is a setup step (as connector and plugin installs already are), so runs stay
          deterministic and offline once the cache is warm.
        </p>
      </main>
    </Layout>
  );
}
