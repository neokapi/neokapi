import { useCallback, useEffect, useMemo, useState } from "react";
import {
  ShieldCheck,
  ShieldAlert,
  ShieldQuestion,
  Play,
  Loader2,
  Wand2,
  FileText,
  FileSearch,
  CheckCircle2,
  Compass,
  TriangleAlert,
} from "lucide-react";
import {
  Button,
  Badge,
  Card,
  CardContent,
  PageHeader,
  ScrollArea,
  FindingSnippet,
  findingSeverityTone,
  findingToneBadgeClass,
} from "@neokapi/ui-primitives";
import type { ContentTree } from "@neokapi/ui-primitives/preview";
import { t } from "@neokapi/i18n-react/runtime";
import { api } from "../hooks/useApi";
import { useError } from "./ErrorBanner";
import { FilePreview } from "./FilePreview";
import { useActiveFilter } from "../context/ActiveFilterContext";
import { findingHighlights, findingSide } from "../lib/findingHighlights";
import type {
  CheckFileResult,
  CheckNotRunCause,
  CheckRunResult,
  CheckWarning,
  DesktopFinding,
} from "../types/api";

/**
 * The sentence for why a run did not run. A broken checker and an empty scope
 * share the verdict, so the sentence is what keeps a reader from taking one for
 * the other.
 */
function notRunSentence(cause: CheckNotRunCause | undefined): string {
  switch (cause) {
    case "checker_invalid":
      return t("A checker failed its canary, so this run's result cannot be trusted.");
    case "nothing_to_check":
      return t("There was nothing in scope to check.");
    case "content_not_checked":
      return t("Content in scope was not checked.");
    default:
      return t("The checks did not run.");
  }
}

export interface ChecksPanelProps {
  /** Project tab ID — the project whose content is checked. */
  tabID: string;
  /** Pre-loaded result for Storybook/tests — skips api.runChecks(). */
  result?: CheckRunResult;
  /** Force the loading/skeleton state (for Storybook). */
  forceLoading?: boolean;
  /**
   * Override the fix handler (for Storybook/tests). Receives the file path and
   * the finding; returns once the fix is applied. Defaults to api.applyCheckFix.
   */
  onApplyFix?: (filePath: string, finding: DesktopFinding) => Promise<void>;
  /**
   * Open the Context explorer standing at a finding's point, with the rule that
   * fired named. Absent renders the findings without the click-through.
   */
  onOpenContext?: (pin: {
    coordinate?: string;
    collection?: string;
    path?: string;
    rule?: string;
  }) => void;
  /**
   * Pre-loaded document for Storybook/tests: handed to the preview in place of
   * inspecting the file, so a finding can be opened without a backend.
   */
  previewTree?: ContentTree;
}

/** A finding severity as a badge: the shared tone, and the word the panel uses for it. */
function severityBadge(severity: string): { className: string; label: string } {
  const className = findingToneBadgeClass(findingSeverityTone(severity));
  switch (severity) {
    case "critical":
      return { className, label: "Critical" };
    case "major":
      return { className, label: "Major" };
    case "minor":
      return { className, label: "Minor" };
    default:
      return { className, label: "Info" };
  }
}

function shortPath(p: string): string {
  const parts = p.split(/[\\/]/);
  return parts.slice(-2).join("/") || p;
}

/**
 * The finding in the text it was raised on: the block's runs on the side the
 * finding names, with the span marked. A finding about the translation carries
 * a position anchored to the source runs (core/check), so for one of those the
 * target text is read first and the source follows with the words underlined.
 * Falls back to the quoted text when the block's runs did not travel with the
 * finding.
 */
