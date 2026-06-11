import { useState, useMemo, useEffect } from "react";
import Layout from "@theme/Layout";
import useBaseUrl from "@docusaurus/useBaseUrl";
import styles from "./styles.module.css";

/* ── Types (new experiment-based format) ── */

interface Stats {
  mean: number;
  median: number;
  p5?: number;
  p95: number;
  stddev: number;
  min: number;
  max: number;
}

interface FileTiming {
  name: string;
  format: string;
  category: string;
  sizeBytes: number;
  startMs: number;
  endMs: number;
  wallMs: number;
  peakRssKB: number;
  userCpuMs: number;
  sysCpuMs: number;
  blockCount?: number;
  success: boolean;
  error?: string;
}

interface FileResult {
  name: string;
  format: string;
  success: boolean;
  error?: string;
  pseudoChars?: number;
  verified?: boolean;
  verifyNote?: string;
}

interface FileTrace {
  file: string;
  format: string;
  startUs: number;
  endUs: number;
  lane: number;
  durationUs: number;
}

interface BatchTrace {
  name: string;
  concurrency: number;
  fileTraces: FileTrace[];
  durationUs: number;
}

interface Experiment {
  engine: string;
  version: string;
  iterations: number;
  wallTimeMs: Stats;
  peakRssKB: Stats;
  daemonRssKB?: Stats;
  fileResults: FileResult[];
  fileTimings?: FileTiming[];
  batchTrace?: BatchTrace;
  // Verification aggregates — populated by the post-batch pass that
  // scans each output for SCRIPT_EXT_LATIN destination runes. A high
  // wallTimeMs.median paired with FilesVerified ≪ FilesSucceeded
  // means the engine is "fast" because it short-circuited pseudo,
  // not because it's efficient.
  filesAttempted?: number;
  filesSucceeded?: number;
  filesVerified?: number;
  filesUnverified?: number;
  totalPseudoChars?: number;
}

interface Metadata {
  timestamp: string;
  platform: string;
  goVersion: string;
  cpuModel: string;
  cpuCores: number;
  memoryGB: number;
}

interface Report {
  metadata: Metadata;
  experiments: Experiment[];
}

/* ── Constants ── */

const ENGINE_STYLES: Record<string, { color: string; label: string }> = {
  "kapi-native": { color: "#2563eb", label: "kapi (native)" },
  "kapi-bridge": { color: "#7c3aed", label: "kapi (bridge, subprocess)" },
  "kapi-bridge-daemon": { color: "#059669", label: "kapi (bridge, daemon)" },
  okapi: { color: "#dc2626", label: "Okapi" },
};

const FORMAT_COLORS: Record<string, string> = {
  openxml: "#4CAF50",
  html: "#2196F3",
  xliff: "#FF9800",
  xliff2: "#FB8C00",
  po: "#9C27B0",
  yaml: "#F44336",
  json: "#00BCD4",
  xml: "#795548",
  properties: "#607D8B",
  srt: "#E91E63",
  vtt: "#EC407A",
  ttml: "#AD1457",
  tmx: "#5E35B1",
  ts: "#3F51B5",
  idml: "#00897B",
  icml: "#26A69A",
  mif: "#7CB342",
  doxygen: "#FFB300",
  markdown: "#8E24AA",
  tex: "#43A047",
  plaintext: "#9E9E9E",
  paraplaintext: "#BDBDBD",
  splicedlines: "#757575",
  mosestext: "#6D4C41",
  transtable: "#558B2F",
  regex: "#37474F",
  csv: "#33691E",
  ini: "#0277BD",
  dtd: "#4527A0",
  table_filewriter: "#283593",
};

function engineLabel(engine: string): string {
  return ENGINE_STYLES[engine]?.label ?? engine;
}
function engineColor(engine: string): string {
  return ENGINE_STYLES[engine]?.color ?? "#888";
}

// Bridge filters use `okf_<name>` (okf_xliff, okf_html, …); native engines
// emit the bare name (xliff, html, …). Normalise so both share one palette.
function normalizeFormat(format: string): string {
  return format.startsWith("okf_") ? format.slice(4) : format;
}
function formatColor(format: string): string {
  return FORMAT_COLORS[normalizeFormat(format)] ?? "#888";
}
function formatLabel(format: string): string {
  return normalizeFormat(format);
}

function fmt(n: number): string {
  if (n >= 10000) return n.toLocaleString("en-US", { maximumFractionDigits: 0 });
  if (n >= 100) return n.toFixed(0);
  if (n >= 10) return n.toFixed(1);
  return n.toFixed(2);
}

