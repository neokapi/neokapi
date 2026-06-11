import { useState, useMemo } from "react";
import type { TestCaseRow, TestState, SkipCategory } from "./_types";
import { skipCategoryLabels, skipCategoryColors } from "./_types";
import styles from "./_index.module.css";

interface Props {
  testCases: TestCaseRow[];
  filterName: string;
  goCommitSHA?: string;
  okapiTag?: string;
  defaultFilter?: string;
}

type FilterMode =
  | "all"
  | "okapi"
  | "bridge"
  | "native"
  | "failing"
  | "implemented"
  | "pending"
  | "skipped"
  | "unmapped"
  | "not-applicable";
type SortMode = "name" | "status" | "category";

const statusBadgeClass: Record<string, string> = {
  pass: "badge badge--success",
  fail: "badge badge--danger",
  error: "badge badge--danger",
  skip: "badge badge--secondary",
};

function StatusCell({ status }: { status: string }) {
  if (!status) {
    return <span className={styles.statusDash}>&mdash;</span>;
  }
  return <span className={statusBadgeClass[status]}>{status}</span>;
}

/** CSS class for test state row background. */
function stateRowClass(state?: TestState): string {
  switch (state) {
    case "implemented":
      return styles.stateImplemented ?? "";
    case "pending":
      return styles.statePending ?? "";
    case "skipped":
      return styles.stateSkipped ?? "";
    default:
      return "";
  }
}

function statusOrder(s: string): number {
  switch (s) {
    case "fail":
      return 0;
    case "error":
      return 1;
    case "skip":
      return 2;
    case "pass":
      return 3;
    default:
      return 4;
  }
}

/** Category badge for not-applicable tests. */
function CategoryBadge({ category }: { category?: SkipCategory }) {
  if (!category) return null;
  const label = skipCategoryLabels[category] ?? category;
  const color = skipCategoryColors[category] ?? "#94a3b8";
  return (
    <span className={styles.categoryBadge} style={{ backgroundColor: color }} title={label}>
      {label}
    </span>
  );
}

/** Build a GitHub source URL for a Go test file+line. */
function goSourceUrl(
  file: string | undefined,
  line: number | undefined,
  commitSHA: string | undefined,
  filterName: string,
  kind: "bridge" | "native",
): string {
  const ref = commitSHA || "main";
  if (file) {
    const base = `https://github.com/neokapi/neokapi/blob/${ref}/${file}`;
    return line ? `${base}#L${line}` : base;
  }
  // Fallback to directory
  const dir =
    kind === "bridge"
      ? `core/plugin/bridge/filters/okf_${filterName}/`
      : `core/formats/${filterName}/`;
  return `https://github.com/neokapi/neokapi/tree/${ref}/${dir}`;
}

/** Build a GitLab source URL for an Okapi Java test file. */
function okapiSourceUrl(
  okapiFile: string | undefined,
  okapiTag: string | undefined,
): string | null {
  if (!okapiFile) return null;
  const ref = okapiTag || "master";
  return `https://gitlab.com/okapiframework/Okapi/-/blob/${ref}/${okapiFile}?ref_type=tags`;
}

/** Check if a test case is not-applicable (has skip reason but no state='implemented'). */
function isNotApplicable(tc: TestCaseRow): boolean {
  return tc.testState !== "implemented" && tc.testState !== "pending" && !!tc.skipReason;
}

