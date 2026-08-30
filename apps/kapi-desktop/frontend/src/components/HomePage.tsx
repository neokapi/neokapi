import { useState, useEffect, useCallback } from "react";
import {
  Globe,
  Workflow,
  Loader2,
  Plug,
  Settings2,
  ShieldCheck,
  AlertTriangle,
  RefreshCw,
} from "lucide-react";
import { Button, Badge, EmptyState, ActionCard, LocalePill } from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import type {
  KapiProject,
  PluginIssue,
  ProjectStatus,
  ConvergenceReport,
  ConvergePlan,
} from "../types/api";
import { api, type SampleInfo } from "../hooks/useApi";
import { useActiveFilter } from "../context/ActiveFilterContext";
import { CollectionsPanel, type RunFlowHandler } from "./CollectionsPanel";
import { ConvergenceHero } from "./ConvergenceHero";
import { ProjectStanding, type ProjectPointsResult } from "./ProjectStanding";

export interface HomePageProps {
  project: KapiProject;
  displayName: string;
  tabID?: string;
  /** Persist project edits made on the merged collection surface. */
  onUpdate?: (project: KapiProject) => void;
  onRunFlow?: RunFlowHandler;
  onNavigate: (view: string) => void;
  /** Open the Review surface narrowed to a (collection, locale) scope. */
  onOpenReview?: (scope?: { collection?: string; locale?: string }) => void;
  /** When false, plugin requirements are unmet — show warning banner. */
  pluginsResolved?: boolean;
  /** Details of unsatisfied plugin requirements. */
  pluginIssues?: PluginIssue[];
  /** Pre-loaded status for Storybook/tests — skips api.getProjectStatus(). */
  status?: ProjectStatus;
  /** Pre-loaded convergence for Storybook/tests — skips api.getConvergence(). */
  convergence?: ConvergenceReport;
  /** Pre-loaded pre-flight plan for Storybook/tests — skips api.getConvergePlan(). */
  plan?: ConvergePlan;
  /** Launch the convergence run (Bring up to date → runner passes view). */
  onBringUpToDate?: () => void;
  /** Refresh this sample to the version bundled with the current kapi. */
  onResetSample?: () => void;
  /** Pre-loaded sample info for Storybook — skips api.getSampleInfo(). */
  sampleInfo?: SampleInfo;
  /** Pre-loaded formats for Storybook — forwarded to CollectionsPanel. */
  formatList?: import("../types/api").FormatInfo[];
  /** Pre-loaded base path for Storybook — forwarded to CollectionsPanel. */
  basePath?: string;
  /** Pre-loaded point map for Storybook/tests — skips ProjectPoints. */
  points?: ProjectPointsResult;
  /** Pre-loaded venue for Storybook/tests — skips GetProjectServer. */
  server?: import("../types/api").ProjectServer;
  /** Open Context standing at a point on the map. */
  onOpenPoint?: (pin: { coordinate?: string; collection?: string }) => void;
}

export function HomePage({
  project,
  displayName,
  tabID,
  onUpdate,
  onRunFlow,
  onNavigate,
  onOpenReview,
  pluginsResolved,
  pluginIssues,
  status,
  convergence,
  plan,
  onBringUpToDate,
  onResetSample,
  sampleInfo: propSampleInfo,
  formatList,
  basePath,
  points,
  server,
  onOpenPoint,
}: HomePageProps) {
  const { active: activeFilter } = useActiveFilter();
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

  const defaults = project.defaults ?? {};
  const plugins = project.plugins ?? {};
  const flowCount = Object.keys(project.flows ?? {}).length;
  // When the active filter narrows to specific languages, only those target
  // pills keep their colour; the rest render grey so the scope reads at a glance.
  const filterLangs = activeFilter?.languages ?? [];

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
                <LocalePill
                  key={String(l)}
                  locale={String(l)}
                  muted={filterLangs.length > 0 && !filterLangs.includes(String(l))}
                />
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

      {/* The graph first: what the project stands at on both axes, and the
          coordinate points its content sits at. The hero's verb follows. */}
      {tabID && (
        <ProjectStanding
          tabID={tabID}
          project={project}
          displayName={displayName}
          status={status}
          points={points}
          server={server}
          onOpenPoint={onOpenPoint}
        />
      )}

      {/* Convergence hero — the primary verb of the home (issue #1078 C4):
          drift summary + Bring up to date + the pre-flight Plan… dialog. */}
      {tabID && (
        <ConvergenceHero
          tabID={tabID}
          onBringUpToDate={onBringUpToDate}
          convergence={convergence}
          plan={plan}
          onOpenSettings={onNavigate}
        />
      )}

      {/* Quick actions — the Content card is gone; the page is content now. */}
      <div className="mb-8 grid grid-cols-1 gap-3 sm:grid-cols-3">
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
          icon={<Settings2 size={16} />}
          title="Settings"
          description="Languages, plugins, processing"
          onClick={() => onNavigate("project-settings")}
        />
      </div>

      {/* Collections — the merged spine: stats, files, patterns, coverage, and
          flow-running (per collection, across a selection, or across all). */}
      {tabID && (
        <CollectionsPanel
          project={project}
          onUpdate={onUpdate ?? (() => {})}
          tabID={tabID}
          flows={project.flows}
          onRunFlow={onRunFlow}
          onOpenReview={onOpenReview}
          formatList={formatList}
          basePath={basePath}
          status={status}
          convergence={convergence}
        />
      )}

      {/* No flows yet — nudge toward authoring one in the Flows library. */}
      {flowCount === 0 && (
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
