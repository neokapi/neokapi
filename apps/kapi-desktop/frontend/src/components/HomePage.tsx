import { useState, useEffect, useCallback } from "react";
import {
  Play,
  Globe,
  Workflow,
  Loader2,
  Plug,
  FileText,
  Settings2,
  Wrench,
  ShieldCheck,
  AlertTriangle,
  PackageOpen,
  RefreshCw,
  ChevronDown,
  ChevronRight,
} from "lucide-react";
import { Button, Badge, Card, EmptyState, ActionCard, LocalePill } from "@neokapi/ui-primitives";
import { PieChart, Pie, Cell, ResponsiveContainer } from "recharts";
import { t } from "@neokapi/kapi-react/runtime";
import type { KapiProject, FlowSpec, FlowInfo, PluginIssue, ProjectStatus } from "../types/api";
import { isBareEntry, effectiveItems } from "../types/api";
import { api, type SampleInfo } from "../hooks/useApi";
import { useJobFeed } from "../context/JobFeedContext";
import { useActiveFilter } from "../context/ActiveFilterContext";
import { filterLanguages } from "../lib/filter";
import { useWailsEvent } from "../hooks/useWailsEvent";
import { useError } from "./ErrorBanner";

// Donut palette for the per-collection block distribution (theme chart vars).
const CHART_COLORS = [
  "var(--chart-1)",
  "var(--chart-2)",
  "var(--chart-3)",
  "var(--chart-4)",
  "var(--chart-5)",
];

/** A compact one-line coverage bar: locale label · bar · "n / total (pct%)". */
function CoverageBar({
  label,
  translated,
  total,
  pct,
}: {
  label: string;
  translated: number;
  total: number;
  pct: number;
}) {
  return (
    <div className="flex items-center gap-2">
      <span className="w-16 shrink-0 text-xs text-muted-foreground" translate="no">
        {label}
      </span>
      <div className="h-1.5 flex-1 overflow-hidden rounded-full bg-accent">
        <div
          className="h-full rounded-full bg-primary transition-all"
          style={{ width: `${pct}%` }}
        />
      </div>
      <span className="w-24 shrink-0 text-right text-[11px] tabular-nums text-muted-foreground">
        {translated} / {total} ({pct}%)
      </span>
    </div>
  );
}

export interface HomePageProps {
  project: KapiProject;
  displayName: string;
  tabID?: string;
  onRunFlow?: (flowName: string, flow: FlowSpec) => void;
  onNavigate: (view: string) => void;
  /** When false, plugin requirements are unmet — show warning banner. */
  pluginsResolved?: boolean;
  /** Details of unsatisfied plugin requirements. */
  pluginIssues?: PluginIssue[];
  /** Pre-loaded status for Storybook — skips api.getProjectStatus(). */
  status?: ProjectStatus;
  /** Refresh this sample to the version bundled with the current kapi. */
  onResetSample?: () => void;
  /** Pre-loaded sample info for Storybook — skips api.getSampleInfo(). */
  sampleInfo?: SampleInfo;
}

