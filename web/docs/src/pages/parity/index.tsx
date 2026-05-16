import { useMemo, useState } from "react";
import Layout from "@theme/Layout";
import report from "@site/static/data/parity-report.json";
import fixturesData from "@site/static/data/parity-fixtures.json";
import styles from "./index.module.css";

type Status = "pass" | "fail" | "skip" | "error";
type Kind = "format" | "step";

interface Row {
  kind: Kind;
  id: string;
  status: Status;
  mode?: string;
  detail?: string;
  duration_ms?: number;
}

interface KindTotals {
  pass: number;
  fail: number;
  skip: number;
  error: number;
  total: number;
}

interface ReportData {
  generated_at: string;
  totals: Record<string, KindTotals>;
  rows: Row[];
}

type Severity = "bug" | "cosmetic" | "native-more-correct" | "fixture-bug" | "unknown";

interface FixtureAnnotation {
  severity?: Severity;
  issue?: number;
  issue_url?: string;
}

interface FixtureRow {
  fixture: string;
  achieved: string;
  annotation?: FixtureAnnotation;
}

interface FixturesFormat {
  format: string;
  engine: string;
  fixtures?: FixtureRow[];
}

interface FixturesShape {
  formats: FixturesFormat[];
}

const data = report as unknown as ReportData;
const fixtures = fixturesData as unknown as FixturesShape;

const kindLabels: Record<Kind, string> = {
  format: "Formats (DataFormatReader/Writer)",
  step: "Steps (Tools)",
};

const statusOrder: Status[] = ["fail", "error", "skip", "pass"];

const statusBadge: Record<Status, string> = {
  pass: styles.badgePass,
  fail: styles.badgeFail,
  skip: styles.badgeSkip,
  error: styles.badgeError,
};

const severityOrder: Severity[] = [
  "bug",
  "unknown",
  "cosmetic",
  "native-more-correct",
  "fixture-bug",
];

const severityLabel: Record<Severity, string> = {
  bug: "bug",
  cosmetic: "cosmetic",
  "native-more-correct": "native+",
  "fixture-bug": "fixture-bug",
  unknown: "unannotated",
};

const severityBadgeClass: Record<Severity, string> = {
  bug: styles.badgeFail,
  cosmetic: styles.badgePass, // visually muted positive — renders identically
  "native-more-correct": styles.badgePass,
  "fixture-bug": styles.badgeSkip,
  unknown: styles.badgeError,
};

function formatPercent(n: number, d: number): string {
  if (d === 0) return "0%";
  return `${Math.round((n / d) * 100)}%`;
}

function severityCounts(): Record<Severity, number> {
  const init: Record<Severity, number> = {
    bug: 0,
    cosmetic: 0,
    "native-more-correct": 0,
    "fixture-bug": 0,
    unknown: 0,
  };
  for (const f of fixtures.formats) {
    if (!f.fixtures) continue;
    for (const fx of f.fixtures) {
      if (fx.achieved !== "divergent") continue;
      const sev: Severity = fx.annotation?.severity ?? (fx.annotation ? "unknown" : "unknown");
      init[sev] = (init[sev] ?? 0) + 1;
    }
  }
  return init;
}

// DivergenceSeverityCard summarises divergent-fixture severity counts
// at the top of the /parity dashboard. The unannotated count is the
// fail-new gate signal — anything > 0 means a divergence landed
// without a YAML annotation, which CI will fail on.
function DivergenceSeverityCard() {
  const counts = severityCounts();
  const total = severityOrder.reduce((n, s) => n + (counts[s] ?? 0), 0);
  if (total === 0) {
    return null;
  }
  return (
    <div className={styles.severityCard}>
      <h2 className={styles.severityCardTitle}>Divergent fixtures by severity</h2>
      <ul className={styles.severityCardList}>
        {severityOrder.map((s) => {
          const n = counts[s] ?? 0;
          if (n === 0) return null;
          return (
            <li key={s}>
              <span className={severityBadgeClass[s]}>{severityLabel[s]}</span> <strong>{n}</strong>
            </li>
          );
        })}
      </ul>
      <p className={styles.severityCardHint}>
        Severity is loaded from <code>core/formats/&lt;format&gt;/parity-annotations.yaml</code>.{" "}
        <a href="/parity/fixtures">Open fixtures drill-down</a> for the full list with issue links,
        spec references, and diff snippets.
      </p>
    </div>
  );
}

