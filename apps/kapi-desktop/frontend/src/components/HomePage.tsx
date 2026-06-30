import { useState, useEffect, useCallback } from "react";
import {
  Play,
  Globe,
  Workflow,
  Loader2,
  Plug,
  Settings2,
  Wrench,
  ShieldCheck,
  AlertTriangle,
  RefreshCw,
} from "lucide-react";
import { Button, Badge, Card, EmptyState, ActionCard, LocalePill } from "@neokapi/ui-primitives";
import { t } from "@neokapi/kapi-react/runtime";
import type { KapiProject, FlowSpec, FlowInfo, PluginIssue, ProjectStatus } from "../types/api";
import { api, type SampleInfo } from "../hooks/useApi";
import { useJobFeed } from "../context/JobFeedContext";
import { CollectionsPanel } from "./CollectionsPanel";

export interface HomePageProps {
  project: KapiProject;
  displayName: string;
  tabID?: string;
  /** Persist project edits made on the merged collection surface. */
  onUpdate?: (project: KapiProject) => void;
  onRunFlow?: (flowName: string, flow: FlowSpec) => void;
  onNavigate: (view: string) => void;
  /** When false, plugin requirements are unmet — show warning banner. */
  pluginsResolved?: boolean;
  /** Details of unsatisfied plugin requirements. */
  pluginIssues?: PluginIssue[];
  /** Pre-loaded status for Storybook/tests — skips api.getProjectStatus(). */
  status?: ProjectStatus;
  /** Refresh this sample to the version bundled with the current kapi. */
  onResetSample?: () => void;
  /** Pre-loaded sample info for Storybook — skips api.getSampleInfo(). */
  sampleInfo?: SampleInfo;
  /** Pre-loaded formats for Storybook — forwarded to CollectionsPanel. */
  formatList?: import("../types/api").FormatInfo[];
  /** Pre-loaded base path for Storybook — forwarded to CollectionsPanel. */
  basePath?: string;
}