export function HomePage({
  project,
  displayName,
  tabID,
  onRunFlow,
  onNavigate,
  pluginsResolved,
  pluginIssues,
  status: propStatus,
  onResetSample,
  sampleInfo: propSampleInfo,
}: HomePageProps) {
  const { hasActive, activeJob } = useJobFeed();
  const { active: activeFilter } = useActiveFilter();
  const { showError } = useError();
  const [flowValidation, setFlowValidation] = useState<Record<string, FlowInfo>>({});
  const [status, setStatus] = useState<ProjectStatus | null>(propStatus ?? null);
  const [extracting, setExtracting] = useState(false);
  const [installingPlugin, setInstallingPlugin] = useState<string | null>(null);
  const [sampleInfo, setSampleInfo] = useState<SampleInfo | null>(propSampleInfo ?? null);
  // "Keep current" dismisses the upgrade prompt for this session.
  const [sampleDismissed, setSampleDismissed] = useState(false);
  // Collections expanded to their full per-language breakdown (compact by default).
  const [expandedColls, setExpandedColls] = useState<Set<string>>(new Set());

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

  const refreshStatus = useCallback(() => {
    if (!tabID || propStatus) return;
    void api
      .getProjectStatus(tabID)
      .then((s) => {
        if (s) setStatus(s);
      })
      .catch(() => {});
  }, [tabID, propStatus]);

  // Load status on mount / content change.
  useEffect(() => {
    refreshStatus();
  }, [refreshStatus, project.content]);

  // Refresh after an extraction completes (emitted by the backend).
  useWailsEvent("project:extracted", () => refreshStatus());

  const handleExtract = useCallback(async () => {
    if (!tabID) return;
    setExtracting(true);
    try {
      await api.runExtract(tabID);
      refreshStatus();
    } catch (err) {
      showError("Extraction failed", err);
    } finally {
      setExtracting(false);
    }
  }, [tabID, refreshStatus, showError]);

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
  const contentCount = project.content?.length ?? 0;
  const itemCount =
    project.content?.reduce((sum, c) => sum + (isBareEntry(c) ? 1 : effectiveItems(c).length), 0) ??
    0;

  // Coverage rows narrowed by the Active Filter's collections (languages are
  // narrowed per-row below). An empty collection filter shows all.
  const filteredCollections = (status?.collections ?? []).filter(
    (c) => !activeFilter?.collections?.length || activeFilter.collections.includes(c.name),
  );

  // ── Content-overview aggregates ──────────────────────────────────────────
  // Block distribution per collection (the donut) and cross-collection coverage
  // per language (the compact project-wide summary), both honoring the filter.
  const totalBlocks = filteredCollections.reduce((s, c) => s + c.blockCount, 0);
  const distData = filteredCollections
    .map((c, i) => ({
      name: c.name,
      value: c.blockCount,
      fill: CHART_COLORS[i % CHART_COLORS.length],
    }))
    .filter((d) => d.value > 0);
  const projectLangs = Array.from(
    new Set(filteredCollections.flatMap((c) => filterLanguages(c.targetLanguages, activeFilter))),
  );
  const projectCoverage = projectLangs.map((lang) => {
    let translated = 0;
    let total = 0;
    for (const c of filteredCollections) {
      if (!c.targetLanguages.includes(lang)) continue;
      translated += c.coverage?.[lang] ?? 0;
      total += c.blockCount;
    }
    return { lang, translated, total, pct: total > 0 ? Math.round((translated / total) * 100) : 0 };
  });

  const toggleColl = (name: string) =>
    setExpandedColls((prev) => {
      const next = new Set(prev);
      if (next.has(name)) next.delete(name);
      else next.add(name);
      return next;
    });

  // Mean coverage across a collection's filtered languages, for the compact row.
  const collAvgPct = (c: ProjectStatus["collections"][number], langs: string[]) =>
    langs.length === 0 || c.blockCount === 0
      ? 0
      : Math.round(
          (langs.reduce((s, l) => s + (c.coverage?.[l] ?? 0) / c.blockCount, 0) / langs.length) *
            100,
        );

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

      {/* Quick actions */}
      <div className="mb-8 grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-5">
        <ActionCard
          icon={<ShieldCheck size={16} />}
          title="Check"
          description="Verify structure, brand, and placeholders"
          onClick={() => onNavigate("checks")}
        />
        <ActionCard
          icon={<FileText size={16} />}
          title="Content"
          description={
            hasContent
              ? t("{contentCount} collection(s), {itemCount} pattern(s)", {
                  contentCount,
                  itemCount,
                })
              : t("Configure file patterns")
          }
          onClick={() => onNavigate("content")}
        />
        <ActionCard
          icon={<Workflow size={16} />}
          title="Flows"
          description={
            flowNames.length > 0
              ? t("{count} flow(s) defined", { count: flowNames.length })
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

      {/* Content status / coverage */}
      {hasContent && (
        <section className="mb-8">
          <h2 className="mb-3 flex items-center gap-2 text-sm font-semibold uppercase tracking-wider text-muted-foreground">
            <FileText size={14} />
            Content Overview
            <Button
              variant="ghost"
              size="sm"
              className="ml-auto h-7 normal-case"
              onClick={() => void handleExtract()}
              disabled={extracting || hasActive}
              aria-label={status?.hasData ? "Re-extract content" : "Run extract"}
            >
              {extracting ? (
                <Loader2 size={12} className="animate-spin" />
              ) : (
                <RefreshCw size={12} />
              )}
              {status?.hasData ? "Re-extract" : "Extract"}
            </Button>
          </h2>

          {status && !status.hasData ? (
            <EmptyState
              icon={<PackageOpen size={24} className="text-muted-foreground/50" />}
              title="Nothing extracted yet."
              description="Run extract to read your content files and analyze their structure."
              action={
                <Button size="sm" onClick={() => void handleExtract()} disabled={extracting}>
                  {extracting ? (
                    <>
                      <Loader2 size={12} className="animate-spin" />
                      Extracting...
                    </>
                  ) : (
                    "Run extract"
                  )}
                </Button>
              }
            />
          ) : status && filteredCollections.length > 0 ? (
            <div className="space-y-3">
              {/* Compact summary: block distribution (donut) + cross-collection
                  coverage per language. */}
              <Card className="p-4">
                <div className="grid gap-5 sm:grid-cols-[auto_1fr] sm:items-center">
                  <div className="flex items-center gap-3">
                    {distData.length > 0 ? (
                      <div className="h-28 w-28 shrink-0">
                        <ResponsiveContainer width="100%" height="100%">
                          <PieChart>
                            <Pie
                              data={distData}
                              dataKey="value"
                              nameKey="name"
                              innerRadius="58%"
                              outerRadius="100%"
                              paddingAngle={distData.length > 1 ? 2 : 0}
                              strokeWidth={0}
                            >
                              {distData.map((d) => (
                                <Cell key={d.name} fill={d.fill} />
                              ))}
                            </Pie>
                          </PieChart>
                        </ResponsiveContainer>
                      </div>
                    ) : (
                      <div className="flex h-28 w-28 shrink-0 items-center justify-center rounded-full border border-dashed text-[10px] text-muted-foreground">
                        {t("No blocks")}
                      </div>
                    )}
                    <ul className="space-y-1 text-xs">
                      <li className="font-medium text-foreground">
                        {t("{count} block(s)", { count: totalBlocks })}
                      </li>
                      {filteredCollections.map((c, i) => (
                        <li key={c.name} className="flex items-center gap-1.5">
                          <span
                            className="size-2 shrink-0 rounded-[2px]"
                            style={{ background: CHART_COLORS[i % CHART_COLORS.length] }}
                          />
                          <span className="truncate text-muted-foreground">{c.name}</span>
                          <span className="tabular-nums text-foreground">{c.blockCount}</span>
                        </li>
                      ))}
                    </ul>
                  </div>

                  {projectCoverage.length > 0 && (
                    <div className="space-y-1.5">
                      <div className="mb-1 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
                        {t("Coverage across collections")}
                      </div>
                      {projectCoverage.map((p) => (
                        <CoverageBar
                          key={p.lang}
                          label={p.lang}
                          translated={p.translated}
                          total={p.total}
                          pct={p.pct}
                        />
                      ))}
                    </div>
                  )}
                </div>
              </Card>

              {/* Per-collection compact rows — expand for the per-language detail. */}
              <div className="space-y-1.5">
                {filteredCollections.map((c) => {
                  const langs = filterLanguages(c.targetLanguages, activeFilter);
                  const open = expandedColls.has(c.name);
                  const avg = collAvgPct(c, langs);
                  return (
                    <Card key={c.name} className="overflow-hidden p-0">
                      <button
                        type="button"
                        onClick={() => langs.length > 0 && toggleColl(c.name)}
                        className={`flex w-full items-center gap-3 px-3 py-2 text-left ${
                          langs.length > 0 ? "hover:bg-accent/30" : "cursor-default"
                        }`}
                        aria-expanded={open}
                      >
                        {langs.length > 0 ? (
                          open ? (
                            <ChevronDown size={13} className="shrink-0 text-muted-foreground" />
                          ) : (
                            <ChevronRight size={13} className="shrink-0 text-muted-foreground" />
                          )
                        ) : (
                          <span className="w-[13px] shrink-0" />
                        )}
                        <span className="flex-1 truncate text-sm font-medium">{c.name}</span>
                        <span className="shrink-0 text-xs text-muted-foreground">
                          {t("{count} block(s)", { count: c.blockCount })}
                        </span>
                        {langs.length > 0 && (
                          <span className="flex w-32 shrink-0 items-center gap-2">
                            <span className="h-1.5 flex-1 overflow-hidden rounded-full bg-accent">
                              <span
                                className="block h-full rounded-full bg-primary"
                                style={{ width: `${avg}%` }}
                              />
                            </span>
                            <span className="w-8 shrink-0 text-right text-[11px] tabular-nums text-muted-foreground">
                              {avg}%
                            </span>
                          </span>
                        )}
                      </button>
                      {open && langs.length > 0 && (
                        <div className="space-y-1.5 border-t border-border px-3 py-2">
                          {langs.map((loc) => {
                            const translated = c.coverage?.[loc] ?? 0;
                            const pct =
                              c.blockCount > 0 ? Math.round((translated / c.blockCount) * 100) : 0;
                            return (
                              <CoverageBar
                                key={loc}
                                label={loc}
                                translated={translated}
                                total={c.blockCount}
                                pct={pct}
                              />
                            );
                          })}
                        </div>
                      )}
                    </Card>
                  );
                })}
              </div>
            </div>
          ) : null}
        </section>
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
