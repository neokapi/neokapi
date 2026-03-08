import {useState, useMemo, useCallback, useEffect} from 'react';
import Layout from '@theme/Layout';
import styles from './styles.module.css';

/* ── Types ── */

interface Stats {
  mean: number;
  median: number;
  p95: number;
  stddev: number;
  min: number;
  max: number;
}

interface BenchMetrics {
  wallTimeMs: Stats;
  userCpuMs: Stats;
  sysCpuMs: Stats;
  peakRssKB: Stats;
  outputBytes: Stats;
}

interface BenchmarkResult {
  category: string;
  engine: string;
  format: string;
  fileSize: string;
  fileSizeBytes: number;
  fileCount: number;
  totalInputBytes: number;
  unitCount: number;
  version: string;
  iterations: number;
  metrics: BenchMetrics;
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
  benchmarks: BenchmarkResult[];
}

/* ── Constants ── */

type MetricKey = 'wallTimeMs' | 'userCpuMs' | 'sysCpuMs' | 'peakRssKB';

const ALL_METRICS: {key: MetricKey; label: string; unit: string}[] = [
  {key: 'wallTimeMs', label: 'Wall Time', unit: 'ms'},
  {key: 'userCpuMs', label: 'User CPU', unit: 'ms'},
  {key: 'sysCpuMs', label: 'Sys CPU', unit: 'ms'},
  {key: 'peakRssKB', label: 'Peak RSS', unit: 'KB'},
];

const ENGINE_STYLES: Record<string, {color: string; label: string}> = {
  'kapi-native': {color: '#2563eb', label: 'Kapi (Native)'},
  'kapi-bridge': {color: '#7c3aed', label: 'Kapi (Bridge)'},
  'okapi': {color: '#dc2626', label: 'Okapi (Tikal)'},
};