function FindingInContext({ finding }: { finding: DesktopFinding }) {
  const tone = findingSeverityTone(finding.severity);
  const onSource = finding.field !== "target";
  const sideRuns = onSource ? finding.source_runs : finding.target_runs;
  const hasRuns = !!sideRuns && sideRuns.length > 0;
  const sourceMarked =
    !onSource && !!finding.position && !!finding.source_runs && finding.source_runs.length > 0;
  if (!hasRuns && !finding.original_text && !sourceMarked) return null;
  return (
    <div className="mt-1.5 space-y-0.5 text-xs" data-testid="finding-context">
      {(hasRuns || finding.original_text) && (
        <FindingSnippet
          runs={sideRuns}
          locale={finding.locale}
          anchor={onSource ? finding.position : undefined}
          tone={tone}
          label={finding.message}
          fallbackText={finding.original_text}
          data-testid="finding-snippet"
        />
      )}
      {sourceMarked && (
        <div
          className="flex items-baseline gap-1.5 text-muted-foreground"
          data-testid="finding-snippet-source"
        >
          <Badge
            variant="outline"
            className="shrink-0 text-[10px] text-muted-foreground"
            title={t("Where the finding sits in the source text.")}
          >
            {t("source")}
          </Badge>
          <FindingSnippet
            runs={finding.source_runs}
            anchor={finding.position}
            tone={tone}
            label={finding.message}
          />
        </div>
      )}
    </div>
  );
}

/**
 * Configuration warnings, listed apart from the findings. Each names a profile,
 * and the key in it, to fix. A warning carries no severity because it changes
 * neither the verdict nor the score.
 */
function ConfigurationWarnings({ warnings }: { warnings: CheckWarning[] }) {
  return (
    <Card className="mb-4 border-amber-500/40" data-testid="check-warnings">
      <CardContent className="p-4">
        <div className="mb-1 flex items-center gap-2 text-sm font-semibold">
          <TriangleAlert size={14} className="text-amber-500" />
          {t("Configuration warnings")}
        </div>
        <p className="mb-3 text-xs text-muted-foreground">
          {t(
            "Each names configuration to fix. Warnings are separate from findings and leave the verdict and the score unchanged.",
          )}
        </p>
        <ul className="space-y-2">
          {warnings.map((warning) => (
            <li
              key={`${warning.source}#${warning.key ?? ""}#${warning.code}#${warning.message}`}
              className="text-sm"
              data-testid="check-warning"
            >
              <div className="mb-0.5 flex flex-wrap items-center gap-1.5">
                <Badge
                  variant="secondary"
                  className="font-mono text-[10px] font-normal"
                  translate="no"
                >
                  {warning.code}
                </Badge>
                <span className="text-xs text-muted-foreground" translate="no">
                  {shortPath(warning.source)}
                  {warning.key ? `: ${warning.key}` : ""}
                </span>
              </div>
              <p>{warning.message}</p>
            </li>
          ))}
        </ul>
      </CardContent>
    </Card>
  );
}