function fmtBytes(bytes: number): string {
  if (bytes >= 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  if (bytes >= 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${bytes} B`;
}

/* ── Components ── */

function MetadataBar({ metadata }: { metadata: Metadata }) {
  return (
    <div className={styles.metadataBar}>
      <span>{metadata.platform}</span>
      <span>
        {metadata.cpuModel} ({metadata.cpuCores} cores)
      </span>
      <span>{metadata.memoryGB.toFixed(0)} GB RAM</span>
      <span>{metadata.goVersion}</span>
      <span>{new Date(metadata.timestamp).toLocaleDateString()}</span>
    </div>
  );
}

function Legend({ engines }: { engines: string[] }) {
  return (
    <div className={styles.legend}>
      {engines.map((e) => (
        <span key={e} className={styles.legendItem}>
          <span className={styles.legendDot} style={{ backgroundColor: engineColor(e) }} />
          {engineLabel(e)}
        </span>
      ))}
    </div>
  );
}

/** VerificationBanner flags engines whose outputs pass the existence
 * check but contain zero pseudo runes — a signal that the engine
 * short-circuited pseudo-translate (silent no-op) and its wall-time
 * numbers are misleading. Renders nothing when every engine clears
 * the threshold.
 */
function VerificationBanner({ experiments }: { experiments: Experiment[] }) {
  const flagged = experiments
    .map((e) => {
      const succeeded = e.filesSucceeded ?? e.fileResults?.length ?? 0;
      const verified = e.filesVerified ?? 0;
      const unverified = e.filesUnverified ?? 0;
      const pct = succeeded > 0 ? (verified / succeeded) * 100 : 100;
      return { engine: e.engine, succeeded, verified, unverified, pct };
    })
    .filter((r) => r.unverified > 0 && r.pct < 80);
  if (flagged.length === 0) return null;
  return (
    <div className={styles.verifyBanner}>
      <strong>⚠ Verification warning:</strong>{" "}
      {flagged.length === 1 ? "One engine" : `${flagged.length} engines`} succeeded on most files
      but produced output with zero pseudo-translated runes. The timings for these engines
      benchmark silent no-ops, not real pseudo work:
      <ul>
        {flagged.map((r) => (
          <li key={r.engine}>
            <strong>{engineLabel(r.engine)}</strong>: only {r.verified}/{r.succeeded} succeeded
            files actually contain pseudo content ({r.unverified} files written without any
            SCRIPT_EXT_LATIN runes).
          </li>
        ))}
      </ul>
    </div>
  );
}

/** engineMetric returns the ms number to use for headline ratios in the
 * given view. Batch view: the wall-time median across iterations.
 * Multi-Invocation: the summed per-file wall-time (each file = a fresh
 * kapi invocation), where the JVM cold-start cost dominates and the
 * subprocess engine ends up *slower* than okapi. Resources/files/
 * concurrency stay on batch.
 */
function engineMetric(exp: Experiment, mode: ViewMode): number {
  if (mode === "multi" && exp.fileTimings && exp.fileTimings.length > 0) {
    return exp.fileTimings.reduce((s, f) => s + (f.success ? f.wallMs : 0), 0);
  }
  return exp.wallTimeMs.median;
}

function metricLabel(mode: ViewMode): string {
  if (mode === "multi") return "vs okapi (multi-invocation)";
  return "vs okapi (batch)";
}

/** Summary cards showing headline numbers that mirror the active view —
 * so switching to Multi-Invocation rewrites the ratios from batch
 * medians to per-call totals (where bridge subprocess is slower than
 * okapi instead of faster). */
function SummaryCards({ experiments, mode }: { experiments: Experiment[]; mode: ViewMode }) {
  const okapi = experiments.find((e) => e.engine === "okapi");

  const cards: { value: string; label: string; color: string }[] = [];

  for (const exp of experiments) {
    if (exp.engine === "okapi" || !okapi) continue;
    const okapiMs = engineMetric(okapi, mode);
    const expMs = engineMetric(exp, mode);
    if (okapiMs <= 0 || expMs <= 0) continue;
    const ratio = okapiMs / expMs;
    if (ratio >= 1) {
      cards.push({
        value: `${ratio.toFixed(1)}x faster`,
        label: `${engineLabel(exp.engine)} ${metricLabel(mode)}`,
        color: engineColor(exp.engine),
      });
    } else {
      const inverse = 1 / ratio;
      cards.push({
        value: `${inverse.toFixed(2)}x slower`,
        label: `${engineLabel(exp.engine)} ${metricLabel(mode)}`,
        color: "#d97706",
      });
    }
  }

  const fileCount = experiments[0]?.fileResults?.length ?? 0;
  const allPass = experiments.every((e) => e.fileResults.every((f) => f.success));
  if (fileCount > 0) {
    cards.push({
      value: `${fileCount}`,
      label: allPass ? "files (all pass)" : "test files",
      color: allPass ? "#059669" : "#d97706",
    });
  }

  return (
    <div className={styles.summaryCards}>
      {cards.map((c, i) => (
        <div key={i} className={styles.summaryCard}>
          <div className={styles.summaryValue} style={{ color: c.color }}>
            {c.value}
          </div>
          <div className={styles.summaryLabel}>{c.label}</div>
        </div>
      ))}
    </div>
  );
}

/** Batch wall time comparison — the main benchmark view. */
function BatchComparison({ experiments }: { experiments: Experiment[] }) {
  const maxTime = Math.max(...experiments.map((e) => e.wallTimeMs.median));

  return (
    <div className={styles.compCard}>
      <div className={styles.compHeader}>
        <span className={styles.compFormat}>Batch Processing</span>
        <span className={styles.compSize}>
          {experiments[0]?.fileResults?.length ?? 0} files, {experiments[0]?.iterations ?? 0}{" "}
          iterations
        </span>
      </div>
      <div className={styles.compGrid} style={{ "--engine-count": experiments.length } as any}>
        <div className={styles.compCorner} />
        <div className={styles.compColHeader}>
          Wall Time<span className={styles.compUnit}>ms</span>
        </div>
        <div className={styles.compColHeader}>
          p95<span className={styles.compUnit}>ms</span>
        </div>
        <div className={styles.compColHeader}>
          Peak RSS<span className={styles.compUnit}>MB</span>
        </div>
        <div className={styles.compColHeader}>
          Stddev<span className={styles.compUnit}>ms</span>
        </div>
        <div
          className={styles.compColHeader}
          title="Files where the output actually contains pseudo-translated runes. An engine that completes a file but produces zero pseudo characters is silently no-op'ing — the timing for that file isn't meaningful."
        >
          Verified<span className={styles.compUnit}>files</span>
        </div>

        {experiments.map((e) => {
          const color = engineColor(e.engine);
          const timePct = maxTime > 0 ? (e.wallTimeMs.median / maxTime) * 100 : 0;
          const isTimeWin =
            e.wallTimeMs.median === Math.min(...experiments.map((x) => x.wallTimeMs.median));
          const isRssWin =
            e.peakRssKB.median === Math.min(...experiments.map((x) => x.peakRssKB.median));
          const verified = e.filesVerified ?? 0;
          const succeeded = e.filesSucceeded ?? e.fileResults?.length ?? 0;
          const unverified = e.filesUnverified ?? 0;
          const verifyPct = succeeded > 0 ? (verified / succeeded) * 100 : 100;
          // Red when fewer than 80% of succeeded files actually contain
          // pseudo runes — strong signal that timing is misleading.
          const verifyBad = succeeded > 0 && verifyPct < 80;

          return (
            <Fragment key={e.engine}>
              <div className={styles.compEngine}>
                <span className={styles.compEngineDot} style={{ backgroundColor: color }} />
                <span className={styles.compEngineName}>
                  {engineLabel(e.engine)}
                  <span className={styles.compVersion}> {e.version}</span>
                </span>
              </div>
              <div className={`${styles.compCell} ${isTimeWin ? styles.compCellWinner : ""}`}>
                <div className={styles.compBarTrack}>
                  <div
                    className={styles.compBar}
                    style={{
                      width: `${Math.max(timePct, 2)}%`,
                      backgroundColor: color,
                      opacity: isTimeWin ? 1 : 0.6,
                    }}
                  />
                </div>
                <span className={styles.compVal}>{fmt(e.wallTimeMs.median)}</span>
                {isTimeWin && !verifyBad && <span className={styles.compWinBadge} />}
              </div>
              <div className={styles.compCell}>
                <span className={styles.compVal}>{fmt(e.wallTimeMs.p95)}</span>
              </div>
              <div className={`${styles.compCell} ${isRssWin ? styles.compCellWinner : ""}`}>
                <span className={styles.compVal}>{fmt(e.peakRssKB.median / 1024)}</span>
                {isRssWin && !verifyBad && <span className={styles.compWinBadge} />}
              </div>
              <div className={styles.compCell}>
                <span className={styles.compVal}>{fmt(e.wallTimeMs.stddev)}</span>
              </div>
              <div className={styles.compCell}>
                <span
                  className={styles.compVal}
                  style={verifyBad ? { color: "#dc2626", fontWeight: 600 } : undefined}
                  title={
                    unverified > 0
                      ? `${unverified} files succeeded but produced ZERO pseudo runes — engine likely short-circuited pseudo-translate. Timing on this row is misleading.`
                      : `All ${verified} succeeded files contain pseudo content.`
                  }
                >
                  {verifyBad && "⚠ "}
                  {verified}/{succeeded}
                </span>
              </div>
            </Fragment>
          );
        })}
      </div>

      <details className={styles.compDetails}>
        <summary>Full statistics</summary>
        <div className={styles.statsTableWrap}>
          <table className={styles.statsTable}>
            <thead>
              <tr>
                <th>Engine</th>
                <th>Median</th>
                <th>Mean</th>
                <th>p95</th>
                <th>Min</th>
                <th>Max</th>
                <th>Stddev</th>
                <th>Iterations</th>
              </tr>
            </thead>
            <tbody>
              {experiments.map((e) => (
                <tr key={e.engine}>
                  <td>
                    <span
                      className={styles.compEngineDot}
                      style={{ backgroundColor: engineColor(e.engine) }}
                    />
                    {engineLabel(e.engine)}
                  </td>
                  <td className={styles.num}>{fmt(e.wallTimeMs.median)} ms</td>
                  <td className={styles.num}>{fmt(e.wallTimeMs.mean)} ms</td>
                  <td className={styles.num}>{fmt(e.wallTimeMs.p95)} ms</td>
                  <td className={styles.num}>{fmt(e.wallTimeMs.min)} ms</td>
                  <td className={styles.num}>{fmt(e.wallTimeMs.max)} ms</td>
                  <td className={styles.num}>{fmt(e.wallTimeMs.stddev)} ms</td>
                  <td className={styles.num}>{e.iterations}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </details>
    </div>
  );
}

/** Multi-invocation comparison: sums per-file wallMs to reveal daemon advantage. */
function MultiInvocationComparison({ experiments }: { experiments: Experiment[] }) {
  const withTimings = experiments.filter((e) => e.fileTimings && e.fileTimings.length > 0);
  if (withTimings.length === 0) {
    return (
      <div className={styles.compCard}>
        <div className={styles.compHeader}>
          <span className={styles.compFormat}>Multi-Invocation</span>
        </div>
        <p className={styles.empty}>
          No per-file timing data available. Re-run benchmarks to generate.
        </p>
      </div>
    );
  }

  const engineTotals = withTimings.map((e) => ({
    engine: e.engine,
    totalMs: e.fileTimings!.reduce((sum, f) => sum + f.wallMs, 0),
  }));

  const maxTotal = Math.max(...engineTotals.map((e) => e.totalMs));
  const okapi = engineTotals.find((e) => e.engine === "okapi");

  return (
    <div className={styles.compCard}>
      <div className={styles.compHeader}>
        <span className={styles.compFormat}>Multi-Invocation</span>
        <span className={styles.compSize}>
          sum of {withTimings[0].fileTimings!.length} sequential per-file invocations
        </span>
      </div>
      <p className={styles.multiDesc}>
        Simulates running each file as a separate <code>kapi</code> invocation. The bridge plugin
        runs a JVM either way; the difference is lifetime. In <strong>subprocess</strong> mode
        (kapi-bridge) <code>kapi</code> spawns a fresh JVM per call and pays the cold-start tax
        every time. In <strong>daemon</strong> mode (kapi-bridge-daemon) the JVM is already running
        as a long-lived process and <code>kapi</code> only pays the gRPC round-trip.
      </p>
      <div className={styles.compGrid} style={{ "--engine-count": engineTotals.length } as any}>
        <div className={styles.compCorner} />
        <div className={styles.compColHeader}>
          Total Time<span className={styles.compUnit}>ms</span>
        </div>
        <div className={styles.compColHeader}>vs okapi</div>
        <div className={styles.compColHeader} />
        <div className={styles.compColHeader} />

        {engineTotals.map((e) => {
          const color = engineColor(e.engine);
          const pct = maxTotal > 0 ? (e.totalMs / maxTotal) * 100 : 0;
          const isWin = e.totalMs === Math.min(...engineTotals.map((x) => x.totalMs));

          let vsLabel = "";
          let vsColor = "";
          if (okapi && e.engine !== "okapi") {
            const ratio = okapi.totalMs / e.totalMs;
            if (ratio > 1) {
              vsLabel = `${ratio.toFixed(1)}x faster`;
              vsColor = engineColor(e.engine);
            } else {
              vsLabel = `${(1 / ratio).toFixed(2)}x slower`;
              vsColor = "#d97706";
            }
          }

          return (
            <Fragment key={e.engine}>
              <div className={styles.compEngine}>
                <span className={styles.compEngineDot} style={{ backgroundColor: color }} />
                <span className={styles.compEngineName}>{engineLabel(e.engine)}</span>
              </div>
              <div className={`${styles.compCell} ${isWin ? styles.compCellWinner : ""}`}>
                <div className={styles.compBarTrack}>
                  <div
                    className={styles.compBar}
                    style={{
                      width: `${Math.max(pct, 2)}%`,
                      backgroundColor: color,
                      opacity: isWin ? 1 : 0.6,
                    }}
                  />
                </div>
                <span className={styles.compVal}>{fmt(e.totalMs)}</span>
                {isWin && <span className={styles.compWinBadge} />}
              </div>
              <div className={styles.compCell}>
                {vsLabel && (
                  <span style={{ color: vsColor, fontWeight: 600, fontSize: "0.78rem" }}>
                    {vsLabel}
                  </span>
                )}
              </div>
              <div className={styles.compCell} />
              <div className={styles.compCell} />
            </Fragment>
          );
        })}
      </div>
    </div>
  );
}

/** Timeline Gantt chart showing per-file processing as horizontal bars. */
function TimelineChart({ experiments }: { experiments: Experiment[] }) {
  const [hoveredFile, setHoveredFile] = useState<{
    engine: string;
    file: FileTiming;
    x: number;
    y: number;
  } | null>(null);
  const withTimings = experiments.filter((e) => e.fileTimings && e.fileTimings.length > 0);
  if (withTimings.length === 0) return null;

  const engines = withTimings.map((e) => e.engine);
  const maxEndMs = Math.max(...withTimings.flatMap((e) => e.fileTimings!.map((f) => f.endMs)));

  // Collect unique formats for legend
  const formats = [...new Set(withTimings[0].fileTimings!.map((f) => f.format))];

  return (
    <div className={styles.compCard}>
      <div className={styles.compHeader}>
        <span className={styles.compFormat}>Timeline</span>
        <span className={styles.compSize}>sequential trace pass</span>
      </div>

      <div className={styles.timelineLegend}>
        {formats.map((f) => (
          <span key={f} className={styles.timelineLegendItem}>
            <span
              className={styles.timelineLegendDot}
              style={{ backgroundColor: formatColor(f) }}
            />
            {f}
          </span>
        ))}
      </div>

      <div className={styles.timelineContainer}>
        {engines.map((engine) => {
          const exp = withTimings.find((e) => e.engine === engine)!;
          return (
            <div key={engine} className={styles.timelineRow}>
              <div className={styles.timelineLabel}>
                <span
                  className={styles.compEngineDot}
                  style={{ backgroundColor: engineColor(engine) }}
                />
                <span>{engineLabel(engine)}</span>
              </div>
              <div className={styles.timelineTrack}>
                {exp.fileTimings!.map((f) => {
                  const left = (f.startMs / maxEndMs) * 100;
                  const width = Math.max(((f.endMs - f.startMs) / maxEndMs) * 100, 0.3);
                  const color = formatColor(f.format);
                  return (
                    <div
                      key={f.name}
                      className={styles.timelineBar}
                      style={{ left: `${left}%`, width: `${width}%`, backgroundColor: color }}
                      onMouseEnter={(ev) =>
                        setHoveredFile({ engine, file: f, x: ev.clientX, y: ev.clientY })
                      }
                      onMouseLeave={() => setHoveredFile(null)}
                    />
                  );
                })}
              </div>
            </div>
          );
        })}

        {/* Time axis */}
        <div className={styles.timelineAxis}>
          {[0, 0.25, 0.5, 0.75, 1].map((frac) => (
            <span key={frac} className={styles.timelineTick} style={{ left: `${frac * 100}%` }}>
              {fmt(frac * maxEndMs)} ms
            </span>
          ))}
        </div>
      </div>

      {hoveredFile && (
        <div
          className={styles.timelineTooltip}
          style={{ left: hoveredFile.x + 12, top: hoveredFile.y - 10, position: "fixed" }}
        >
          <strong>{hoveredFile.file.name}</strong>
          <br />
          {hoveredFile.file.format} &middot; {fmtBytes(hoveredFile.file.sizeBytes)}
          <br />
          {fmt(hoveredFile.file.wallMs)} ms
          {hoveredFile.file.blockCount ? (
            <>
              <br />
              {hoveredFile.file.blockCount} blocks
            </>
          ) : null}
        </div>
      )}
    </div>
  );
}

/** Batch concurrency Gantt chart showing files across lanes per engine. */
function BatchConcurrencyChart({ experiments }: { experiments: Experiment[] }) {
  const [hoveredFile, setHoveredFile] = useState<{
    file: FileTrace;
    engine: string;
    x: number;
    y: number;
  } | null>(null);
  const withTraces = experiments.filter((e) => e.batchTrace && e.batchTrace.fileTraces.length > 0);
  if (withTraces.length === 0) {
    return (
      <div className={styles.compCard}>
        <div className={styles.compHeader}>
          <span className={styles.compFormat}>Batch Concurrency</span>
        </div>
        <p className={styles.empty}>
          No batch trace data available. Re-run pseudobench with <code>--trace</code> to generate.
        </p>
      </div>
    );
  }

  return (
    <>
      {withTraces.map((exp) => {
        const bt = exp.batchTrace!;
        const traces = bt.fileTraces;
        const maxTimeUs = Math.max(...traces.map((f) => f.endUs));
        const maxLane = Math.max(...traces.map((f) => f.lane));
        const numLanes = maxLane + 1;

        const formats = [...new Set(traces.map((f) => f.format))];
        const totalMs = (bt.durationUs / 1000).toFixed(0);
        const rssMB = (exp.peakRssKB.max / 1024).toFixed(1);

        // Group traces by lane
        const lanes: FileTrace[][] = Array.from({ length: numLanes }, () => []);
        for (const ft of traces) {
          lanes[ft.lane].push(ft);
        }

        return (
          <div key={exp.engine} className={styles.compCard}>
            <div className={styles.compHeader}>
              <span className={styles.compFormat}>
                Batch Concurrency — {engineLabel(exp.engine)}
              </span>
              <span className={styles.compSize}>
                {totalMs} ms · {rssMB} MB RSS · {bt.concurrency} lanes · {traces.length} files
              </span>
            </div>

            <div className={styles.timelineLegend}>
              {formats.map((f) => (
                <span key={f} className={styles.timelineLegendItem}>
                  <span
                    className={styles.timelineLegendDot}
                    style={{ backgroundColor: formatColor(f) }}
                  />
                  {formatLabel(f)}
                </span>
              ))}
            </div>

            <div className={styles.timelineContainer}>
              {lanes.map((laneTraces, laneIdx) => (
                <div key={laneIdx} className={styles.timelineRow}>
                  <div className={styles.timelineLabel}>
                    <span>Lane {laneIdx}</span>
                  </div>
                  <div className={styles.timelineTrack}>
                    {laneTraces.map((ft) => {
                      const left = (ft.startUs / maxTimeUs) * 100;
                      const width = Math.max(((ft.endUs - ft.startUs) / maxTimeUs) * 100, 0.3);
                      const color = formatColor(ft.format);
                      return (
                        <div
                          key={ft.file}
                          className={styles.timelineBar}
                          style={{ left: `${left}%`, width: `${width}%`, backgroundColor: color }}
                          onMouseEnter={(ev) =>
                            setHoveredFile({
                              file: ft,
                              engine: exp.engine,
                              x: ev.clientX,
                              y: ev.clientY,
                            })
                          }
                          onMouseLeave={() => setHoveredFile(null)}
                        />
                      );
                    })}
                  </div>
                </div>
              ))}

              {/* Time axis */}
              <div className={styles.timelineAxis}>
                {[0, 0.25, 0.5, 0.75, 1].map((frac) => (
                  <span
                    key={frac}
                    className={styles.timelineTick}
                    style={{ left: `${frac * 100}%` }}
                  >
                    {fmt((frac * maxTimeUs) / 1000)} ms
                  </span>
                ))}
              </div>
            </div>

            {hoveredFile && hoveredFile.engine === exp.engine && (
              <div
                className={styles.timelineTooltip}
                style={{ left: hoveredFile.x + 12, top: hoveredFile.y - 10, position: "fixed" }}
              >
                <strong>{hoveredFile.file.file}</strong>
                <br />
                {formatLabel(hoveredFile.file.format)} · Lane {hoveredFile.file.lane}
                <br />
                {fmt(hoveredFile.file.durationUs / 1000)} ms
              </div>
            )}
          </div>
        );
      })}
    </>
  );
}

/** Per-format CPU + memory breakdown across engines.
 *
 * Aggregates fileTimings into one row per format and shows side-by-side bars
 * for wall time, peak RSS, and CPU (user + sys) per engine. Lets you spot
 * which formats blow up memory or burn CPU on which engine — the batch-level
 * stat cards average it out, this preserves the per-format shape.
 */
function ResourcesChart({ experiments }: { experiments: Experiment[] }) {
  const withTimings = experiments.filter((e) => e.fileTimings && e.fileTimings.length > 0);
  if (withTimings.length === 0) {
    return (
      <div className={styles.compCard}>
        <div className={styles.compHeader}>
          <span className={styles.compFormat}>Resources by Format</span>
        </div>
        <p className={styles.empty}>
          No per-file timing data available. Re-run pseudobench to generate.
        </p>
      </div>
    );
  }

  // Aggregate per (engine, normalized format): sum wall, sum cpu, max RSS, count.
  type Agg = { wallMs: number; cpuMs: number; peakRssKB: number; n: number };
  const byEngine = new Map<string, Map<string, Agg>>();
  const formatsSet = new Set<string>();
  for (const exp of withTimings) {
    const formatMap = new Map<string, Agg>();
    for (const ft of exp.fileTimings!) {
      if (!ft.success) continue;
      const fmtKey = normalizeFormat(ft.format);
      formatsSet.add(fmtKey);
      const cur = formatMap.get(fmtKey) ?? { wallMs: 0, cpuMs: 0, peakRssKB: 0, n: 0 };
      cur.wallMs += ft.wallMs;
      cur.cpuMs += ft.userCpuMs + ft.sysCpuMs;
      cur.peakRssKB = Math.max(cur.peakRssKB, ft.peakRssKB);
      cur.n += 1;
      formatMap.set(fmtKey, cur);
    }
    byEngine.set(exp.engine, formatMap);
  }

  // Sort formats by aggregate wall time across engines (heaviest first).
  const formats = [...formatsSet].sort((a, b) => {
    const aw = [...byEngine.values()].reduce((s, m) => s + (m.get(a)?.wallMs ?? 0), 0);
    const bw = [...byEngine.values()].reduce((s, m) => s + (m.get(b)?.wallMs ?? 0), 0);
    return bw - aw;
  });

  // Per-metric maxima for normalising bar widths.
  let maxWall = 0;
  let maxCpu = 0;
  let maxRss = 0;
  for (const m of byEngine.values()) {
    for (const a of m.values()) {
      if (a.wallMs > maxWall) maxWall = a.wallMs;
      if (a.cpuMs > maxCpu) maxCpu = a.cpuMs;
      if (a.peakRssKB > maxRss) maxRss = a.peakRssKB;
    }
  }

  const engines = withTimings.map((e) => e.engine);

  return (
    <>
      <div className={styles.compCard}>
        <div className={styles.compHeader}>
          <span className={styles.compFormat}>Engine Memory (peak RSS, batch)</span>
          <span className={styles.compSize}>max RSS observed across the batch run</span>
        </div>
        <div className={styles.pfCardBody}>
          {experiments.map((e) => {
            const maxBatchRss = Math.max(...experiments.map((x) => x.peakRssKB.max));
            const pct = maxBatchRss > 0 ? (e.peakRssKB.max / maxBatchRss) * 100 : 0;
            return (
              <div key={e.engine} className={styles.pfRow}>
                <div className={styles.pfRowLabel}>
                  <span
                    className={styles.compEngineDot}
                    style={{ backgroundColor: engineColor(e.engine) }}
                  />
                  <span>{engineLabel(e.engine)}</span>
                </div>
                <div className={styles.pfBarWrap}>
                  <div
                    className={styles.pfBar}
                    style={{ width: `${pct}%`, backgroundColor: engineColor(e.engine) }}
                  />
                  <span className={styles.pfBarLabel}>{fmt(e.peakRssKB.max / 1024)} MB</span>
                </div>
              </div>
            );
          })}
        </div>
      </div>

      <div className={styles.compCard}>
        <div className={styles.compHeader}>
          <span className={styles.compFormat}>Per-Format Resources</span>
          <span className={styles.compSize}>
            {formats.length} formats · sorted by total wall time · sequential trace pass
          </span>
        </div>

        <div className={styles.timelineLegend}>
          {engines.map((e) => (
            <span key={e} className={styles.timelineLegendItem}>
              <span
                className={styles.timelineLegendDot}
                style={{ backgroundColor: engineColor(e) }}
              />
              {engineLabel(e)}
            </span>
          ))}
        </div>

        <div className={styles.overviewWrap}>
          <table className={styles.overviewTable}>
            <thead>
              <tr>
                <th rowSpan={2}>Format</th>
                <th rowSpan={2} className={styles.num}>
                  Files
                </th>
                <th colSpan={engines.length} className={styles.resourceGroup}>
                  Wall (ms)
                </th>
                <th colSpan={engines.length} className={styles.resourceGroup}>
                  CPU user+sys (ms)
                </th>
                <th colSpan={engines.length} className={styles.resourceGroup}>
                  Peak RSS (MB)
                </th>
              </tr>
              <tr>
                {[0, 1, 2].map((g) =>
                  engines.map((e) => (
                    <th key={`${g}-${e}`} className={styles.resourceCell}>
                      <span
                        className={styles.compEngineDot}
                        style={{ backgroundColor: engineColor(e) }}
                      />
                    </th>
                  )),
                )}
              </tr>
            </thead>
            <tbody>
              {formats.map((f) => {
                const sample = withTimings[0].fileTimings!.filter(
                  (ft) => normalizeFormat(ft.format) === f,
                );
                const fileCount = sample.length;
                return (
                  <tr key={f}>
                    <td className={styles.overviewFormat}>
                      <span
                        className={styles.timelineLegendDot}
                        style={{ backgroundColor: formatColor(f) }}
                      />
                      {f}
                    </td>
                    <td className={styles.num}>{fileCount}</td>
                    {engines.map((eng) => {
                      const a = byEngine.get(eng)?.get(f);
                      const pct = maxWall > 0 && a ? (a.wallMs / maxWall) * 100 : 0;
                      return (
                        <td key={`w-${eng}`} className={styles.resourceCell}>
                          {a ? (
                            <div className={styles.miniBarWrap}>
                              <div
                                className={styles.miniBar}
                                style={{
                                  width: `${pct}%`,
                                  backgroundColor: engineColor(eng),
                                }}
                              />
                              <span className={styles.miniBarLabel}>{fmt(a.wallMs)}</span>
                            </div>
                          ) : (
                            <span className={styles.overviewMissing}>-</span>
                          )}
                        </td>
                      );
                    })}
                    {engines.map((eng) => {
                      const a = byEngine.get(eng)?.get(f);
                      const pct = maxCpu > 0 && a ? (a.cpuMs / maxCpu) * 100 : 0;
                      return (
                        <td key={`c-${eng}`} className={styles.resourceCell}>
                          {a ? (
                            <div className={styles.miniBarWrap}>
                              <div
                                className={styles.miniBar}
                                style={{
                                  width: `${pct}%`,
                                  backgroundColor: engineColor(eng),
                                }}
                              />
                              <span className={styles.miniBarLabel}>{fmt(a.cpuMs)}</span>
                            </div>
                          ) : (
                            <span className={styles.overviewMissing}>-</span>
                          )}
                        </td>
                      );
                    })}
                    {engines.map((eng) => {
                      const a = byEngine.get(eng)?.get(f);
                      const rssMB = a ? a.peakRssKB / 1024 : 0;
                      const pct = maxRss > 0 && a ? (a.peakRssKB / maxRss) * 100 : 0;
                      return (
                        <td key={`r-${eng}`} className={styles.resourceCell}>
                          {a ? (
                            <div className={styles.miniBarWrap}>
                              <div
                                className={styles.miniBar}
                                style={{
                                  width: `${pct}%`,
                                  backgroundColor: engineColor(eng),
                                }}
                              />
                              <span className={styles.miniBarLabel}>{fmt(rssMB)}</span>
                            </div>
                          ) : (
                            <span className={styles.overviewMissing}>-</span>
                          )}
                        </td>
                      );
                    })}
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </div>
    </>
  );
}

/** Per-file timing breakdown table. */
function FileTimingsTable({ experiments }: { experiments: Experiment[] }) {
  // Collect all unique file names from the first experiment with timings
  const withTimings = experiments.filter((e) => e.fileTimings && e.fileTimings.length > 0);
  if (withTimings.length === 0) return null;

  const fileNames = withTimings[0].fileTimings!.map((f) => f.name);
  const engines = withTimings.map((e) => e.engine);

  // Check if any file has block counts
  const hasBlocks = withTimings.some((e) =>
    e.fileTimings!.some((f) => f.blockCount && f.blockCount > 0),
  );

  // Group by category for display
  const categories = [...new Set(withTimings[0].fileTimings!.map((f) => f.category))];

  // Build a map of block counts from whichever engine has them (typically native)
  const blockCounts = new Map<string, number>();
  if (hasBlocks) {
    for (const exp of withTimings) {
      for (const ft of exp.fileTimings!) {
        if (ft.blockCount && ft.blockCount > 0 && !blockCounts.has(ft.name)) {
          blockCounts.set(ft.name, ft.blockCount);
        }
      }
    }
  }

  return (
    <>
      <TimelineChart experiments={experiments} />
      <div className={styles.compCard}>
        <div className={styles.compHeader}>
          <span className={styles.compFormat}>Per-File Timing</span>
          <span className={styles.compSize}>sequential trace pass</span>
        </div>
        <div className={styles.overviewWrap}>
          <table className={styles.overviewTable}>
            <thead>
              <tr>
                <th>File</th>
                <th>Format</th>
                <th>Size</th>
                {hasBlocks && <th>Blocks</th>}
                {engines.map((e) => (
                  <th key={e}>
                    <span
                      className={styles.compEngineDot}
                      style={{ backgroundColor: engineColor(e) }}
                    />
                    {engineLabel(e)}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {categories.map((cat) => {
                const catFiles = fileNames.filter((name) => {
                  const ft = withTimings[0].fileTimings!.find((f) => f.name === name);
                  return ft?.category === cat;
                });
                return (
                  <Fragment key={cat}>
                    <tr>
                      <td
                        colSpan={3 + (hasBlocks ? 1 : 0) + engines.length}
                        className={styles.overviewFormat}
                      >
                        <strong>{cat}</strong>
                      </td>
                    </tr>
                    {catFiles.map((name) => {
                      const sample = withTimings[0].fileTimings!.find((f) => f.name === name)!;
                      const times = engines.map((eng) => {
                        const exp = withTimings.find((e) => e.engine === eng);
                        return exp?.fileTimings?.find((f) => f.name === name);
                      });
                      const minTime = Math.min(...times.filter(Boolean).map((t) => t!.wallMs));
                      const blocks = blockCounts.get(name);

                      return (
                        <tr key={name}>
                          <td className={styles.overviewFormat}>{name}</td>
                          <td>{sample.format}</td>
                          <td>{fmtBytes(sample.sizeBytes)}</td>
                          {hasBlocks && <td className={styles.num}>{blocks ?? "-"}</td>}
                          {times.map((t, i) => {
                            if (!t || !t.success) {
                              return (
                                <td key={engines[i]} className={styles.overviewMissing}>
                                  {t?.error ? "err" : "-"}
                                </td>
                              );
                            }
                            const isWin = engines.length > 1 && t.wallMs === minTime;
                            return (
                              <td
                                key={engines[i]}
                                className={`${styles.num} ${isWin ? styles.overviewWin : ""}`}
                              >
                                {fmt(t.wallMs)} ms
                              </td>
                            );
                          })}
                        </tr>
                      );
                    })}
                  </Fragment>
                );
              })}
            </tbody>
          </table>
        </div>
      </div>
    </>
  );
}

function Fragment({ children }: { children: React.ReactNode }) {
  return <>{children}</>;
}

/* ── Main Page ── */

type ViewMode = "summary" | "resources" | "files" | "concurrency" | "multi";

export default function PseudoBench() {
  const [report, setReport] = useState<Report | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [viewMode, setViewMode] = useState<ViewMode>("summary");
  const dataUrl = useBaseUrl("/data/pseudobench.json");

  useEffect(() => {
    fetch(dataUrl)
      .then((r) => {
        if (!r.ok) throw new Error(`HTTP ${r.status}`);
        return r.json();
      })
      .then((data: any) => {
        // Handle both old format (benchmarks array) and new format (experiments array).
        if (data.experiments) {
          setReport(data as Report);
        } else if (data.benchmarks) {
          // Convert old format: group benchmarks by engine into experiments.
          const byEngine = new Map<string, any[]>();
          for (const b of data.benchmarks) {
            if (!byEngine.has(b.engine)) byEngine.set(b.engine, []);
            byEngine.get(b.engine)!.push(b);
          }
          const experiments: Experiment[] = [...byEngine.entries()].map(([engine, benchmarks]) => ({
            engine,
            version: benchmarks[0]?.version ?? "",
            iterations: benchmarks[0]?.iterations ?? 0,
            wallTimeMs: { mean: 0, median: 0, p95: 0, stddev: 0, min: 0, max: 0 },
            peakRssKB: { mean: 0, median: 0, p95: 0, stddev: 0, min: 0, max: 0 },
            fileResults: benchmarks.map((b: any) => ({
              name: `${b.format}/${b.fileSize}`,
              format: b.format,
              success: true,
            })),
          }));
          setReport({ metadata: data.metadata, experiments });
        } else {
          throw new Error("Unknown data format");
        }
      })
      .catch(() =>
        setError(
          "No benchmark data found. Run PseudoBench and copy results to web/static/data/pseudobench.json",
        ),
      );
  }, [dataUrl]);

  const experiments = useMemo(() => report?.experiments ?? [], [report]);
  const engines = useMemo(() => experiments.map((e) => e.engine), [experiments]);

  return (
    <Layout title="PseudoBench" description="neokapi performance benchmarks">
      <div className={styles.container}>
        <h1>PseudoBench</h1>
        <p className={styles.subtitle}>
          Performance benchmarks: read &rarr; pseudo-translate &rarr; write across a corpus of
          real-world files
        </p>

        {error && <div className={styles.error}>{error}</div>}

        {report && experiments.length > 0 && (
          <>
            <MetadataBar metadata={report.metadata} />
            <Legend engines={engines} />
            <VerificationBanner experiments={experiments} />
            <SummaryCards experiments={experiments} mode={viewMode} />

            <div className={styles.filters}>
              <div className={styles.filterGroup}>
                <span className={styles.filterLabel}>View:</span>
                <button
                  className={`${styles.filterBtn} ${viewMode === "summary" ? styles.filterBtnActive : ""}`}
                  onClick={() => setViewMode("summary")}
                >
                  Batch Summary
                </button>
                <button
                  className={`${styles.filterBtn} ${viewMode === "resources" ? styles.filterBtnActive : ""}`}
                  onClick={() => setViewMode("resources")}
                >
                  Resources by Format
                </button>
                <button
                  className={`${styles.filterBtn} ${viewMode === "files" ? styles.filterBtnActive : ""}`}
                  onClick={() => setViewMode("files")}
                >
                  Per-File Timing
                </button>
                <button
                  className={`${styles.filterBtn} ${viewMode === "concurrency" ? styles.filterBtnActive : ""}`}
                  onClick={() => setViewMode("concurrency")}
                >
                  Concurrency
                </button>
                <button
                  className={`${styles.filterBtn} ${viewMode === "multi" ? styles.filterBtnActive : ""}`}
                  onClick={() => setViewMode("multi")}
                >
                  Multi-Invocation
                </button>
              </div>
            </div>

            {viewMode === "summary" && <BatchComparison experiments={experiments} />}
            {viewMode === "resources" && <ResourcesChart experiments={experiments} />}
            {viewMode === "files" && <FileTimingsTable experiments={experiments} />}
            {viewMode === "concurrency" && <BatchConcurrencyChart experiments={experiments} />}
            {viewMode === "multi" && <MultiInvocationComparison experiments={experiments} />}
          </>
        )}

        {!report && !error && <p>Loading benchmark data...</p>}
      </div>
    </Layout>
  );
}