export default function ParityDashboard() {
  const [search, setSearch] = useState("");
  const [activeKind, setActiveKind] = useState<Kind | null>(null);
  const [activeStatus, setActiveStatus] = useState<Status | null>(null);

  const filtered = useMemo<Row[]>(() => {
    const q = search.trim().toLowerCase();
    return data.rows.filter((r) => {
      if (activeKind && r.kind !== activeKind) return false;
      if (activeStatus && r.status !== activeStatus) return false;
      if (q && !r.id.toLowerCase().includes(q) && !(r.detail ?? "").toLowerCase().includes(q)) {
        return false;
      }
      return true;
    });
  }, [search, activeKind, activeStatus]);

  // Sort: fails first, then skips, then passes; alphabetical within.
  const sorted = useMemo<Row[]>(() => {
    return [...filtered].sort((a, b) => {
      if (a.kind !== b.kind) return a.kind.localeCompare(b.kind);
      const sa = statusOrder.indexOf(a.status);
      const sb = statusOrder.indexOf(b.status);
      if (sa !== sb) return sa - sb;
      return a.id.localeCompare(b.id);
    });
  }, [filtered]);

  return (
    <Layout
      title="Parity Dashboard"
      description="Per-format and per-step parity status between neokapi (Go) and Okapi Framework via okapi-bridge"
    >
      <main className="container margin-vert--lg">
        <h1>Parity Dashboard</h1>
        <p className={styles.subtitle}>
          Per-format and per-step parity status produced by <code>make parity-publish</code>. Each
          row corresponds to one Okapi filter or pipeline step; head-to-head rows compare neokapi
          (Go) and Okapi (Java via okapi-bridge) on the same input. Bridge-only rows are stability
          gates (the daemon must accept the input and complete without erroring) until a Go
          counterpart lands. Report generated <strong>{data.generated_at}</strong>.
        </p>
        <p className={styles.subtitle}>
          For per-fixture divergence detail (first-diff offsets, byte deltas, comparison snippets,
          severity, GitHub issue links) see the <a href="/parity/fixtures">fixtures drill-down</a>.
        </p>

        <DivergenceSeverityCard />

        <div className={styles.totals}>
          {(Object.keys(data.totals) as Kind[]).map((kind) => {
            const t = data.totals[kind];
            return (
              <div key={kind} className={styles.totalCard}>
                <h2>{kindLabels[kind] ?? kind}</h2>
                <div className={styles.headline}>
                  {t.pass} / {t.total}{" "}
                  <span className={styles.headlineSuffix}>
                    pass ({formatPercent(t.pass, t.total)})
                  </span>
                </div>
                <ul className={styles.totalBreakdown}>
                  <li>
                    <span className={statusBadge.pass}>pass</span> {t.pass}
                  </li>
                  <li>
                    <span className={statusBadge.fail}>fail</span> {t.fail}
                  </li>
                  {t.error > 0 && (
                    <li>
                      <span className={statusBadge.error}>error</span> {t.error}
                    </li>
                  )}
                  <li>
                    <span className={statusBadge.skip}>skip</span> {t.skip}
                  </li>
                </ul>
              </div>
            );
          })}
        </div>

        <div className={styles.toolbar}>
          <input
            type="text"
            className={styles.search}
            placeholder="Filter by id or skip reason..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
          <div className={styles.buttons}>
            <button
              className={`button button--sm ${activeKind === null ? "button--primary" : "button--outline button--secondary"}`}
              onClick={() => setActiveKind(null)}
            >
              All kinds
            </button>
            {(["format", "step"] as Kind[]).map((k) => (
              <button
                key={k}
                className={`button button--sm ${activeKind === k ? "button--primary" : "button--outline button--secondary"}`}
                onClick={() => setActiveKind(activeKind === k ? null : k)}
              >
                {k}
              </button>
            ))}
            <span className={styles.separator}>|</span>
            <button
              className={`button button--sm ${activeStatus === null ? "button--primary" : "button--outline button--secondary"}`}
              onClick={() => setActiveStatus(null)}
            >
              All statuses
            </button>
            {(["pass", "fail", "skip"] as Status[]).map((s) => (
              <button
                key={s}
                className={`button button--sm ${activeStatus === s ? "button--primary" : "button--outline button--secondary"}`}
                onClick={() => setActiveStatus(activeStatus === s ? null : s)}
              >
                {s}
              </button>
            ))}
          </div>
        </div>

        <p className={styles.resultCount}>{sorted.length} rows</p>

        <table className={styles.table}>
          <thead>
            <tr>
              <th>Kind</th>
              <th>ID</th>
              <th>Status</th>
              <th>Mode</th>
              <th>Detail</th>
              <th>Duration</th>
            </tr>
          </thead>
          <tbody>
            {sorted.map((r) => (
              <tr key={`${r.kind}/${r.id}`}>
                <td>{r.kind}</td>
                <td>
                  <code>{r.id}</code>
                </td>
                <td>
                  <span className={statusBadge[r.status]}>{r.status}</span>
                </td>
                <td className={styles.modeCell}>{r.mode ?? "—"}</td>
                <td className={styles.detailCell}>{r.detail || ""}</td>
                <td className={styles.durationCell}>
                  {r.duration_ms ? `${r.duration_ms} ms` : ""}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {sorted.length === 0 && <p>No rows match the current filters.</p>}
      </main>
    </Layout>
  );
}
