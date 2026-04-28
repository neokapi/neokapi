import { useMemo, useState } from "react";
import Layout from "@theme/Layout";
import report from "@site/static/data/parity-report.json";
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

const data = report as unknown as ReportData;

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

function formatPercent(n: number, d: number): string {
  if (d === 0) return "0%";
  return `${Math.round((n / d) * 100)}%`;
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
          Per-format and per-step parity status produced by{" "}
          <code>make parity-publish</code>. Each row corresponds to one Okapi
          filter or pipeline step; head-to-head rows compare neokapi (Go) and
          Okapi (Java via okapi-bridge) on the same input. Bridge-only rows are
          stability gates (the daemon must accept the input and complete
          without erroring) until a Go counterpart lands. Report generated{" "}
          <strong>{data.generated_at}</strong>.
        </p>

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
