import { FolderKanban, Workflow, Wrench } from "lucide-react";
interface AppHomeProps {
  recentFiles: Array<{ path: string; name: string; opened_at: string }>;
  onOpenRecent: (path: string) => void;
  onNewProject: () => void;
  onNavigate: (view: string) => void;
}

export function AppHome({ recentFiles, onOpenRecent, onNewProject, onNavigate }: AppHomeProps) {
  return (
    <div className="p-6">
      <div className="mb-8">
        <h1 className="text-xl font-semibold">Welcome to Kapi Desktop</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Localization plumbing and glue for people, elves, and agents.
        </p>
      </div>

      {/* Quick actions */}
      <div className="mb-8 grid grid-cols-1 gap-3 sm:grid-cols-3">
        <button
          onClick={onNewProject}
          className="rounded-lg border border-border p-4 text-left transition-colors hover:border-primary/30 hover:bg-accent/30"
        >
          <FolderKanban size={18} className="mb-2 text-primary" />
          <div className="text-sm font-medium">New Project</div>
          <div className="text-xs text-muted-foreground">
            Create a Kapi project with content, flows, and config
          </div>
        </button>
        <button
          onClick={() => onNavigate("flows")}
          className="rounded-lg border border-border p-4 text-left transition-colors hover:border-primary/30 hover:bg-accent/30"
        >
          <Workflow size={18} className="mb-2 text-primary" />
          <div className="text-sm font-medium">Design a Flow</div>
          <div className="text-xs text-muted-foreground">
            Build and run tool pipelines on ad-hoc files
          </div>
        </button>
        <button
          onClick={() => onNavigate("tools")}
          className="rounded-lg border border-border p-4 text-left transition-colors hover:border-primary/30 hover:bg-accent/30"
        >
          <Wrench size={18} className="mb-2 text-primary" />
          <div className="text-sm font-medium">Run a Tool</div>
          <div className="text-xs text-muted-foreground">
            Execute a single tool on files
          </div>
        </button>
      </div>

      {/* Recent projects */}
      <section>
        <h2 className="mb-3 text-sm font-semibold uppercase tracking-wider text-muted-foreground">
          Recent Projects
        </h2>
        {recentFiles.length > 0 ? (
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
                  <div className="truncate text-xs text-muted-foreground">{file.path}</div>
                </div>
              </button>
            ))}
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">No recent projects</p>
        )}
      </section>
    </div>
  );
}
