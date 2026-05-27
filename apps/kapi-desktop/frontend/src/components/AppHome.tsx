import { FolderKanban, FolderOpen, Sparkles, Workflow, Wrench, X } from "lucide-react";
import { Button } from "@neokapi/ui-primitives";
import { useShortenHome } from "../hooks/useShortenHome";

interface AppHomeProps {
  recentFiles: Array<{ path: string; name: string; opened_at: string }>;
  samplesDismissed: boolean;
  onOpenRecent: (path: string) => void;
  onNewProject: () => void;
  onOpenProject: () => void;
  onNavigate: (view: string) => void;
  onCreateSampleProject: (name: string) => void;
  onDismissSamples: () => void;
}

export function AppHome({
  recentFiles,
  samplesDismissed,
  onOpenRecent,
  onNewProject,
  onOpenProject,
  onNavigate,
  onCreateSampleProject,
  onDismissSamples,
}: AppHomeProps) {
  const shortenHome = useShortenHome();
  return (
    <div className="p-6">
      <div className="mb-8 flex items-center gap-4">
        <img src="/neokapi-logo.png" alt="neokapi" className="h-12 w-12 drop-shadow-lg" />
        <div>
          <h1 className="text-xl font-semibold">Welcome to Kapi</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Localization plumbing and glue for people, elves, and agents.
          </p>
        </div>
      </div>

      {/* Sample projects — shown until explicitly dismissed */}
      {!samplesDismissed && (
        <div className="mb-8">
          <div className="mb-3 flex items-center gap-2 text-sm font-medium text-muted-foreground">
            <Sparkles size={14} />
            <span className="flex-1">New to Kapi? Try a sample project</span>
            <Button
              variant="ghost"
              size="icon-xs"
              onClick={onDismissSamples}
              className="text-muted-foreground/60"
              title="Dismiss"
            >
              <X size={14} />
            </Button>
          </div>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <Button
              variant="outline"
              data-testid="sample-kapimart"
              onClick={() => onCreateSampleProject("kapimart")}
              className="h-auto whitespace-normal rounded-lg border-primary/20 bg-primary/5 p-4 text-left flex-col items-start hover:border-primary/40 hover:bg-primary/10"
            >
              <div className="text-sm font-medium">KapiMart</div>
              <p className="mt-1 text-xs text-muted-foreground font-normal">
                A realistic localization project with docs, store UI, Office documents, and
                templates — 4 content collections, 5 target languages, 1000+ TM entries. No plugins
                needed.
              </p>
            </Button>
            <Button
              variant="outline"
              data-testid="sample-okapimart"
              onClick={() => onCreateSampleProject("okapimart")}
              className="h-auto whitespace-normal rounded-lg border-primary/20 bg-primary/5 p-4 text-left flex-col items-start hover:border-primary/40 hover:bg-primary/10"
            >
              <div className="text-sm font-medium">OkapiMart</div>
              <p className="mt-1 text-xs text-muted-foreground font-normal">
                Same store files processed through Okapi Java filters. Requires the okapi-bridge
                plugin.
              </p>
            </Button>
          </div>
        </div>
      )}

      {/* Quick actions */}
      <div className="mb-8 grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <Button
          variant="outline"
          onClick={onNewProject}
          className="h-auto whitespace-normal rounded-lg p-4 text-left flex-col items-start hover:border-primary/30 hover:bg-accent/30"
        >
          <FolderKanban size={18} className="mb-2 text-primary" />
          <div className="text-sm font-medium">New Project</div>
          <div className="text-xs text-muted-foreground font-normal">Create a Kapi project</div>
        </Button>
        <Button
          variant="outline"
          onClick={onOpenProject}
          className="h-auto whitespace-normal rounded-lg p-4 text-left flex-col items-start hover:border-primary/30 hover:bg-accent/30"
        >
          <FolderOpen size={18} className="mb-2 text-primary" />
          <div className="text-sm font-medium">Open a Project</div>
          <div className="text-xs text-muted-foreground font-normal">
            Open an existing Kapi project
          </div>
        </Button>
        <Button
          variant="outline"
          onClick={() => onNavigate("flows")}
          className="h-auto whitespace-normal rounded-lg p-4 text-left flex-col items-start hover:border-primary/30 hover:bg-accent/30"
        >
          <Workflow size={18} className="mb-2 text-primary" />
          <div className="text-sm font-medium">Design a Flow</div>
          <div className="text-xs text-muted-foreground font-normal">Build tool pipelines</div>
        </Button>
        <Button
          variant="outline"
          onClick={() => onNavigate("tools")}
          className="h-auto whitespace-normal rounded-lg p-4 text-left flex-col items-start hover:border-primary/30 hover:bg-accent/30"
        >
          <Wrench size={18} className="mb-2 text-primary" />
          <div className="text-sm font-medium">Run a Tool</div>
          <div className="text-xs text-muted-foreground font-normal">Execute a tool on files</div>
        </Button>
      </div>

      {/* Recent projects */}
      {recentFiles.length > 0 && (
        <section>
          <h2 className="mb-3 text-sm font-semibold uppercase tracking-wider text-muted-foreground">
            Recent Projects
          </h2>
          <div className="space-y-1">
            {recentFiles.map((file) => (
              <Button
                key={file.path}
                variant="outline"
                onClick={() => onOpenRecent(file.path)}
                className="flex w-full h-auto items-center gap-3 rounded-lg p-3 text-left hover:bg-accent/30"
              >
                <FolderKanban size={16} className="shrink-0 text-muted-foreground" />
                <div className="flex-1 truncate">
                  <div className="text-sm font-medium">{file.name}</div>
                  <div className="truncate text-xs text-muted-foreground">
                    {file.path.endsWith("/project.kapi")
                      ? shortenHome(file.path.replace(/\/project\.kapi$/, ""))
                      : shortenHome(file.path)}
                  </div>
                </div>
              </Button>
            ))}
          </div>
        </section>
      )}
    </div>
  );
}
