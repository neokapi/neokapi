import { useState, useMemo, useEffect } from "react";
import Layout from "@theme/Layout";
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
  "kapi-bridge": { color: "#7c3aed", label: "kapi (bridge)" },
  "kapi-bridge-daemon": { color: "#059669", label: "kapi (bridge daemon)" },
  okapi: { color: "#dc2626", label: "Okapi (tikal)" },
};

const FORMAT_COLORS: Record<string, string> = {
  openxml: "#4CAF50",
  html: "#2196F3",
  xliff: "#FF9800",
  po: "#9C27B0",
  yaml: "#F44336",
  json: "#00BCD4",
  xml: "#795548",
  properties: "#607D8B",
  srt: "#E91E63",
};

function engineLabel(engine: string): string {
  return ENGINE_STYLES[engine]?.label ?? engine;
}
function engineColor(engine: string): string {
  return ENGINE_STYLES[engine]?.color ?? "#888";
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

/** Summary cards showing headline numbers with correct faster/slower labels. */
function SummaryCards({ experiments }: { experiments: Experiment[] }) {
  const okapi = experiments.find((e) => e.engine === "okapi");

  const cards: { value: string; label: string; color: string }[] = [];

  for (const exp of experiments) {
    if (exp.engine === "okapi" || !okapi) continue;
    const ratio = okapi.wallTimeMs.median / exp.wallTimeMs.median;
    if (ratio > 1) {
      cards.push({
        value: `${ratio.toFixed(1)}x faster`,
        label: `${engineLabel(exp.engine)} vs tikal`,
        color: engineColor(exp.engine),
      });
    } else {
      const inverse = 1 / ratio;
      cards.push({
        value: `${inverse.toFixed(2)}x slower`,
        label: `${engineLabel(exp.engine)} vs tikal`,
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

        {experiments.map((e) => {
          const color = engineColor(e.engine);
          const timePct = maxTime > 0 ? (e.wallTimeMs.median / maxTime) * 100 : 0;
          const isTimeWin =
            e.wallTimeMs.median === Math.min(...experiments.map((x) => x.wallTimeMs.median));
          const isRssWin =
            e.peakRssKB.median === Math.min(...experiments.map((x) => x.peakRssKB.median));

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
                {isTimeWin && <span className={styles.compWinBadge} />}
              </div>
              <div className={styles.compCell}>
                <span className={styles.compVal}>{fmt(e.wallTimeMs.p95)}</span>
              </div>
              <div className={`${styles.compCell} ${isRssWin ? styles.compCellWinner : ""}`}>
                <span className={styles.compVal}>{fmt(e.peakRssKB.median / 1024)}</span>
                {isRssWin && <span className={styles.compWinBadge} />}
              </div>
              <div className={styles.compCell}>
                <span className={styles.compVal}>{fmt(e.wallTimeMs.stddev)}</span>
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
        Simulates running each file as a separate <code>kapi</code> invocation. Bridge pays JVM
        startup per call; daemon pays only gRPC round-trip.
      </p>
      <div className={styles.compGrid} style={{ "--engine-count": engineTotals.length } as any}>
        <div className={styles.compCorner} />
        <div className={styles.compColHeader}>
          Total Time<span className={styles.compUnit}>ms</span>
        </div>
        <div className={styles.compColHeader}>vs tikal</div>
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
              style={{ backgroundColor: FORMAT_COLORS[f] ?? "#888" }}
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
                  const color = FORMAT_COLORS[f.format] ?? "#888";
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
                    style={{ backgroundColor: FORMAT_COLORS[f] ?? "#888" }}
                  />
                  {f}
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
                      const color = FORMAT_COLORS[ft.format] ?? "#888";
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
                {hoveredFile.file.format} · Lane {hoveredFile.file.lane}
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

type ViewMode = "summary" | "files" | "concurrency" | "multi";

export default function PseudoBench() {
  const [report, setReport] = useState<Report | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [viewMode, setViewMode] = useState<ViewMode>("summary");

  useEffect(() => {
    fetch("/data/pseudobench.json")
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
          "No benchmark data found. Run PseudoBench and copy results to web/docs/static/data/pseudobench.json",
        ),
      );
  }, []);

  const experiments = useMemo(() => report?.experiments ?? [], [report]);
  const engines = useMemo(() => experiments.map((e) => e.engine), [experiments]);

  return (
    <Layout title="PseudoBench" description="neokapi performance benchmarks">
      <div className={styles.container}>
        <h1>PseudoBench</h1>
        <p className={styles.subtitle}>
          Performance benchmarks: read &rarr; pseudo-translate &rarr; write across 21 real-world
          files
        </p>

        {error && <div className={styles.error}>{error}</div>}

        {report && experiments.length > 0 && (
          <>
            <MetadataBar metadata={report.metadata} />
            <Legend engines={engines} />
            <SummaryCards experiments={experiments} />

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
