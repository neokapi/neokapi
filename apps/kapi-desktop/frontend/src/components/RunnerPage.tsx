import { useState, useCallback, useEffect, useMemo, useRef } from "react";
import {
  Play,
  Square,
  CheckCircle2,
  XCircle,
  Loader2,
  FileText,
  AlertTriangle,
} from "lucide-react";
import {
  Button,
  Card,
  CardHeader,
  CardTitle,
  CardContent,
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuCheckboxItem,
  Label,
  Input,
  ScrollArea,
  PageContainer,
  PageHeader,
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import type { AISetupNeed, FlowSpec, KapiProject } from "../types/api";
import { useQuery } from "@tanstack/react-query";
import { api } from "../hooks/useApi";
import { qk } from "../lib/queryKeys";
import { useJobFeed, type RunEvent } from "../context/JobFeedContext";
import { useActiveFilter } from "../context/ActiveFilterContext";
import { filterFiles, filterLanguages } from "../lib/filter";
import { PipelineProgress } from "./PipelineProgress";
import { AIModelPromptDialog } from "./AIModelPromptDialog";
import { AICredentialMissingDialog } from "./AICredentialMissingDialog";
import { useExecTrustGate } from "./ExecTrustDialog";
import { ConvergeRunView } from "./ConvergeRunView";

type RunState = "idle" | "running" | "complete" | "error" | "canceled";

export { type RunEvent };

export interface RunnerPageProps {
  tabID: string;
  flowName: string;
  flow?: FlowSpec;
  onClose: () => void;
  /** When set, the project is used to resolve content and target languages. */
  project?: KapiProject;
  /** When true, automatically resolve content and run for all target languages on mount. */
  autoRun?: boolean;
  /** Marks an automatic run request as consumed across navigation. */
  onLaunched?: () => void;
  /**
   * Explicit input paths to run, scoping the flow to a single collection's
   * files (the per-collection "Run" on the project home). When set, these win
   * over the matched-content ∩ active-filter resolution; target languages still
   * follow the active filter. Empty/undefined ⇒ resolve the whole project.
   */
  scopePaths?: string[];
  /** Human label for the scope (e.g. the collection name), shown in the header. */
  scopeLabel?: string;
  /**
   * Convergence mode (Catch up): the run goes through the shared
   * `kapi up` engine and the page renders passes (extracted / produced /
   * failing checks / waiting for review) instead of raw flow logs. Custom-flow runs keep
   * the classic runner view.
   */
  converge?: boolean;
  jobID?: string;
  /** Open the Review page filtered to a (collection, locale) scope waiting for review. */
  onOpenReview?: (scope: { collection?: string; locale?: string }) => void;
  /** Navigate to a view; used to open App Settings on Credentials. */
  onNavigate?: (view: string) => void;
}

export function RunnerPage({
  tabID,
  flowName,
  flow,
  onClose,
  project,
  autoRun,
  onLaunched,
  scopePaths,
  scopeLabel,
  converge,
  jobID,
  onOpenReview,
  onNavigate,
}: RunnerPageProps) {
  const { jobs, launchJob, hasActive } = useJobFeed();
  const { active: activeFilter } = useActiveFilter();
  const job = jobs.find(
    (item) =>
      item.tabID === tabID &&
      (jobID ? item.id === jobID : converge ? item.converge : item.flowName === flowName),
  );

  // Derive state from job feed.
  const state: RunState = job?.status ?? "idle";
  const events: RunEvent[] = job?.events ?? [];
  const progress = job?.progress ?? { current: 0, total: 0 };
  const stepSnapshots = job?.stepSnapshots ?? [];
  const error = job?.error ?? null;

  const [inputFiles, setInputFiles] = useState<string[]>([]);
  const [availableFiles, setAvailableFiles] = useState<string[]>([]);
  const [targetLang, setTargetLang] = useState("");

  // Unknown tools can require a target language.
  const toolsQuery = useQuery({
    queryKey: qk.projectTools(tabID),
    queryFn: () => api.listProjectTools(tabID),
  });
  const needsTargetLanguage = useMemo(() => {
    const steps = flow?.steps ?? [];
    if (steps.length === 0 || !toolsQuery.data) return true;
    const cardinality = new Map(toolsQuery.data.map((tool) => [tool.name, tool.cardinality]));
    return steps.some((step) => cardinality.get(step.tool) !== "monolingual");
  }, [flow, toolsQuery.data]);
  const [projectTargets, setProjectTargets] = useState<string[]>([]);
  const autoRunStarted = useRef(false);

  const projectName = project?.name || undefined;

  const [launchError, setLaunchError] = useState<string | null>(null);
  // Stashed launch args while the "pick an AI model" prompt is open.
  const [modelPrompt, setModelPrompt] = useState<{
    paths: string[];
    targets: string[];
    converge?: boolean;
  } | null>(null);
  // The recipe names a provider whose key is missing: no model choice helps.
  const [missingKey, setMissingKey] = useState<AISetupNeed | null>(null);
  // The project's commands are asked about before each run, so one changed
  // since the last answer is asked about again rather than run.
  const execTrust = useExecTrustGate(tabID);

  const runNow = useCallback(
    async (paths: string[], targets: string[]) => {
      try {
        await launchJob(
          { tabID, flowName, projectName, targetLangs: targets, fileCount: paths.length },
          () => api.runFlow(tabID, flowName, paths, targets),
        );
      } catch (err) {
        setLaunchError(String(err));
      }
    },
    [tabID, flowName, projectName, launchJob],
  );

  // launchFlow prompts for a default AI model first when the flow needs one and
  // none is configured (and credentials don't auto-resolve), then runs.
  const launchFlow = useCallback(
    async (paths: string[], targets: string[]) => {
      setLaunchError(null);
      if (!(await execTrust.gate())) return;
      const need = await api.checkAISetup(tabID, flowName);
      if (need?.kind === "missing_credential") {
        setMissingKey(need);
        return;
      }
      if (need?.kind === "choose_model") {
        setModelPrompt({ paths, targets });
        return;
      }
      await runNow(paths, targets);
    },
    [tabID, flowName, runNow, execTrust],
  );

  const convergeNow = useCallback(async () => {
    try {
      await launchJob({ flowName, tabID, projectName, converge: true }, () =>
        api.bringUpToDate(tabID),
      );
    } catch (err) {
      setLaunchError(String(err));
    }
  }, [tabID, flowName, projectName, launchJob]);

  const launchConverge = useCallback(async () => {
    setLaunchError(null);
    // Catch up runs convergence (`kapi up`), which needs a model or a key only
    // when it has work for the AI step: asked with no flow name.
    if (!(await execTrust.gate())) return;
    const need = await api.checkAISetup(tabID, "");
    if (need?.kind === "missing_credential") {
      setMissingKey(need);
      return;
    }
    if (need?.kind === "choose_model") {
      setModelPrompt({ paths: [], targets: [], converge: true });
      return;
    }
    await convergeNow();
  }, [tabID, convergeNow, execTrust]);

  // Auto-run (convergence): launch Catch up once per request.
  useEffect(() => {
    if (!converge || !autoRun || autoRunStarted.current) return;
    autoRunStarted.current = true;
    onLaunched?.();
    void launchConverge();
  }, [converge, autoRun, launchConverge, onLaunched]);

  // Auto-run: resolve content and execute for all target languages.
  useEffect(() => {
    if (converge || !autoRun || !project || autoRunStarted.current) return;
    const targets = filterLanguages(project.defaults?.target_languages ?? [], activeFilter);
    if (targets.length === 0 && toolsQuery.isPending) return;
    autoRunStarted.current = true;
    // Mark the run request consumed so a remount (navigating back to the runner
    // while this flow runs) doesn't relaunch and duplicate it.
    onLaunched?.();

    if (targets.length === 0 && needsTargetLanguage) {
      setLaunchError(t("Choose a target language before running this flow."));
      return;
    }

    void (async () => {
      // An explicit per-collection scope wins over the matched-content ∩ filter
      // resolution; otherwise resolve the whole project narrowed by the filter.
      let paths: string[];
      if (scopePaths && scopePaths.length > 0) {
        paths = scopePaths;
      } else {
        const matches = await api.matchContent(tabID);
        paths = filterFiles(matches ?? [], activeFilter).map((m) => m.path);
      }
      if (paths.length === 0) {
        setLaunchError(t("No project files match this run."));
        return;
      }

      setInputFiles(paths);
      await launchFlow(paths, needsTargetLanguage ? targets : []);
    })().catch((error: unknown) => setLaunchError(String(error)));
  }, [
    converge,
    autoRun,
    project,
    tabID,
    launchFlow,
    onLaunched,
    activeFilter,
    scopePaths,
    toolsQuery.isPending,
    needsTargetLanguage,
  ]);

  useEffect(() => {
    if (converge || autoRun || !project) return;
    let current = true;
    const targets = filterLanguages(project.defaults?.target_languages ?? [], activeFilter);
    setProjectTargets(targets);
    setTargetLang((value) => (targets.includes(value) ? value : (targets[0] ?? "")));
    setInputFiles([]);
    setAvailableFiles([]);
    void api
      .matchContent(tabID)
      .then((matches) => {
        if (!current) return;
        const paths = filterFiles(matches ?? [], activeFilter).map((file) => file.path);
        setAvailableFiles(paths);
        setInputFiles(paths);
      })
      .catch((error: unknown) => {
        if (current) setLaunchError(String(error));
      });
    return () => {
      current = false;
    };
  }, [converge, autoRun, project, tabID, activeFilter]);

  // Manual run (single language).
  const handleRun = useCallback(async () => {
    if (inputFiles.length === 0) return;
    if (needsTargetLanguage && !targetLang) return;
    await launchFlow(inputFiles, needsTargetLanguage ? [targetLang] : []);
  }, [inputFiles, targetLang, needsTargetLanguage, launchFlow]);

  const handleCancel = useCallback(async () => {
    try {
      await api.cancelProjectRun(tabID);
    } catch (error) {
      setLaunchError(String(error));
    }
  }, [tabID]);

  const stateIcon = {
    idle: null,
    running: <Loader2 size={16} className="animate-spin text-primary" />,
    complete: <CheckCircle2 size={16} className="text-green-500" />,
    error: <XCircle size={16} className="text-destructive" />,
    canceled: <XCircle size={16} className="text-muted-foreground" />,
  };

  return (
    <PageContainer width="wide">
      <PageHeader
        title={
          converge
            ? flowName && flowName !== "up"
              ? t("Catch up · {name}", { name: flowName })
              : t("Catch up")
            : scopeLabel
              ? t("Run: {name} · {scope}", { name: flowName, scope: scopeLabel })
              : t("Run: {name}", { name: flowName })
        }
        className="mb-4"
        actions={
          <div className="flex items-center gap-2">
            {stateIcon[state]}
            <Button variant="outline" size="sm" onClick={onClose} aria-label="Back">
              {state === "complete" || state === "error" ? t("Done") : t("Back")}
            </Button>
          </div>
        }
      />

      {flow && !converge && (
        <Card className="mb-4">
          <CardHeader className="px-4">
            <CardTitle className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
              Pipeline
            </CardTitle>
          </CardHeader>
          <CardContent className="px-4">
            <PipelineProgress steps={flow.steps} snapshots={stepSnapshots} runState={state} />
          </CardContent>
        </Card>
      )}

      {/* Manual configuration (only in idle state, non-autoRun) */}
      {state === "idle" && !autoRun && !converge && (
        <div className="mb-4 space-y-3">
          <div>
            <Label htmlFor="runner-files" className="mb-1 block">
              Input Files
            </Label>
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  id="runner-files"
                  disabled={availableFiles.length === 0}
                  variant="outline"
                  className="flex items-center gap-2 border-dashed text-muted-foreground hover:border-primary hover:text-primary"
                  aria-label="Select input files for flow"
                >
                  <FileText size={16} />
                  {inputFiles.length > 0
                    ? t("{count, plural, one {# file} other {# files}} selected", {
                        count: inputFiles.length,
                      })
                    : t("Select files…")}
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="start" className="max-h-72 overflow-auto">
                {availableFiles.map((path) => (
                  <DropdownMenuCheckboxItem
                    key={path}
                    checked={inputFiles.includes(path)}
                    onSelect={(event) => event.preventDefault()}
                    onCheckedChange={(checked) =>
                      setInputFiles((files) =>
                        checked ? [...files, path] : files.filter((file) => file !== path),
                      )
                    }
                  >
                    {path}
                  </DropdownMenuCheckboxItem>
                ))}
              </DropdownMenuContent>
            </DropdownMenu>
            {project && inputFiles.length > 0 && (
              <p className="mt-1 text-xs text-muted-foreground">{t("From project content")}</p>
            )}
          </div>
          {needsTargetLanguage && (
            <div>
              <Label htmlFor="runner-target-lang" className="mb-1 block">
                Target Language
              </Label>
              {projectTargets.length > 0 ? (
                <Select value={targetLang} onValueChange={setTargetLang}>
                  <SelectTrigger
                    id="runner-target-lang"
                    className="w-48"
                    aria-label="Target language"
                  >
                    <SelectValue placeholder={t("Select a language")} />
                  </SelectTrigger>
                  <SelectContent>
                    {projectTargets.map((l) => (
                      <SelectItem key={l} value={l} translate="no">
                        {l}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              ) : (
                <Input
                  id="runner-target-lang"
                  type="text"
                  value={targetLang}
                  onChange={(e) => setTargetLang(e.target.value)}
                  placeholder="e.g. fr"
                  className="w-48"
                />
              )}
            </div>
          )}
        </div>
      )}

      {/* Controls */}
      <div className="mb-6 flex gap-2">
        {state === "idle" && !autoRun && !converge && (
          <Button
            onClick={handleRun}
            disabled={(needsTargetLanguage && !targetLang) || inputFiles.length === 0 || hasActive}
            aria-label="Run flow"
          >
            {hasActive ? (
              <>
                <Loader2 size={14} className="animate-spin" />
                Running...
              </>
            ) : (
              <>
                <Play size={14} />
                Run Flow
              </>
            )}
          </Button>
        )}
        {state === "running" && (
          <Button
            variant="destructive"
            onClick={handleCancel}
            className="border border-destructive bg-transparent text-destructive hover:bg-destructive/10"
            aria-label="Cancel flow execution"
          >
            <Square size={14} />
            Cancel
          </Button>
        )}
      </div>

      {/* Errors */}
      {launchError && (
        <div
          className="mb-4 flex items-center gap-2 rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-sm text-destructive"
          role="alert"
        >
          <AlertTriangle size={14} className="shrink-0" />
          {launchError}
        </div>
      )}
      {error && (
        <p className="mb-4 text-sm text-destructive" role="alert">
          {error}
        </p>
      )}

      {/* Progress bar */}
      {(state === "running" || state === "complete") && progress.total > 0 && (
        <div className="mb-4">
          <div className="mb-1 flex justify-between text-xs text-muted-foreground">
            <span>
              {progress.current} / {progress.total} files
            </span>
            <span>{Math.round((progress.current / progress.total) * 100)}%</span>
          </div>
          <div
            className="h-2 overflow-hidden rounded-full bg-accent"
            role="progressbar"
            aria-valuenow={progress.current}
            aria-valuemin={0}
            aria-valuemax={progress.total}
          >
            <div
              className="h-full rounded-full bg-primary transition-[width] duration-300"
              style={{
                width: `${(progress.current / progress.total) * 100}%`,
              }}
            />
          </div>
        </div>
      )}

      {converge && (
        <ConvergeRunView
          events={events}
          running={state === "running"}
          canceled={state === "canceled"}
          onOpenReview={onOpenReview}
        />
      )}

      {/* Event log */}
      {!converge && events.length > 0 && (
        <Card>
          <CardHeader className="px-4">
            <CardTitle className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
              Output
            </CardTitle>
          </CardHeader>
          <CardContent className="px-4">
            <div className="h-64">
              <ScrollArea className="h-full">
                {events.map((event, i) => (
                  <div
                    key={i}
                    className={`py-0.5 font-mono text-xs ${
                      event.type === "error"
                        ? "text-destructive"
                        : event.type === "complete"
                          ? "text-green-500"
                          : "text-muted-foreground"
                    }`}
                  >
                    {event.message || event.file_path || event.type}
                  </div>
                ))}
              </ScrollArea>
            </div>
          </CardContent>
        </Card>
      )}

      <AIModelPromptDialog
        open={modelPrompt !== null}
        onResolved={() => {
          const pending = modelPrompt;
          setModelPrompt(null);
          if (!pending) return;
          if (pending.converge) void convergeNow();
          else void runNow(pending.paths, pending.targets);
        }}
        onCancel={() => setModelPrompt(null)}
      />
      {execTrust.dialog}
      <AICredentialMissingDialog
        need={missingKey}
        onAddKey={
          onNavigate
            ? () => {
                setMissingKey(null);
                onNavigate("app-settings:credentials");
              }
            : undefined
        }
        onCancel={() => setMissingKey(null)}
      />
    </PageContainer>
  );
}
