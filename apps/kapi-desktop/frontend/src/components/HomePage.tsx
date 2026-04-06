import { useState } from "react";
import { Play, Globe, Workflow, Loader2, Plug, FileText, Settings2, Wrench } from "lucide-react";
import { Button, Badge, Card, EmptyState, ActionCard } from "@neokapi/ui-primitives";
import type { KapiProject, FlowSpec } from "../types/api";
import { isBareEntry, effectiveItems } from "../types/api";

export interface HomePageProps {
  project: KapiProject;
  displayName: string;
  onRunFlow?: (flowName: string, flow: FlowSpec) => void;
  onNavigate: (view: string) => void;
}

export function HomePage({ project, displayName, onRunFlow, onNavigate }: HomePageProps) {
  const [runningFlow, setRunningFlow] = useState<string | null>(null);
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
    setRunningFlow(name);
    onRunFlow(name, spec);
    setTimeout(() => setRunningFlow(null), 2000);
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

      {/* Quick actions */}
      <div className="mb-8 grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <ActionCard
          icon={<FileText size={16} />}
          title="Content"
          description={
            hasContent
              ? `${contentCount} collection${contentCount !== 1 ? "s" : ""}, ${itemCount} pattern${itemCount !== 1 ? "s" : ""}`
              : "Configure file patterns"
          }
          onClick={() => onNavigate("content")}
        />
        <ActionCard
          icon={<Workflow size={16} />}
          title="Flows"
          description={
            flowNames.length > 0
              ? `${flowNames.length} flow${flowNames.length !== 1 ? "s" : ""} defined`
              : "Build your first flow"
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
              return (
                <Card key={name} className="flex flex-row items-center gap-3 p-3">
                  <div className="flex-1">
                    <div className="text-sm font-medium">{name}</div>
                    <div className="mt-0.5 flex items-center gap-1.5 text-xs text-muted-foreground">
                      {spec.steps.map((step, i) => (
                        <span key={i} className="flex items-center gap-1">
                          {i > 0 && <span>&rarr;</span>}
                          <Badge variant="secondary">{step.tool}</Badge>
                        </span>
                      ))}
                    </div>
                  </div>
                  <Button
                    size="sm"
                    onClick={() => handleRunFlow(name)}
                    disabled={runningFlow === name || !hasContent}
                    aria-label={`Run flow ${name}`}
                    title={!hasContent ? "Configure content patterns first" : undefined}
                  >
                    {runningFlow === name ? (
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
