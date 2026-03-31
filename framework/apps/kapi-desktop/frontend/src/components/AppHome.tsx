import { FolderKanban, FolderOpen, Sparkles, Workflow, Wrench, X } from "lucide-react";
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
            <button
              onClick={onDismissSamples}
              className="rounded p-0.5 text-muted-foreground/60 transition-colors hover:bg-accent hover:text-foreground"
              title="Dismiss"
            >
              <X size={14} />
            </button>
          </div>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <button
              onClick={() => onCreateSampleProject("kapimart")}
              className="rounded-lg border border-primary/20 bg-primary/5 p-4 text-left transition-colors hover:border-primary/40 hover:bg-primary/10"
            >
              <div className="text-sm font-medium">KapiMart</div>
              <p className="mt-1 text-xs text-muted-foreground">
                A sample store using Kapi's built-in Go formats — JSON, HTML, Markdown, and more.
                No plugins needed.
              </p>
            </button>
            <button
              onClick={() => onCreateSampleProject("okapimart")}
              className="rounded-lg border border-primary/20 bg-primary/5 p-4 text-left transition-colors hover:border-primary/40 hover:bg-primary/10"
            >
              <div className="text-sm font-medium">OkapiMart</div>
              <p className="mt-1 text-xs text-muted-foreground">
                Same store files processed through Okapi Java filters. Requires the okapi-bridge
                plugin.
              </p>
            </button>
          </div>
        </div>
      )}

      {/* Quick actions */}
      <div className="mb-8 grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <button
          onClick={onNewProject}
          className="rounded-lg border border-border p-4 text-left transition-colors hover:border-primary/30 hover:bg-accent/30"
        >
          <FolderKanban size={18} className="mb-2 text-primary" />
          <div className="text-sm font-medium">New Project</div>
          <div className="text-xs text-muted-foreground">Create a Kapi project</div>
        </button>
        <button
          onClick={onOpenProject}
          className="rounded-lg border border-border p-4 text-left transition-colors hover:border-primary/30 hover:bg-accent/30"
        >
          <FolderOpen size={18} className="mb-2 text-primary" />
          <div className="text-sm font-medium">Open a Project</div>
          <div className="text-xs text-muted-foreground">Open an existing Kapi project</div>
        </button>
        <button
          onClick={() => onNavigate("flows")}
          className="rounded-lg border border-border p-4 text-left transition-colors hover:border-primary/30 hover:bg-accent/30"
        >
          <Workflow size={18} className="mb-2 text-primary" />
          <div className="text-sm font-medium">Design a Flow</div>
          <div className="text-xs text-muted-foreground">Build tool pipelines</div>
        </button>
        <button
          onClick={() => onNavigate("tools")}
          className="rounded-lg border border-border p-4 text-left transition-colors hover:border-primary/30 hover:bg-accent/30"
        >
          <Wrench size={18} className="mb-2 text-primary" />
          <div className="text-sm font-medium">Run a Tool</div>
          <div className="text-xs text-muted-foreground">Execute a tool on files</div>
        </button>
      </div>

      {/* Recent projects */}
      {recentFiles.length > 0 && (
        <section>
          <h2 className="mb-3 text-sm font-semibold uppercase tracking-wider text-muted-foreground">
            Recent Projects
          </h2>
          <div className="space-y-1">
            {recentFiles.map((file) => (
              <button
                key={file.path}
                onClick={() => onOpenRecent(file.path)}
                className="flex w-full items-center gap-3 rounded-lg border border-border p-3 text-left transition-colors hover:bg-accent/30"
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
              </button>
            ))}
          </div>
        </section>
      )}
    </div>
  );
}
