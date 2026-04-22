import { useState, useEffect } from "react";
import {
  Play,
  Globe,
  Workflow,
  Loader2,
  Plug,
  FileText,
  Settings2,
  Wrench,
  AlertTriangle,
} from "lucide-react";
import { Button, Badge, Card, EmptyState, ActionCard } from "@neokapi/ui-primitives";
import { t } from "@neokapi/kapi-react/runtime";
import type { KapiProject, FlowSpec, FlowInfo, PluginIssue } from "../types/api";
import { isBareEntry, effectiveItems } from "../types/api";
import { api } from "../hooks/useApi";
import { useJobFeed } from "../context/JobFeedContext";

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
}

export function HomePage({
  project,
  displayName,
  tabID,
  onRunFlow,
  onNavigate,
  pluginsResolved,
  pluginIssues,
}: HomePageProps) {
  const { hasActive, activeJob } = useJobFeed();
  const [flowValidation, setFlowValidation] = useState<Record<string, FlowInfo>>({});

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
          <span className="flex items-center gap-1.5">
            <Globe size={14} />
            {defaults.source_language || "No source"} &rarr;{" "}
            {defaults.target_languages?.length
              ? defaults.target_languages.join(", ")
              : "No targets"}
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
                      <span className="text-muted-foreground">not installed</span>
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
                  Install Plugins
                </Button>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Quick actions */}
      <div className="mb-8 grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
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