export default function TestCaseTable({
  testCases,
  filterName,
  goCommitSHA,
  okapiTag,
  defaultFilter,
}: Props) {
  const [filter, setFilter] = useState<FilterMode>((defaultFilter as FilterMode) || "all");
  const [sort, setSort] = useState<SortMode>("name");
  const [expandedRow, setExpandedRow] = useState<string | null>(null);

  // Compute counts
  const counts = useMemo(() => {
    let implemented = 0;
    let pending = 0;
    let notApplicable = 0;
    let unmapped = 0;
    let failing = 0;
    const categories: Record<string, number> = {};

    for (const tc of testCases) {
      if (tc.testState === "implemented") implemented++;
      else if (tc.testState === "pending") pending++;
      else if (isNotApplicable(tc)) {
        notApplicable++;
        if (tc.skipCategory) {
          categories[tc.skipCategory] = (categories[tc.skipCategory] || 0) + 1;
        }
      } else unmapped++;

      if (
        tc.okapiStatus === "fail" ||
        tc.okapiStatus === "error" ||
        tc.bridgeStatus === "fail" ||
        tc.bridgeStatus === "error" ||
        tc.nativeStatus === "fail" ||
        tc.nativeStatus === "error"
      )
        failing++;
    }

    return { implemented, pending, notApplicable, unmapped, failing, categories };
  }, [testCases]);

  const filtered = testCases.filter((tc) => {
    switch (filter) {
      case "okapi":
        return tc.okapiStatus !== "";
      case "bridge":
        return tc.bridgeStatus !== "";
      case "native":
        return tc.nativeStatus !== "";
      case "failing":
        return (
          tc.okapiStatus === "fail" ||
          tc.okapiStatus === "error" ||
          tc.bridgeStatus === "fail" ||
          tc.bridgeStatus === "error" ||
          tc.nativeStatus === "fail" ||
          tc.nativeStatus === "error"
        );
      case "implemented":
        return tc.testState === "implemented";
      case "pending":
        return tc.testState === "pending";
      case "not-applicable":
        return isNotApplicable(tc);
      case "unmapped":
        return !tc.testState && !tc.skipReason;
      default:
        return true;
    }
  });

  const sorted = [...filtered].sort((a, b) => {
    if (sort === "status") {
      const aMin = Math.min(
        statusOrder(a.okapiStatus),
        statusOrder(a.bridgeStatus),
        statusOrder(a.nativeStatus),
      );
      const bMin = Math.min(
        statusOrder(b.okapiStatus),
        statusOrder(b.bridgeStatus),
        statusOrder(b.nativeStatus),
      );
      if (aMin !== bMin) return aMin - bMin;
    } else if (sort === "category") {
      const aCat = a.skipCategory || "zzz";
      const bCat = b.skipCategory || "zzz";
      if (aCat !== bCat) return aCat.localeCompare(bCat);
    }
    return a.testName.localeCompare(b.testName);
  });

  const filterButtons: { mode: FilterMode; label: string; count: number }[] = [
    { mode: "all", label: "All", count: testCases.length },
    { mode: "implemented", label: "Implemented", count: counts.implemented },
    {
      mode: "not-applicable",
      label: "Not Applicable",
      count: counts.notApplicable,
    },
    ...(counts.pending > 0
      ? [{ mode: "pending" as FilterMode, label: "Pending", count: counts.pending }]
      : []),
    ...(counts.unmapped > 0
      ? [{ mode: "unmapped" as FilterMode, label: "Unmapped", count: counts.unmapped }]
      : []),
    ...(counts.failing > 0
      ? [{ mode: "failing" as FilterMode, label: "Failing", count: counts.failing }]
      : []),
  ];

  const toggleRow = (key: string) => {
    setExpandedRow(expandedRow === key ? null : key);
  };

  return (
    <div className={styles.testCaseTableWrap}>
      {/* State summary mini-bar */}
      {testCases.length > 0 && (
        <div className={styles.filterStateBar}>
          <div className={styles.filterStateSegments}>
            {[
              { value: counts.implemented, color: "#2e8555", label: "Implemented" },
              { value: counts.notApplicable, color: "#94a3b8", label: "Not Applicable" },
              { value: counts.pending, color: "#e3a008", label: "Pending" },
              { value: counts.unmapped, color: "#dc2626", label: "Unmapped" },
            ]
              .filter((s) => s.value > 0)
              .map((s, i) => (
                <div
                  key={i}
                  className={styles.filterStateSegment}
                  style={{
                    width: `${(s.value / testCases.length) * 100}%`,
                    backgroundColor: s.color,
                  }}
                  title={`${s.label}: ${s.value}`}
                />
              ))}
          </div>
        </div>
      )}

      {/* Category breakdown for this filter */}
      {Object.keys(counts.categories).length > 0 && (
        <div className={styles.filterCategoryRow}>
          {Object.entries(counts.categories)
            .sort(([, a], [, b]) => b - a)
            .map(([cat, count]) => {
              const label = skipCategoryLabels[cat as SkipCategory] ?? cat;
              const color = skipCategoryColors[cat as SkipCategory] ?? "#94a3b8";
              return (
                <span key={cat} className={styles.filterCategoryTag}>
                  <span className={styles.categoryDot} style={{ backgroundColor: color }} />
                  {label}: {count}
                </span>
              );
            })}
        </div>
      )}

      <div className={styles.testCaseToolbar}>
        <div className={styles.testCaseFilterButtons}>
          {filterButtons.map((fb) => (
            <button
              key={fb.mode}
              className={`button button--sm ${filter === fb.mode ? "button--primary" : "button--outline button--secondary"}`}
              onClick={(e) => {
                e.stopPropagation();
                setFilter(fb.mode);
              }}
            >
              {fb.label} ({fb.count})
            </button>
          ))}
        </div>
        <div className={styles.testCaseSortButtons}>
          <span className={styles.sortLabel}>Sort:</span>
          {(["name", "status", "category"] as SortMode[]).map((mode) => (
            <button
              key={mode}
              className={`button button--sm ${sort === mode ? "button--primary" : "button--outline button--secondary"}`}
              onClick={(e) => {
                e.stopPropagation();
                setSort(mode);
              }}
            >
              {mode.charAt(0).toUpperCase() + mode.slice(1)}
            </button>
          ))}
        </div>
      </div>
      <table className={styles.testCaseTable}>
        <thead>
          <tr>
            <th>Test</th>
            <th className={styles.testCaseStatusHeader}>Okapi</th>
            <th className={styles.testCaseStatusHeader}>Bridge</th>
            <th className={styles.testCaseStatusHeader}>Native</th>
          </tr>
        </thead>
        <tbody>
          {sorted.map((tc, i) => {
            const rowKey = `${tc.testName}-${i}`;
            const isExpanded = expandedRow === rowKey;
            const showCategory = isNotApplicable(tc);
            return (
              <>
                <tr
                  key={rowKey}
                  className={`${styles.testCaseRow} ${isExpanded ? styles.testCaseRowExpanded : ""} ${stateRowClass(tc.testState)} ${showCategory ? styles.stateNotApplicable : ""}`}
                  title={
                    tc.skipReason
                      ? `${isNotApplicable(tc) ? "Not applicable" : tc.testState}: ${tc.skipReason}`
                      : undefined
                  }
                  onClick={(e) => {
                    e.stopPropagation();
                    toggleRow(rowKey);
                  }}
                >
                  <td className={styles.testCaseName}>
                    {tc.javaClass ? (
                      <>
                        <span className={styles.testCaseClass}>{tc.javaClass}</span>
                        <span className={styles.testCaseMethod}>#{tc.testName}</span>
                      </>
                    ) : (
                      <span className={styles.testCaseGoName}>{tc.testName}</span>
                    )}
                    {showCategory && tc.skipCategory && (
                      <CategoryBadge category={tc.skipCategory} />
                    )}
                  </td>
                  <td className={styles.testCaseStatus}>
                    <StatusCell status={tc.okapiStatus} />
                  </td>
                  <td className={styles.testCaseStatus}>
                    <StatusCell status={tc.bridgeStatus} />
                    {tc.bridgeSubtests != null && tc.bridgeSubtests > 0 && (
                      <span className={styles.subtestCount} title={`${tc.bridgeSubtests} subtests`}>
                        +{tc.bridgeSubtests}
                      </span>
                    )}
                  </td>
                  <td className={styles.testCaseStatus}>
                    <StatusCell status={tc.nativeStatus} />
                    {tc.nativeSubtests != null && tc.nativeSubtests > 0 && (
                      <span className={styles.subtestCount} title={`${tc.nativeSubtests} subtests`}>
                        +{tc.nativeSubtests}
                      </span>
                    )}
                  </td>
                </tr>
                {isExpanded && (
                  <tr key={`${rowKey}-detail`} className={styles.detailRow}>
                    <td colSpan={4}>
                      <div className={styles.detailContent}>
                        {tc.okapiStatus && tc.javaClass && (
                          <div className={styles.detailItem}>
                            <span className={styles.detailLabel}>Okapi:</span>
                            {(() => {
                              const url = okapiSourceUrl(tc.okapiFile, okapiTag);
                              return url ? (
                                <a
                                  href={url}
                                  target="_blank"
                                  rel="noopener noreferrer"
                                  onClick={(e) => e.stopPropagation()}
                                >
                                  <code>
                                    {tc.javaClass}#{tc.testName}
                                  </code>
                                </a>
                              ) : (
                                <code>
                                  {tc.javaClass}#{tc.testName}
                                </code>
                              );
                            })()}
                            {tc.okapiFile && (
                              <span className={styles.detailPath}>{tc.okapiFile}</span>
                            )}
                          </div>
                        )}
                        {(tc.bridgeTest || tc.bridgeStatus) && (
                          <div className={styles.detailItem}>
                            <span className={styles.detailLabel}>Bridge:</span>
                            <a
                              href={goSourceUrl(
                                tc.bridgeFile,
                                tc.bridgeLine,
                                goCommitSHA,
                                filterName,
                                "bridge",
                              )}
                              target="_blank"
                              rel="noopener noreferrer"
                              onClick={(e) => e.stopPropagation()}
                            >
                              <code>{tc.bridgeTest || tc.testName}</code>
                            </a>
                            {tc.bridgeFile && (
                              <span className={styles.detailPath}>
                                {tc.bridgeFile}
                                {tc.bridgeLine ? `:${tc.bridgeLine}` : ""}
                              </span>
                            )}
                          </div>
                        )}
                        {(tc.nativeTest || tc.nativeStatus) && (
                          <div className={styles.detailItem}>
                            <span className={styles.detailLabel}>Native:</span>
                            <a
                              href={goSourceUrl(
                                tc.nativeFile,
                                tc.nativeLine,
                                goCommitSHA,
                                filterName,
                                "native",
                              )}
                              target="_blank"
                              rel="noopener noreferrer"
                              onClick={(e) => e.stopPropagation()}
                            >
                              <code>{tc.nativeTest || tc.testName}</code>
                            </a>
                            {tc.nativeFile && (
                              <span className={styles.detailPath}>
                                {tc.nativeFile}
                                {tc.nativeLine ? `:${tc.nativeLine}` : ""}
                              </span>
                            )}
                          </div>
                        )}
                        {tc.skipReason && (
                          <div className={styles.detailItem}>
                            <span className={styles.detailLabel}>Reason:</span>
                            <span>{tc.skipReason}</span>
                            {tc.skipCategory && <CategoryBadge category={tc.skipCategory} />}
                          </div>
                        )}
                        {tc.coveredByTest && (
                          <div className={styles.detailItem}>
                            <span className={styles.detailLabel}>Covered by:</span>
                            <a
                              href={`https://github.com/neokapi/neokapi/blob/${goCommitSHA || "main"}/${tc.coveredByFile}${
                                tc.coveredByLine ? `#L${tc.coveredByLine}` : ""
                              }`}
                              target="_blank"
                              rel="noopener noreferrer"
                            >
                              {tc.coveredByTest} ↗
                            </a>
                          </div>
                        )}
                        {!tc.okapiStatus &&
                          !tc.bridgeTest &&
                          !tc.bridgeStatus &&
                          !tc.nativeTest &&
                          !tc.nativeStatus && (
                            <span className={styles.noData}>No source mapping available.</span>
                          )}
                      </div>
                    </td>
                  </tr>
                )}
              </>
            );
          })}
          {sorted.length === 0 && (
            <tr>
              <td colSpan={4} className={styles.noData}>
                No test cases match the current filter.
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
}