export function HomePage({
  project,
  displayName,
  tabID,
  onUpdate,
  onRunFlow,
  onNavigate,
  pluginsResolved,
  pluginIssues,
  status,
  onResetSample,
  sampleInfo: propSampleInfo,
  formatList,
  basePath,
}: HomePageProps) {
  const { hasActive, activeJob } = useJobFeed();
  const [flowValidation, setFlowValidation] = useState<Record<string, FlowInfo>>({});
  const [installingPlugin, setInstallingPlugin] = useState<string | null>(null);
  const [sampleInfo, setSampleInfo] = useState<SampleInfo | null>(propSampleInfo ?? null);
  // "Keep current" dismisses the upgrade prompt for this session.
  const [sampleDismissed, setSampleDismissed] = useState(false);

  // Detect whether this project is an out-of-date sample so we can offer a reset.
  useEffect(() => {
    if (!tabID || propSampleInfo) return;
    void api
      .getSampleInfo(tabID)
      .then((info) => {
        if (info) setSampleInfo(info);
      })
      .catch(() => {});
  }, [tabID, propSampleInfo]);

  // Acknowledge the on-disk revision so the prompt stays dismissed across reopens.
  const handleKeepSample = useCallback(() => {
    setSampleDismissed(true);
    if (tabID) void api.acknowledgeSampleRevision(tabID);
  }, [tabID]);

  // Install a missing project plugin directly from the banner. The backend
  // emits plugins-changed, which re-checks the project and clears the banner.
  const handleInstallPlugin = useCallback((plugin: string) => {
    setInstallingPlugin(plugin);
    void api.installPlugin(plugin);
  }, []);

  // Fetch flow validation on mount / project change.
  useEffect(() => {
    if (!tabID) return;
    void api.listFlows(tabID).then((flows) => {
      if (!flows) return;
      const map: Record<string, FlowInfo> = {};
      for (const f of flows) {
        map[f.name] = f;
      }
      setFlowValidation(map);
    });
  }, [tabID, project.flows]);

  const defaults = project.defaults ?? {};
  const plugins = project.plugins ?? {};
  const flowNames = Object.keys(project.flows ?? {});
  const hasContent = (project.content?.length ?? 0) > 0;
  const flowCount = flowNames.length;

  const handleRunFlow = (name: string) => {
    const spec = project.flows?.[name];
    if (!spec || !onRunFlow) return;
    onRunFlow(name, spec);
  };

  return (
    <div className="p-6">
      {/* Header */}
      <div className="mb-6">
        <h1 className="text-xl font-semibold">{displayName}</h1>
        <div className="mt-2 flex flex-wrap items-center gap-3 text-sm text-muted-foreground">
          <span className="flex flex-wrap items-center gap-1.5">
            <Globe size={14} />
            {defaults.source_language ? (
              <LocalePill locale={String(defaults.source_language)} />
            ) : (
              <span>{t("No source")}</span>
            )}
            <span>&rarr;</span>
            {defaults.target_languages?.length ? (
              defaults.target_languages.map((l) => (
                <LocalePill key={String(l)} locale={String(l)} />
              ))
            ) : (
              <span>{t("No targets")}</span>
            )}
          </span>
          {project.preset && (
            <Badge variant="secondary" className="text-xs">
              {project.preset}
            </Badge>
          )}
          {Object.keys(plugins).length > 0 &&
            Object.entries(plugins).map(([name, spec]) => (
              <span key={name} className="flex items-center gap-1">
                <Plug size={10} />
                <span className="text-xs">
                  {name}
                  {spec.framework_version && (
                    <span className="text-muted-foreground/60"> {spec.framework_version}</span>
                  )}
                </span>
              </span>
            ))}
        </div>
      </div>

      {/* Plugin issues banner */}
      {pluginsResolved === false && pluginIssues && pluginIssues.length > 0 && (
        <div className="mb-6 rounded-lg border border-amber-500/30 bg-amber-500/5 p-4">
          <div className="flex items-start gap-3">
            <AlertTriangle size={16} className="mt-0.5 shrink-0 text-amber-500" />
            <div className="flex-1">
              <p className="text-sm font-medium">Plugin requirements not met</p>
              <p className="mt-1 text-xs text-muted-foreground">
                This project requires plugins that are not installed or have incompatible versions.
                Content and flow features are disabled until this is resolved.
              </p>
              <ul className="mt-2 space-y-1">
                {pluginIssues.map((issue) => (
                  <li key={issue.plugin} className="flex items-center gap-2 text-xs">
                    <Badge variant="outline" className="text-[10px]">
                      {issue.plugin}
                    </Badge>
                    {issue.type === "missing" ? (
                      <>
                        <span className="text-muted-foreground">not installed</span>
                        <Button
                          size="xs"
                          variant="outline"
                          className="ml-auto"
                          onClick={() => handleInstallPlugin(issue.plugin)}
                          disabled={installingPlugin === issue.plugin}
                        >
                          {installingPlugin === issue.plugin ? (
                            <Loader2 size={11} className="animate-spin" />
                          ) : (
                            <Plug size={11} />
                          )}
                          {t("Install")}
                        </Button>
                      </>
                    ) : (
                      <span className="text-muted-foreground">
                        requires {issue.required}, installed {issue.installed_version}
                      </span>
                    )}
                  </li>
                ))}
              </ul>
              <div className="mt-3 flex gap-2">
                <Button size="sm" variant="outline" onClick={() => onNavigate("project-settings")}>
                  <Settings2 size={12} />
                  Edit Plugin Settings
                </Button>
                <Button size="sm" variant="outline" onClick={() => onNavigate("app-settings")}>
                  <Plug size={12} />
                  Manage Plugins
                </Button>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Sample upgrade banner — a newer revision of this sample ships with kapi */}
      {sampleInfo?.upgrade_available && !sampleDismissed && (
        <div className="mb-6 rounded-lg border border-primary/30 bg-primary/5 p-4">
          <div className="flex items-start gap-3">
            <RefreshCw size={16} className="mt-0.5 shrink-0 text-primary" />
            <div className="flex-1">
              <p className="text-sm font-medium">
                {t("A newer version of this sample is available")}
              </p>
              <p className="mt-1 text-xs text-muted-foreground">
                {t(
                  "This sample was created by an earlier version of kapi. Reset it to get the latest content and configuration — your current copy is backed up first.",
                )}
              </p>
              <div className="mt-3 flex gap-2">
                <Button size="sm" onClick={() => onResetSample?.()}>
                  <RefreshCw size={12} />
                  {t("Reset to latest")}
                </Button>
                <Button size="sm" variant="outline" onClick={handleKeepSample}>
                  {t("Keep current")}
                </Button>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Quick actions — the Content card is gone; the page is content now. */}
      <div className="mb-8 grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <ActionCard
          icon={<ShieldCheck size={16} />}
          title="Check"
          description="Verify structure, brand, and placeholders"
          onClick={() => onNavigate("checks")}
        />
        <ActionCard
          icon={<Workflow size={16} />}
          title="Flows"
          description={
            flowCount > 0
              ? t("{count} flow(s) defined", { count: flowCount })
              : t("Build your first flow")
          }
          onClick={() => onNavigate("flows")}
        />
        <ActionCard
          icon={<Wrench size={16} />}
          title="Tools"
          description="Run individual tools on files"
          onClick={() => onNavigate("tools")}
        />
        <ActionCard
          icon={<Settings2 size={16} />}
          title="Settings"
          description="Languages, plugins, processing"
          onClick={() => onNavigate("project-settings")}
        />
      </div>

      {/* Collections — the merged spine: stats, files, patterns, coverage. */}
      {tabID && (
        <CollectionsPanel
          project={project}
          onUpdate={onUpdate ?? (() => {})}
          tabID={tabID}
          formatList={formatList}
          basePath={basePath}
          status={status}
        />
      )}

      {/* Run flows */}
      {flowNames.length > 0 && (
        <section>
          <h2 className="mb-3 flex items-center gap-2 text-sm font-semibold uppercase tracking-wider text-muted-foreground">
            <Workflow size={14} />
            Run Flows
          </h2>
          <div className="space-y-2">
            {flowNames.map((name) => {
              const spec = project.flows?.[name];
              if (!spec) return null;
              const validation = flowValidation[name];
              const isValid = validation?.valid !== false;
              const flowIssues = validation?.issues ?? [];
              const canRun = isValid && hasContent && !hasActive;
              const runTitle = !isValid
                ? `Cannot run: ${flowIssues.map((i) => i.message).join("; ")}`
                : !hasContent
                  ? "Configure content patterns first"
                  : undefined;

              return (
                <Card
                  key={name}
                  className={`flex flex-row items-center gap-3 p-3 ${!isValid ? "border-amber-500/30 bg-amber-500/5" : ""}`}
                >
                  <div className="flex-1">
                    <div className="flex items-center gap-1.5">
                      <span className="text-sm font-medium">{name}</span>
                      {!isValid && <AlertTriangle size={12} className="text-amber-500" />}
                    </div>
                    <div className="mt-0.5 flex items-center gap-1.5 text-xs text-muted-foreground">
                      {spec.steps.map((step, i) => {
                        const stepHasIssue = flowIssues.some((issue) => issue.tool === step.tool);
                        return (
                          <span key={i} className="flex items-center gap-1">
                            {i > 0 && <span>&rarr;</span>}
                            <Badge
                              variant={stepHasIssue ? "destructive" : "secondary"}
                              className={stepHasIssue ? "line-through opacity-70" : ""}
                            >
                              {step.tool}
                            </Badge>
                          </span>
                        );
                      })}
                    </div>
                    {flowIssues.length > 0 && (
                      <div className="mt-1 text-[10px] text-amber-600 dark:text-amber-400">
                        {flowIssues.map((issue, i) => (
                          <div key={i}>{issue.message}</div>
                        ))}
                      </div>
                    )}
                  </div>
                  <Button
                    size="sm"
                    onClick={() => handleRunFlow(name)}
                    disabled={!canRun}
                    aria-label={t("Run flow {name}", { name })}
                    title={runTitle}
                  >
                    {activeJob?.flowName === name ? (
                      <Loader2 size={12} className="animate-spin" />
                    ) : (
                      <Play size={12} />
                    )}
                    Run
                  </Button>
                </Card>
              );
            })}
          </div>
        </section>
      )}

      {/* Empty state */}
      {flowNames.length === 0 && (
        <EmptyState
          icon={<Workflow size={24} className="text-muted-foreground/50" />}
          title="No flows defined yet."
          action={
            <Button
              variant="link"
              size="sm"
              onClick={() => onNavigate("flows")}
              className="h-auto px-0"
            >
              Create your first flow
            </Button>
          }
        />
      )}
    </div>
  );
}
