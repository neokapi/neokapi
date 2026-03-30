import { useState } from "react";
import { Play, Globe, Workflow, Wrench, Loader2 } from "lucide-react";
import type { KapiProject, FlowSpec } from "../types/api";

interface HomePageProps {
  project: KapiProject;
  onRunFlow?: (flowName: string, flow: FlowSpec) => void;
  onNavigate: (view: string) => void;
}

export function HomePage({ project, onRunFlow, onNavigate }: HomePageProps) {
  const [runningFlow, setRunningFlow] = useState<string | null>(null);
  const flowNames = Object.keys(project.flows ?? {});
  const hasContent = (project.content?.length ?? 0) > 0;

  const handleRunFlow = (name: string) => {
    const spec = project.flows?.[name];
    if (!spec || !onRunFlow) return;
    setRunningFlow(name);
    onRunFlow(name, spec);
    setTimeout(() => setRunningFlow(null), 2000);
  };

  return (
    <div className="p-6">
      {/* Project summary */}
      <div className="mb-6">
        <h1 className="text-xl font-semibold">{project.name}</h1>
        <div className="mt-2 flex items-center gap-4 text-sm text-muted-foreground">
          <span className="flex items-center gap-1.5">
            <Globe size={14} />
            {project.source_language || "No source"} &rarr;{" "}
            {project.target_languages?.length ? project.target_languages.join(", ") : "No targets"}
          </span>
        </div>
      </div>

      {/* Quick actions */}
      <div className="mb-8 grid grid-cols-1 gap-3 sm:grid-cols-3">
        <button
          onClick={() => onNavigate("content")}
          className="rounded-lg border border-border p-4 text-left transition-colors hover:border-primary/30 hover:bg-accent/30"
        >
          <div className="mb-1 text-sm font-medium">Content</div>
          <div className="text-xs text-muted-foreground">
            {hasContent
              ? `${project.content!.length} pattern${project.content!.length !== 1 ? "s" : ""} configured`
              : "Configure file patterns"}
          </div>
        </button>
        <button
          onClick={() => onNavigate("flows")}
          className="rounded-lg border border-border p-4 text-left transition-colors hover:border-primary/30 hover:bg-accent/30"
        >
          <div className="mb-1 text-sm font-medium">Flows</div>
          <div className="text-xs text-muted-foreground">
            {flowNames.length > 0
              ? `${flowNames.length} flow${flowNames.length !== 1 ? "s" : ""} defined`
              : "Build your first flow"}
          </div>
        </button>
        <button
          onClick={() => onNavigate("tools")}
          className="rounded-lg border border-border p-4 text-left transition-colors hover:border-primary/30 hover:bg-accent/30"
        >
          <div className="mb-1 text-sm font-medium">Tools</div>
          <div className="text-xs text-muted-foreground">Run individual tools on files</div>
        </button>
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
                <div
                  key={name}
                  className="flex items-center gap-3 rounded-lg border border-border p-3"
                >
                  <div className="flex-1">
                    <div className="text-sm font-medium">{name}</div>
                    <div className="mt-0.5 flex items-center gap-1.5 text-xs text-muted-foreground">
                      {spec.steps.map((step, i) => (
                        <span key={i} className="flex items-center gap-1">
                          {i > 0 && <span>&rarr;</span>}
                          <span className="rounded bg-accent px-1.5 py-0.5">{step.tool}</span>
                        </span>
                      ))}
                    </div>
                  </div>
                  <button
                    onClick={() => handleRunFlow(name)}
                    disabled={runningFlow === name || !hasContent}
                    className="flex items-center gap-1.5 rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
                    aria-label={`Run flow ${name}`}
                    title={!hasContent ? "Configure content patterns first" : undefined}
                  >
                    {runningFlow === name ? (
                      <Loader2 size={12} className="animate-spin" />
                    ) : (
                      <Play size={12} />
                    )}
                    Run
                  </button>
                </div>
              );
            })}
          </div>
        </section>
      )}

      {/* Empty state */}
      {flowNames.length === 0 && (
        <div className="rounded-lg border border-dashed border-border p-8 text-center">
          <Workflow size={24} className="mx-auto mb-2 text-muted-foreground/50" />
          <p className="text-sm text-muted-foreground">
            No flows defined yet.{" "}
            <button onClick={() => onNavigate("flows")} className="text-primary hover:underline">
              Create your first flow
            </button>{" "}
            to get started.
          </p>
        </div>
      )}
    </div>
  );
}
