import { useCallback, useEffect, useMemo, useState } from "react";
import {
  ShieldCheck,
  ShieldAlert,
  Play,
  Loader2,
  Wand2,
  FileText,
  CheckCircle2,
} from "lucide-react";
import { Button, Badge, Card, CardContent, PageHeader, ScrollArea } from "@neokapi/ui-primitives";
import { api } from "../hooks/useApi";
import { useError } from "./ErrorBanner";
import { useActiveFilter } from "../context/ActiveFilterContext";
import type { CheckRunResult, DesktopFinding } from "../types/api";

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
}

/** Map a finding severity to a Badge variant + supplementary class. */
function severityBadge(severity: string): {
  variant: "destructive" | "outline" | "secondary";
  className?: string;
  label: string;
} {
  switch (severity) {
    case "critical":
      return { variant: "destructive", label: "Critical" };
    case "major":
      return {
        variant: "outline",
        className: "border-amber-500/40 bg-amber-500/10 text-amber-600 dark:text-amber-400",
        label: "Major",
      };
    case "minor":
      return {
        variant: "outline",
        className: "border-amber-500/30 bg-amber-500/5 text-amber-600/90 dark:text-amber-400/90",
        label: "Minor",
      };
    default:
      return { variant: "secondary", className: "text-muted-foreground", label: "Info" };
  }
}

function shortPath(p: string): string {
  const parts = p.split(/[\\/]/);
  return parts.slice(-2).join("/") || p;
}

export function ChecksPanel({
  tabID,
  result: propResult,
  forceLoading = false,
  onApplyFix,
}: ChecksPanelProps) {
  const { showError } = useError();
  const { active: activeFilter } = useActiveFilter();
  const [result, setResult] = useState<CheckRunResult | null>(propResult ?? null);
  const [loading, setLoading] = useState(forceLoading);
  const [fixingKey, setFixingKey] = useState<string | null>(null);

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
      setResult(res ?? { pass: true, score: 100, files: [] });
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

  const totalFindings = useMemo(
    () => (result?.files ?? []).reduce((n, f) => n + f.findings.length, 0),
    [result],
  );

  const filesWithFindings = useMemo(
    () => (result?.files ?? []).filter((f) => f.findings.length > 0),
    [result],
  );

  return (
    <div className="p-6">
      <PageHeader
        title="Checks"
        subtitle="Run content checks like tests over your project — terminology, placeholders, and brand vocabulary. Scope which files and languages to check with the menu-bar filter."
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
              {result.pass ? (
                <ShieldCheck size={20} className="text-emerald-500" />
              ) : (
                <ShieldAlert size={20} className="text-destructive" />
              )}
              <div>
                <div className="text-sm font-semibold">{result.pass ? "Passing" : "Failing"}</div>
                <div className="text-xs text-muted-foreground">
                  {totalFindings === 0
                    ? "No findings"
                    : `${totalFindings} finding${totalFindings === 1 ? "" : "s"} across ${filesWithFindings.length} file${filesWithFindings.length === 1 ? "" : "s"}`}
                </div>
              </div>
            </div>
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
          </CardContent>
        </Card>
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
      {result && !loading && totalFindings === 0 && (
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
                </div>
                <div className="space-y-2">
                  {file.findings.map((finding, idx) => {
                    const sev = severityBadge(finding.severity);
                    const key = `${file.path}#${finding.block_id ?? ""}#${idx}`;
                    return (
                      <Card key={key}>
                        <CardContent className="p-3">
                          <div className="flex items-start justify-between gap-3">
                            <div className="min-w-0 flex-1">
                              <div className="mb-1 flex flex-wrap items-center gap-1.5">
                                <Badge variant={sev.variant} className={sev.className}>
                                  {sev.label}
                                </Badge>
                                <Badge
                                  variant="outline"
                                  className="text-muted-foreground"
                                  translate="no"
                                >
                                  {finding.category}
                                </Badge>
                              </div>
                              <p className="text-sm">{finding.message}</p>
                              {finding.original_text && (
                                <p className="mt-1 text-xs text-muted-foreground">
                                  Found:{" "}
                                  <code
                                    className="rounded bg-muted px-1 py-0.5 text-[11px]"
                                    translate="no"
                                  >
                                    {finding.original_text}
                                  </code>
                                </p>
                              )}
                              {finding.suggestion && (
                                <p className="mt-1 text-xs text-muted-foreground">
                                  <span className="text-muted-foreground/70">{"↳ "}</span>
                                  {finding.suggestion}
                                </p>
                              )}
                            </div>
                            {finding.fixable && (
                              <Button
                                variant="outline"
                                size="sm"
                                className="shrink-0"
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
    </div>
  );
}