export function ChecksPanel({
  tabID,
  result: propResult,
  forceLoading = false,
  onApplyFix,
  onOpenContext,
  previewTree,
}: ChecksPanelProps) {
  const { showError } = useError();
  const { active: activeFilter } = useActiveFilter();
  const [result, setResult] = useState<CheckRunResult | null>(propResult ?? null);
  const [loading, setLoading] = useState(forceLoading);
  const [fixingKey, setFixingKey] = useState<string | null>(null);
  // The document a finding is being read in. The list stays mounted behind the
  // sheet, so closing it lands back on the same card with the list where it was.
  const [documentOpen, setDocumentOpen] = useState(false);
  const [documentAt, setDocumentAt] = useState<{
    file: CheckFileResult;
    finding: DesktopFinding | null;
  } | null>(null);

  // When a caller supplies a result (Storybook/tests), treat it as the source
  // of truth so an interactive parent can drive the panel (e.g. swap the result
  // after a simulated fix).
  useEffect(() => {
    if (propResult) setResult(propResult);
  }, [propResult]);

  // Checks are scoped by the project's Active Filter (collections + glob of
  // files, and its target languages). There is no per-panel language picker —
  // pick which languages to check via the menu-bar filter. No languages → only
  // source-side checks run.
  const runChecks = useCallback(async () => {
    if (propResult) return; // Storybook/tests supply a fixed result.
    setLoading(true);
    try {
      const res = await api.runChecks(tabID, activeFilter ?? { id: "", name: "" });
      setResult(
        res ?? {
          pass: false,
          verdict: "did_not_run",
          did_not_run_cause: "content_not_checked",
          did_not_run: [t("The checks returned no result.")],
          score: 0,
          files: [],
        },
      );
    } catch (err) {
      showError("Failed to run checks", err);
    } finally {
      setLoading(false);
    }
  }, [tabID, activeFilter, propResult, showError]);

  const handleApplyFix = useCallback(
    async (filePath: string, finding: DesktopFinding, key: string) => {
      setFixingKey(key);
      try {
        if (onApplyFix) {
          await onApplyFix(filePath, finding);
        } else {
          await api.applyCheckFix(
            tabID,
            filePath,
            finding.block_id ?? "",
            finding.field ?? "source",
            finding.original_text ?? "",
            finding.replacement ?? "",
          );
        }
        // Re-run so the resolved finding disappears and the score updates.
        await runChecks();
      } catch (err) {
        showError("Failed to apply fix", err);
      } finally {
        setFixingKey(null);
      }
    },
    [tabID, onApplyFix, runChecks, showError],
  );

  // Open the file in the document sheet: at one finding's block, on the side it
  // names, or on the whole file with every finding drawn. A finding that names
  // no block (a file that could not be read) opens the file.
  const openInDocument = useCallback((file: CheckFileResult, finding: DesktopFinding | null) => {
    setDocumentAt({ file, finding: finding?.block_id ? finding : null });
    setDocumentOpen(true);
  }, []);

  const documentHighlights = useMemo(
    () =>
      documentAt ? findingHighlights(documentAt.file.findings, documentAt.finding) : undefined,
    [documentAt],
  );

  const documentNote = useMemo(() => {
    if (!documentAt) return undefined;
    const finding = documentAt.finding;
    if (!finding) {
      return (
        <span className="text-muted-foreground">
          {t("{count} findings", { count: documentAt.file.findings.length })}
        </span>
      );
    }
    const sev = severityBadge(finding.severity);
    return (
      <>
        <Badge variant="outline" className={sev.className}>
          {sev.label}
        </Badge>
        <span className="min-w-0 truncate text-muted-foreground" title={finding.message}>
          {finding.message}
        </span>
      </>
    );
  }, [documentAt]);

  const totalFindings = useMemo(
    () => (result?.files ?? []).reduce((n, f) => n + f.findings.length, 0),
    [result],
  );

  const filesWithFindings = useMemo(
    () => (result?.files ?? []).filter((f) => f.findings.length > 0),
    [result],
  );

  // A result from a backend that predates the verdict carries only pass.
  const verdict = result?.verdict ?? (result?.pass ? "passed" : "failed");
  const brokenChecker =
    verdict === "did_not_run" && result?.did_not_run_cause === "checker_invalid";

  return (
    <div className="p-6">
      <PageHeader
        title="Checks"
        subtitle="Run content checks like tests over your project: terminology, placeholders, and brand vocabulary. Scope which files and languages to check with the menu-bar filter."
        actions={
          <div className="flex items-center gap-2">
            <Button size="sm" onClick={() => void runChecks()} disabled={loading}>
              {loading ? <Loader2 size={12} className="animate-spin" /> : <Play size={12} />}
              {loading ? "Running..." : "Run checks"}
            </Button>
          </div>
        }
      />

      {/* Verdict summary */}
      {result && !loading && (
        <Card className="mb-4">
          <CardContent className="flex items-center justify-between p-4">
            <div className="flex items-center gap-3">
              {verdict === "passed" && <ShieldCheck size={20} className="text-emerald-500" />}
              {verdict === "failed" && <ShieldAlert size={20} className="text-destructive" />}
              {verdict === "did_not_run" && (
                <ShieldQuestion
                  size={20}
                  className={brokenChecker ? "text-destructive" : "text-amber-500"}
                />
              )}
              <div>
                <div className="text-sm font-semibold">
                  {verdict === "passed" && "Passing"}
                  {verdict === "failed" && "Failing"}
                  {verdict === "did_not_run" && t("Did not run")}
                </div>
                <div className="text-xs text-muted-foreground">
                  {verdict === "did_not_run"
                    ? notRunSentence(result.did_not_run_cause)
                    : totalFindings === 0
                      ? "No findings"
                      : `${totalFindings} finding${totalFindings === 1 ? "" : "s"} across ${filesWithFindings.length} file${filesWithFindings.length === 1 ? "" : "s"}`}
                </div>
              </div>
            </div>
            {verdict !== "did_not_run" && (
              <div className="text-right">
                <div
                  className={`text-2xl font-semibold tabular-nums ${
                    result.score >= 90
                      ? "text-emerald-500"
                      : result.score >= 70
                        ? "text-amber-500"
                        : "text-destructive"
                  }`}
                >
                  {result.score}
                </div>
                <div className="text-[10px] uppercase tracking-wide text-muted-foreground">
                  Score / 100
                </div>
              </div>
            )}
          </CardContent>
        </Card>
      )}

      {/* Did not run: nothing here is a pass, and the reasons say why. */}
      {result && !loading && verdict === "did_not_run" && (
        <Card className={brokenChecker ? "mb-4 border-destructive" : "mb-4 border-dashed"}>
          <CardContent className="p-4">
            <p className="mb-2 text-sm">{notRunSentence(result.did_not_run_cause)}</p>
            {(result.did_not_run ?? []).length > 0 && (
              <ul className="list-disc space-y-1 pl-5 text-xs text-muted-foreground">
                {(result.did_not_run ?? []).map((reason) => (
                  <li key={reason}>{reason}</li>
                ))}
              </ul>
            )}
          </CardContent>
        </Card>
      )}

      {/* Configuration warnings: apart from the findings, whatever the verdict. */}
      {result && !loading && (result.warnings ?? []).length > 0 && (
        <ConfigurationWarnings warnings={result.warnings ?? []} />
      )}

      {/* Loading skeleton */}
      {loading && (
        <div className="space-y-2">
          {[0, 1, 2].map((i) => (
            <Card key={i} className="animate-pulse">
              <CardContent className="h-16 p-4" />
            </Card>
          ))}
        </div>
      )}

      {/* Idle (no run yet) */}
      {!result && !loading && (
        <Card className="border-dashed">
          <CardContent className="p-8 text-center">
            <ShieldCheck size={24} className="mx-auto mb-2 text-muted-foreground/50" />
            <p className="mb-3 text-sm text-muted-foreground">
              Run checks to verify your content against terminology, placeholder integrity, and
              brand vocabulary rules.
            </p>
            <Button size="sm" onClick={() => void runChecks()}>
              <Play size={12} />
              Run checks
            </Button>
          </CardContent>
        </Card>
      )}

      {/* All clear */}
      {result && !loading && verdict === "passed" && totalFindings === 0 && (
        <Card className="border-dashed">
          <CardContent className="p-8 text-center">
            <CheckCircle2 size={24} className="mx-auto mb-2 text-emerald-500" />
            <p className="text-sm text-muted-foreground">
              No findings. Your content passes all checks.
            </p>
          </CardContent>
        </Card>
      )}

      {/* Findings grouped by file */}
      {result && !loading && totalFindings > 0 && (
        <ScrollArea className="h-[calc(100vh-16rem)]">
          <div className="space-y-4 pr-3">
            {filesWithFindings.map((file) => (
              <div key={file.path}>
                <div className="mb-1.5 flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
                  <FileText size={13} />
                  <span translate="no">{shortPath(file.path)}</span>
                  <span className="text-muted-foreground/60">· {file.findings.length}</span>
                  <button
                    type="button"
                    onClick={() => openInDocument(file, null)}
                    className="ml-auto inline-flex items-center gap-1 text-[11px] font-normal hover:text-foreground hover:underline"
                    data-testid="file-open-document"
                  >
                    <FileSearch size={11} />
                    Open in document
                  </button>
                </div>
                <div className="space-y-2">
                  {file.findings.map((finding, idx) => {
                    const sev = severityBadge(finding.severity);
                    const key = `${file.path}#${finding.block_id ?? ""}#${idx}`;
                    return (
                      <Card key={key} data-testid="finding-card">
                        <CardContent className="p-3">
                          <div className="flex items-start justify-between gap-3">
                            <div className="min-w-0 flex-1">
                              <div className="mb-1 flex flex-wrap items-center gap-1.5">
                                <Badge variant="outline" className={sev.className}>
                                  {sev.label}
                                </Badge>
                                <Badge
                                  variant="outline"
                                  className="text-muted-foreground"
                                  translate="no"
                                >
                                  {finding.category}
                                </Badge>
                                {/* The rule that fired and the point it is
                                    scoped to. A finding a reader cannot trace
                                    to a decision is a complaint with nowhere
                                    to go. */}
                                {finding.rule && (
                                  <Badge
                                    variant="secondary"
                                    className="font-mono text-[10px] font-normal"
                                    translate="no"
                                    data-testid="finding-rule"
                                  >
                                    {finding.rule}
                                  </Badge>
                                )}
                                {onOpenContext && (
                                  <button
                                    type="button"
                                    onClick={() =>
                                      onOpenContext({
                                        coordinate: finding.point || undefined,
                                        collection: finding.collection || undefined,
                                        path: file.path,
                                        rule: finding.rule,
                                      })
                                    }
                                    className="inline-flex items-center gap-1 text-[11px] text-muted-foreground hover:text-foreground hover:underline"
                                    data-testid="finding-point"
                                  >
                                    <Compass size={11} />
                                    {finding.point || "project default"}
                                  </button>
                                )}
                              </div>
                              <p className="text-sm">{finding.message}</p>
                              <FindingInContext finding={finding} />
                              {finding.suggestion && (
                                <p className="mt-1 text-xs text-muted-foreground">
                                  <span className="text-muted-foreground/70">{"↳ "}</span>
                                  {finding.suggestion}
                                </p>
                              )}
                            </div>
                            <div className="flex shrink-0 flex-wrap justify-end gap-1.5">
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={() => openInDocument(file, finding)}
                                data-testid="finding-open-document"
                              >
                                <FileSearch size={12} />
                                Open in document
                              </Button>
                              {finding.fixable && (
                                <Button
                                  variant="outline"
                                  size="sm"
                                  disabled={fixingKey === key}
                                  onClick={() => void handleApplyFix(file.path, finding, key)}
                                >
                                  {fixingKey === key ? (
                                    <Loader2 size={12} className="animate-spin" />
                                  ) : (
                                    <Wand2 size={12} />
                                  )}
                                  Apply fix
                                </Button>
                              )}
                            </div>
                          </div>
                        </CardContent>
                      </Card>
                    );
                  })}
                </div>
              </div>
            ))}
          </div>
        </ScrollArea>
      )}

      {/* The finding in its document: the file opens at the finding's block, on
          the side it names, with its span marked and the file's other findings
          drawn dimmer. */}
      <FilePreview
        tabID={tabID}
        filePath={documentOpen && documentAt ? documentAt.file.path : null}
        filename={documentAt ? shortPath(documentAt.file.path) : ""}
        tree={previewTree}
        focusBlockID={documentAt?.finding?.block_id ?? null}
        side={documentAt?.finding ? findingSide(documentAt.finding) : "source"}
        highlights={documentHighlights}
        focusNote={documentNote}
        backLabel={t("Back to checks")}
        onClose={() => setDocumentOpen(false)}
      />
    </div>
  );
}
