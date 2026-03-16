import {useState, useMemo, useEffect} from 'react';
import Layout from '@theme/Layout';
import styles from './styles.module.css';

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
  success: boolean;
  error?: string;
}

interface FileResult {
  name: string;
  format: string;
  success: boolean;
  error?: string;
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

const ENGINE_STYLES: Record<string, {color: string; label: string}> = {
  'kapi-native': {color: '#2563eb', label: 'kapi (native)'},
  'kapi-bridge': {color: '#7c3aed', label: 'kapi (bridge)'},
  'kapi-bridge-daemon': {color: '#059669', label: 'kapi (bridge daemon)'},
  'okapi': {color: '#dc2626', label: 'Okapi (tikal)'},
};

function engineLabel(engine: string): string {
  return ENGINE_STYLES[engine]?.label ?? engine;
}
function engineColor(engine: string): string {
  return ENGINE_STYLES[engine]?.color ?? '#888';
}

function fmt(n: number): string {
  if (n >= 10000) return n.toLocaleString('en-US', {maximumFractionDigits: 0});
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

function MetadataBar({metadata}: {metadata: Metadata}) {
  return (
    <div className={styles.metadataBar}>
      <span>{metadata.platform}</span>
      <span>{metadata.cpuModel} ({metadata.cpuCores} cores)</span>
      <span>{metadata.memoryGB.toFixed(0)} GB RAM</span>
      <span>{metadata.goVersion}</span>
      <span>{new Date(metadata.timestamp).toLocaleDateString()}</span>
    </div>
  );
}

function Legend({engines}: {engines: string[]}) {
  return (
    <div className={styles.legend}>
      {engines.map((e) => (
        <span key={e} className={styles.legendItem}>
          <span className={styles.legendDot} style={{backgroundColor: engineColor(e)}} />
          {engineLabel(e)}
        </span>
      ))}
    </div>
  );
}

/** Summary cards showing headline numbers. */
function SummaryCards({experiments}: {experiments: Experiment[]}) {
  const native = experiments.find((e) => e.engine === 'kapi-native');
  const bridge = experiments.find((e) => e.engine === 'kapi-bridge');
  const daemon = experiments.find((e) => e.engine === 'kapi-bridge-daemon');
  const okapi = experiments.find((e) => e.engine === 'okapi');

  const cards: {value: string; label: string; color: string}[] = [];

  if (native && okapi) {
    const ratio = okapi.wallTimeMs.median / native.wallTimeMs.median;
    cards.push({value: `${ratio.toFixed(1)}x`, label: 'kapi native vs tikal', color: '#2563eb'});
  }
  if (bridge && okapi) {
    const ratio = bridge.wallTimeMs.median / okapi.wallTimeMs.median;
    cards.push({value: `${ratio.toFixed(2)}x`, label: 'bridge vs tikal', color: '#7c3aed'});
  }
  if (daemon && okapi) {
    const ratio = daemon.wallTimeMs.median / okapi.wallTimeMs.median;
    cards.push({value: `${ratio.toFixed(2)}x`, label: 'daemon vs tikal', color: '#059669'});
  }

  const fileCount = experiments[0]?.fileResults?.length ?? 0;
  const allPass = experiments.every((e) => e.fileResults.every((f) => f.success));
  if (fileCount > 0) {
    cards.push({
      value: `${fileCount}`,
      label: allPass ? 'files (all pass)' : 'test files',
      color: allPass ? '#059669' : '#d97706',
    });
  }

  return (
    <div className={styles.summaryCards}>
      {cards.map((c, i) => (
        <div key={i} className={styles.summaryCard}>
          <div className={styles.summaryValue} style={{color: c.color}}>{c.value}</div>
          <div className={styles.summaryLabel}>{c.label}</div>
        </div>
      ))}
    </div>
  );
}

/** Batch wall time comparison — the main benchmark view. */
function BatchComparison({experiments}: {experiments: Experiment[]}) {
  const maxTime = Math.max(...experiments.map((e) => e.wallTimeMs.median));
  const maxRss = Math.max(...experiments.map((e) => e.peakRssKB.median));

  return (
    <div className={styles.compCard}>
      <div className={styles.compHeader}>
        <span className={styles.compFormat}>Batch Processing</span>
        <span className={styles.compSize}>
          {experiments[0]?.fileResults?.length ?? 0} files, {experiments[0]?.iterations ?? 0} iterations
        </span>
      </div>
      <div className={styles.compGrid} style={{'--engine-count': experiments.length} as any}>
        <div className={styles.compCorner} />
        <div className={styles.compColHeader}>Wall Time<span className={styles.compUnit}>ms</span></div>
        <div className={styles.compColHeader}>p95<span className={styles.compUnit}>ms</span></div>
        <div className={styles.compColHeader}>Peak RSS<span className={styles.compUnit}>MB</span></div>
        <div className={styles.compColHeader}>Stddev<span className={styles.compUnit}>ms</span></div>

        {experiments.map((e) => {
          const color = engineColor(e.engine);
          const timePct = maxTime > 0 ? (e.wallTimeMs.median / maxTime) * 100 : 0;
          const isTimeWin = e.wallTimeMs.median === Math.min(...experiments.map((x) => x.wallTimeMs.median));
          const isRssWin = e.peakRssKB.median === Math.min(...experiments.map((x) => x.peakRssKB.median));

          return (
            <Fragment key={e.engine}>
              <div className={styles.compEngine}>
                <span className={styles.compEngineDot} style={{backgroundColor: color}} />
                <span className={styles.compEngineName}>
                  {engineLabel(e.engine)}
                  <span className={styles.compVersion}> {e.version}</span>
                </span>
              </div>
              <div className={`${styles.compCell} ${isTimeWin ? styles.compCellWinner : ''}`}>
                <div className={styles.compBarTrack}>
                  <div className={styles.compBar} style={{width: `${Math.max(timePct, 2)}%`, backgroundColor: color, opacity: isTimeWin ? 1 : 0.6}} />
                </div>
                <span className={styles.compVal}>{fmt(e.wallTimeMs.median)}</span>
                {isTimeWin && <span className={styles.compWinBadge} />}
              </div>
              <div className={styles.compCell}>
                <span className={styles.compVal}>{fmt(e.wallTimeMs.p95)}</span>
              </div>
              <div className={`${styles.compCell} ${isRssWin ? styles.compCellWinner : ''}`}>
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
                    <span className={styles.compEngineDot} style={{backgroundColor: engineColor(e.engine)}} />
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

/** Per-file timing breakdown table. */
function FileTimingsTable({experiments}: {experiments: Experiment[]}) {
  // Collect all unique file names from the first experiment with timings
  const withTimings = experiments.filter((e) => e.fileTimings && e.fileTimings.length > 0);
  if (withTimings.length === 0) return null;

  const fileNames = withTimings[0].fileTimings!.map((f) => f.name);
  const engines = withTimings.map((e) => e.engine);

  // Group by category for display
  const categories = [...new Set(withTimings[0].fileTimings!.map((f) => f.category))];

  return (
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
              {engines.map((e) => (
                <th key={e}>
                  <span className={styles.compEngineDot} style={{backgroundColor: engineColor(e)}} />
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
                    <td colSpan={3 + engines.length} className={styles.overviewFormat}>
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

                    return (
                      <tr key={name}>
                        <td className={styles.overviewFormat}>{name}</td>
                        <td>{sample.format}</td>
                        <td>{fmtBytes(sample.sizeBytes)}</td>
                        {times.map((t, i) => {
                          if (!t || !t.success) {
                            return <td key={engines[i]} className={styles.overviewMissing}>{t?.error ? 'err' : '-'}</td>;
                          }
                          const isWin = engines.length > 1 && t.wallMs === minTime;
                          return (
                            <td key={engines[i]} className={`${styles.num} ${isWin ? styles.overviewWin : ''}`}>
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
  );
}

function Fragment({children}: {children: React.ReactNode}) {
  return <>{children}</>;
}

/* ── Main Page ── */

type ViewMode = 'summary' | 'files';

export default function PseudoBench() {
  const [report, setReport] = useState<Report | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [viewMode, setViewMode] = useState<ViewMode>('summary');

  useEffect(() => {
    fetch('/data/pseudobench.json')
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
            version: benchmarks[0]?.version ?? '',
            iterations: benchmarks[0]?.iterations ?? 0,
            wallTimeMs: {mean: 0, median: 0, p95: 0, stddev: 0, min: 0, max: 0},
            peakRssKB: {mean: 0, median: 0, p95: 0, stddev: 0, min: 0, max: 0},
            fileResults: benchmarks.map((b: any) => ({name: `${b.format}/${b.fileSize}`, format: b.format, success: true})),
          }));
          setReport({metadata: data.metadata, experiments});
        } else {
          throw new Error('Unknown data format');
        }
      })
      .catch(() =>
        setError('No benchmark data found. Run PseudoBench and copy results to website/static/data/pseudobench.json'),
      );
  }, []);

  const experiments = useMemo(() => report?.experiments ?? [], [report]);
  const engines = useMemo(() => experiments.map((e) => e.engine), [experiments]);

  return (
    <Layout title="PseudoBench" description="neokapi performance benchmarks">
      <div className={styles.container}>
        <h1>PseudoBench</h1>
        <p className={styles.subtitle}>
          Performance benchmarks: read &rarr; pseudo-translate &rarr; write across 21 real-world files
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
                  className={`${styles.filterBtn} ${viewMode === 'summary' ? styles.filterBtnActive : ''}`}
                  onClick={() => setViewMode('summary')}
                >
                  Batch Summary
                </button>
                <button
                  className={`${styles.filterBtn} ${viewMode === 'files' ? styles.filterBtnActive : ''}`}
                  onClick={() => setViewMode('files')}
                >
                  Per-File Timing
                </button>
              </div>
            </div>

            {viewMode === 'summary' && <BatchComparison experiments={experiments} />}
            {viewMode === 'files' && <FileTimingsTable experiments={experiments} />}
          </>
        )}

        {!report && !error && <p>Loading benchmark data...</p>}
      </div>
    </Layout>
  );
}
