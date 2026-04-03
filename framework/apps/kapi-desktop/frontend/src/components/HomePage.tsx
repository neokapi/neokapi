import { useState } from "react";
import { Play, Globe, Workflow, Wrench, Loader2 } from "lucide-react";
import { Button, Badge, Card, CardContent } from "@neokapi/ui-primitives";
import type { KapiProject, FlowSpec } from "../types/api";

interface HomePageProps {
  project: KapiProject;
  displayName: string;
  onRunFlow?: (flowName: string, flow: FlowSpec) => void;
  onNavigate: (view: string) => void;
}

export function HomePage({ project, displayName, onRunFlow, onNavigate }: HomePageProps) {
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
        <h1 className="text-xl font-semibold">{displayName}</h1>
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
        <Button
          variant="outline"
          onClick={() => onNavigate("content")}
          className="h-auto rounded-lg p-4 text-left flex-col items-start hover:border-primary/30 hover:bg-accent/30"
        >
          <div className="mb-1 text-sm font-medium">Content</div>
          <div className="text-xs text-muted-foreground font-normal">
            {hasContent
              ? `${project.content!.length} pattern${project.content!.length !== 1 ? "s" : ""} configured`
              : "Configure file patterns"}
          </div>
        </Button>
        <Button
          variant="outline"
          onClick={() => onNavigate("flows")}
          className="h-auto rounded-lg p-4 text-left flex-col items-start hover:border-primary/30 hover:bg-accent/30"
        >
          <div className="mb-1 text-sm font-medium">Flows</div>
          <div className="text-xs text-muted-foreground font-normal">
            {flowNames.length > 0
              ? `${flowNames.length} flow${flowNames.length !== 1 ? "s" : ""} defined`
              : "Build your first flow"}
          </div>
        </Button>
        <Button
          variant="outline"
          onClick={() => onNavigate("tools")}
          className="h-auto rounded-lg p-4 text-left flex-col items-start hover:border-primary/30 hover:bg-accent/30"
        >
          <div className="mb-1 text-sm font-medium">Tools</div>
          <div className="text-xs text-muted-foreground font-normal">Run individual tools on files</div>
        </Button>
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
                </div>
              );
            })}
          </div>
        </section>
      )}

      {/* Empty state */}
      {flowNames.length === 0 && (
        <Card className="border-dashed">
          <CardContent className="p-8 text-center">
            <Workflow size={24} className="mx-auto mb-2 text-muted-foreground/50" />
            <p className="text-sm text-muted-foreground">
              No flows defined yet.{" "}
              <Button variant="link" size="sm" onClick={() => onNavigate("flows")} className="px-0 h-auto">
                Create your first flow
              </Button>{" "}
              to get started.
            </p>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