const CATEGORY_LABELS: Record<string, string> = {
  single: 'Single File',
  collection: 'File Collection',
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

/* ── Speedup helper ── */

interface SpeedupPair {
  format: string;
  size: string;
  metric: MetricKey;
  fastest: string;
  slowest: string;
  ratio: number;
}

function computeSpeedups(benchmarks: BenchmarkResult[]): SpeedupPair[] {
  const grouped = new Map<string, BenchmarkResult[]>();
  for (const b of benchmarks) {
    const key = `${b.category}/${b.format}/${b.fileSize}`;
    if (!grouped.has(key)) grouped.set(key, []);
    grouped.get(key)!.push(b);
  }

  const pairs: SpeedupPair[] = [];
  for (const [key, group] of grouped) {
    if (group.length < 2) continue;
    const [, format, size] = key.split('/');
    for (const m of ALL_METRICS) {
      const sorted = [...group].sort(
        (a, b) => a.metrics[m.key].median - b.metrics[m.key].median,
      );
      const fastest = sorted[0];
      const slowest = sorted[sorted.length - 1];
      if (fastest.metrics[m.key].median > 0) {
        pairs.push({
          format,
          size,
          metric: m.key,
          fastest: fastest.engine,
          slowest: slowest.engine,
          ratio: slowest.metrics[m.key].median / fastest.metrics[m.key].median,
        });
      }
    }
  }
  return pairs;
}

/* ── Components ── */

function MetadataBar({metadata}: {metadata: Metadata}) {
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

function Legend({engines}: {engines: string[]}) {
  return (
    <div className={styles.legend}>
      {engines.map((e) => (
        <span key={e} className={styles.legendItem}>
          <span
            className={styles.legendDot}
            style={{backgroundColor: engineColor(e)}}
          />
          {engineLabel(e)}
        </span>
      ))}
    </div>
  );
}

function SpeedupSummary({pairs}: {pairs: SpeedupPair[]}) {
  if (pairs.length === 0) return null;

  const timePairs = pairs.filter((p) => p.metric === 'wallTimeMs');
  if (timePairs.length === 0) return null;

  const avgRatio = timePairs.reduce((s, p) => s + p.ratio, 0) / timePairs.length;
  const maxRatio = Math.max(...timePairs.map((p) => p.ratio));

  const rssPairs = pairs.filter((p) => p.metric === 'peakRssKB');
  const avgRssRatio =
    rssPairs.length > 0
      ? rssPairs.reduce((s, p) => s + p.ratio, 0) / rssPairs.length
      : 0;

  const cpuPairs = pairs.filter((p) => p.metric === 'userCpuMs');
  const avgCpuRatio =
    cpuPairs.length > 0
      ? cpuPairs.reduce((s, p) => s + p.ratio, 0) / cpuPairs.length
      : 0;

  return (
    <div className={styles.summaryCards}>
      <div className={styles.summaryCard}>
        <div className={styles.summaryValue}>{avgRatio.toFixed(1)}x</div>
        <div className={styles.summaryLabel}>Avg wall time spread</div>
      </div>
      <div className={styles.summaryCard}>
        <div className={styles.summaryValue}>{maxRatio.toFixed(1)}x</div>
        <div className={styles.summaryLabel}>Max wall time spread</div>
      </div>
      {avgCpuRatio > 0 && (
        <div className={styles.summaryCard}>
          <div className={styles.summaryValue}>{avgCpuRatio.toFixed(1)}x</div>
          <div className={styles.summaryLabel}>Avg CPU spread</div>
        </div>
      )}
      {avgRssRatio > 0 && (
        <div className={styles.summaryCard}>
          <div className={styles.summaryValue}>{avgRssRatio.toFixed(1)}x</div>
          <div className={styles.summaryLabel}>Avg memory spread</div>
        </div>
      )}
    </div>
  );
}

/**
 * The core comparison component.
 * For one format+size, shows all engines as rows and all metrics as columns.
 */
function ComparisonCard({
  format,
  fileSize,
  data,
  metricMaxes,
  isCollection,
}: {
  format: string;
  fileSize: string;
  data: BenchmarkResult[];
  metricMaxes: Record<MetricKey, number>;
  isCollection?: boolean;
}) {
  const winners: Record<MetricKey, string> = {} as any;
  for (const m of ALL_METRICS) {
    let best = Infinity;
    let bestEngine = '';
    for (const b of data) {
      const v = b.metrics[m.key].median;
      if (v < best) {
        best = v;
        bestEngine = b.engine;
      }
    }
    winners[m.key] = bestEngine;
  }

  const hasMultipleEngines = data.length > 1;
  const sample = data[0];

  return (
    <div className={styles.compCard}>
      <div className={styles.compHeader}>
        <span className={styles.compFormat}>{format}</span>
        <span className={styles.compSize}>{fileSize}</span>
        {isCollection && sample && (
          <>
            <span className={styles.compSize}>{sample.fileCount} files</span>
            <span className={styles.compSize}>{sample.unitCount} units</span>
            <span className={styles.compSize}>{fmtBytes(sample.totalInputBytes)}</span>
          </>
        )}
        {!isCollection && sample && (
          <>
            <span className={styles.compSize}>{sample.unitCount} units</span>
            <span className={styles.compSize}>{fmtBytes(sample.fileSizeBytes || sample.totalInputBytes)}</span>
          </>
        )}
      </div>

      <div className={styles.compGrid} style={{'--engine-count': data.length} as any}>
        <div className={styles.compCorner} />
        {ALL_METRICS.map((m) => (
          <div key={m.key} className={styles.compColHeader}>
            {m.label}
            <span className={styles.compUnit}>{m.unit}</span>
          </div>
        ))}

        {data.map((b) => (
          <ComparisonRow
            key={b.engine + b.version}
            result={b}
            winners={winners}
            metricMaxes={metricMaxes}
            hasMultipleEngines={hasMultipleEngines}
          />
        ))}
      </div>

      <details className={styles.compDetails}>
        <summary>Full statistics</summary>
        <FullStatsTable data={data} />
      </details>
    </div>
  );
}

function ComparisonRow({
  result: b,
  winners,
  metricMaxes,
  hasMultipleEngines,
}: {
  result: BenchmarkResult;
  winners: Record<MetricKey, string>;
  metricMaxes: Record<MetricKey, number>;
  hasMultipleEngines: boolean;
}) {
  const color = engineColor(b.engine);
  const version = b.version ? ` ${b.version}` : '';

  return (
    <>
      <div className={styles.compEngine}>
        <span className={styles.compEngineDot} style={{backgroundColor: color}} />
        <span className={styles.compEngineName}>
          {engineLabel(b.engine)}
          <span className={styles.compVersion}>{version}</span>
        </span>
      </div>

      {ALL_METRICS.map((m) => {
        const val = b.metrics[m.key].median;
        const max = metricMaxes[m.key];
        const pct = max > 0 ? (val / max) * 100 : 0;
        const isWinner = hasMultipleEngines && winners[m.key] === b.engine;
        const p95 = b.metrics[m.key].p95;

        return (
          <div
            key={m.key}
            className={`${styles.compCell} ${isWinner ? styles.compCellWinner : ''}`}
            title={`median: ${fmt(val)} | p95: ${fmt(p95)} | stddev: ${fmt(b.metrics[m.key].stddev)}`}
          >
            <div className={styles.compBarTrack}>
              <div
                className={styles.compBar}
                style={{
                  width: `${Math.max(pct, 2)}%`,
                  backgroundColor: color,
                  opacity: isWinner ? 1 : 0.6,
                }}
              />
            </div>
            <span className={styles.compVal}>{fmt(val)}</span>
            {isWinner && <span className={styles.compWinBadge} />}
          </div>
        );
      })}
    </>
  );
}

function FullStatsTable({data}: {data: BenchmarkResult[]}) {
  return (
    <div className={styles.statsTableWrap}>
      {ALL_METRICS.map((m) => (
        <div key={m.key} className={styles.statsSection}>
          <div className={styles.statsMetricLabel}>
            {m.label} ({m.unit})
          </div>
          <table className={styles.statsTable}>
            <thead>
              <tr>
                <th>Engine</th>
                <th>Median</th>
                <th>Mean</th>
                <th>P95</th>
                <th>Min</th>
                <th>Max</th>
                <th>Stddev</th>
              </tr>
            </thead>
            <tbody>
              {data.map((b) => {
                const s = b.metrics[m.key];
                return (
                  <tr key={b.engine}>
                    <td>
                      <span
                        className={styles.compEngineDot}
                        style={{backgroundColor: engineColor(b.engine)}}
                      />
                      {engineLabel(b.engine)}
                    </td>
                    <td className={styles.num}>{fmt(s.median)}</td>
                    <td className={styles.num}>{fmt(s.mean)}</td>
                    <td className={styles.num}>{fmt(s.p95)}</td>
                    <td className={styles.num}>{fmt(s.min)}</td>
                    <td className={styles.num}>{fmt(s.max)}</td>
                    <td className={styles.num}>{fmt(s.stddev)}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      ))}
    </div>
  );
}

/** Overview table for quick scanning across all format+size combos. */
function OverviewTable({
  groups,
  engines,
  isCollection,
}: {
  groups: Map<string, BenchmarkResult[]>;
  engines: string[];
  isCollection?: boolean;
}) {
  return (
    <div className={styles.overviewWrap}>
      <table className={styles.overviewTable}>
        <thead>
          <tr>
            <th>{isCollection ? 'Collection' : 'Format'}</th>
            <th>Size</th>
            {isCollection && <th>Files</th>}
            {engines.map((e) => (
              <th key={e} colSpan={2}>
                <span
                  className={styles.compEngineDot}
                  style={{backgroundColor: engineColor(e)}}
                />
                {engineLabel(e)}
              </th>
            ))}
          </tr>
          <tr>
            <th />
            <th />
            {isCollection && <th />}
            {engines.map((e) => (
              <Fragment key={e}>
                <th className={styles.overviewSubHeader}>Time</th>
                <th className={styles.overviewSubHeader}>RSS</th>
              </Fragment>
            ))}
          </tr>
        </thead>
        <tbody>
          {[...groups.entries()].map(([key, data]) => {
            const [format, fileSize] = key.split('/');
            let minTime = Infinity;
            let minRss = Infinity;
            for (const b of data) {
              minTime = Math.min(minTime, b.metrics.wallTimeMs.median);
              minRss = Math.min(minRss, b.metrics.peakRssKB.median);
            }
            const sample = data[0];
            return (
              <tr key={key}>
                <td className={styles.overviewFormat}>{format}</td>
                <td className={styles.overviewSize}>{fileSize}</td>
                {isCollection && (
                  <td className={styles.overviewSize}>
                    {sample?.fileCount ?? '-'}
                  </td>
                )}
                {engines.map((eng) => {
                  const b = data.find((d) => d.engine === eng);
                  if (!b)
                    return (
                      <Fragment key={eng}>
                        <td className={styles.overviewMissing}>-</td>
                        <td className={styles.overviewMissing}>-</td>
                      </Fragment>
                    );
                  const time = b.metrics.wallTimeMs.median;
                  const rss = b.metrics.peakRssKB.median;
                  const isTimeWin = data.length > 1 && time === minTime;
                  const isRssWin = data.length > 1 && rss === minRss;
                  return (
                    <Fragment key={eng}>
                      <td
                        className={`${styles.num} ${isTimeWin ? styles.overviewWin : ''}`}
                      >
                        {fmt(time)} ms
                      </td>
                      <td
                        className={`${styles.num} ${isRssWin ? styles.overviewWin : ''}`}
                      >
                        {fmt(rss / 1024)} MB
                      </td>
                    </Fragment>
                  );
                })}
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

function Fragment({children}: {children: React.ReactNode}) {
  return <>{children}</>;
}

/* ── Main Page ── */

type ViewMode = 'cards' | 'table';

export default function PseudoBench() {
  const [report, setReport] = useState<Report | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [selectedCategory, setSelectedCategory] = useState<string>('single');
  const [selectedFormat, setSelectedFormat] = useState<string>('');
  const [selectedSize, setSelectedSize] = useState<string>('');
  const [viewMode, setViewMode] = useState<ViewMode>('cards');

  useEffect(() => {
    fetch('/data/pseudobench.json')
      .then((r) => {
        if (!r.ok) throw new Error(`HTTP ${r.status}`);
        return r.json();
      })
      .then((data: Report) => setReport(data))
      .catch(() =>
        setError(
          'No benchmark data found. Run PseudoBench and copy results to website/static/data/pseudobench.json',
        ),
      );
  }, []);

  const categories = useMemo(
    () => (report ? [...new Set(report.benchmarks.map((b) => b.category))] : []),
    [report],
  );

  const catBenchmarks = useMemo(
    () =>
      report
        ? report.benchmarks.filter((b) => b.category === selectedCategory)
        : [],
    [report, selectedCategory],
  );

  const engines = useMemo(
    () => [...new Set(catBenchmarks.map((b) => b.engine))],
    [catBenchmarks],
  );

  const formats = useMemo(
    () => [...new Set(catBenchmarks.map((b) => b.format))],
    [catBenchmarks],
  );

  const sizes = useMemo(
    () => [...new Set(catBenchmarks.map((b) => b.fileSize))],
    [catBenchmarks],
  );

  const filtered = useMemo(() => {
    return catBenchmarks.filter(
      (b) =>
        (!selectedFormat || b.format === selectedFormat) &&
        (!selectedSize || b.fileSize === selectedSize),
    );
  }, [catBenchmarks, selectedFormat, selectedSize]);

  const groups = useMemo(() => {
    const map = new Map<string, BenchmarkResult[]>();
    for (const b of filtered) {
      const key = `${b.format}/${b.fileSize}`;
      if (!map.has(key)) map.set(key, []);
      map.get(key)!.push(b);
    }
    return map;
  }, [filtered]);

  const metricMaxes = useMemo(() => {
    const maxes: Record<MetricKey, number> = {
      wallTimeMs: 0,
      userCpuMs: 0,
      sysCpuMs: 0,
      peakRssKB: 0,
    };
    for (const b of filtered) {
      for (const m of ALL_METRICS) {
        maxes[m.key] = Math.max(maxes[m.key], b.metrics[m.key].median);
      }
    }
    return maxes;
  }, [filtered]);

  const speedups = useMemo(
    () => (report ? computeSpeedups(catBenchmarks) : []),
    [report, catBenchmarks],
  );

  const toggleFormat = useCallback(
    (f: string) => setSelectedFormat((prev) => (prev === f ? '' : f)),
    [],
  );
  const toggleSize = useCallback(
    (s: string) => setSelectedSize((prev) => (prev === s ? '' : s)),
    [],
  );

  const isCollection = selectedCategory === 'collection';

  // Reset format filter when switching categories.
  const switchCategory = useCallback((cat: string) => {
    setSelectedCategory(cat);
    setSelectedFormat('');
    setSelectedSize('');
  }, []);

  return (
    <Layout title="PseudoBench" description="gokapi performance benchmarks">
      <div className={styles.container}>
        <h1>PseudoBench</h1>
        <p className={styles.subtitle}>
          Performance benchmarks: read &rarr; pseudo-translate &rarr; write
        </p>

        {error && <div className={styles.error}>{error}</div>}

        {report && (
          <>
            <MetadataBar metadata={report.metadata} />
            <Legend engines={engines} />
            <SpeedupSummary pairs={speedups} />

            {/* Filters */}
            <div className={styles.filters}>
              {/* Category tabs */}
              {categories.length > 1 && (
                <div className={styles.filterGroup}>
                  <span className={styles.filterLabel}>Category:</span>
                  {categories.map((cat) => (
                    <button
                      key={cat}
                      className={`${styles.filterBtn} ${selectedCategory === cat ? styles.filterBtnActive : ''}`}
                      onClick={() => switchCategory(cat)}
                    >
                      {CATEGORY_LABELS[cat] ?? cat}
                    </button>
                  ))}
                </div>
              )}

              {/* Format filter */}
              {formats.length > 1 && (
                <div className={styles.filterGroup}>
                  <span className={styles.filterLabel}>Format:</span>
                  <button
                    className={`${styles.filterBtn} ${!selectedFormat ? styles.filterBtnActive : ''}`}
                    onClick={() => setSelectedFormat('')}
                  >
                    All
                  </button>
                  {formats.map((f) => (
                    <button
                      key={f}
                      className={`${styles.filterBtn} ${selectedFormat === f ? styles.filterBtnActive : ''}`}
                      onClick={() => toggleFormat(f)}
                    >
                      {f}
                    </button>
                  ))}
                </div>
              )}

              <div className={styles.filterGroup}>
                <span className={styles.filterLabel}>Size:</span>
                <button
                  className={`${styles.filterBtn} ${!selectedSize ? styles.filterBtnActive : ''}`}
                  onClick={() => setSelectedSize('')}
                >
                  All
                </button>
                {sizes.map((s) => (
                  <button
                    key={s}
                    className={`${styles.filterBtn} ${selectedSize === s ? styles.filterBtnActive : ''}`}
                    onClick={() => toggleSize(s)}
                  >
                    {s}
                  </button>
                ))}
              </div>

              <div className={styles.filterGroup}>
                <span className={styles.filterLabel}>View:</span>
                <button
                  className={`${styles.filterBtn} ${viewMode === 'cards' ? styles.filterBtnActive : ''}`}
                  onClick={() => setViewMode('cards')}
                >
                  Comparison Cards
                </button>
                <button
                  className={`${styles.filterBtn} ${viewMode === 'table' ? styles.filterBtnActive : ''}`}
                  onClick={() => setViewMode('table')}
                >
                  Overview Table
                </button>
              </div>
            </div>

            {viewMode === 'cards' && (
              <div className={styles.compGrid2col}>
                {[...groups.entries()].map(([key, data]) => {
                  const [format, fileSize] = key.split('/');
                  return (
                    <ComparisonCard
                      key={key}
                      format={format}
                      fileSize={fileSize}
                      data={data}
                      metricMaxes={metricMaxes}
                      isCollection={isCollection}
                    />
                  );
                })}
              </div>
            )}

            {viewMode === 'table' && (
              <OverviewTable
                groups={groups}
                engines={engines}
                isCollection={isCollection}
              />
            )}

            {groups.size === 0 && (
              <p className={styles.empty}>No benchmarks match the selected filters.</p>
            )}
          </>
        )}

        {!report && !error && <p>Loading benchmark data...</p>}
      </div>
    </Layout>
  );
}
