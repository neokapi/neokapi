import { useNavigate } from "@tanstack/react-router";
import { Globe, Languages, FolderOpen } from "lucide-react";
import { usePulseFrontPage } from "../hooks/use-pulse";
import type { PulseWorkspaceSummary } from "../api";

export function FrontPage() {
  const navigate = useNavigate();
  const { data, isLoading, error } = usePulseFrontPage();

  if (isLoading) {
    return <FrontPageSkeleton />;
  }

  if (error || !data) {
    return (
      <div className="flex min-h-[400px] items-center justify-center">
        <div className="text-center">
          <h2 className="text-lg font-semibold">Unable to load</h2>
          <p className="mt-1 text-sm text-muted-foreground">Please try again later.</p>
        </div>
      </div>
    );
  }

  const hasWorkspaces = data.workspaces.length > 0;

  return (
    <div className="space-y-10">
      <section className="text-center">
        <h1 className="text-3xl font-bold tracking-tight sm:text-4xl">
          Discover localization projects
        </h1>
        <p className="mx-auto mt-3 max-w-xl text-muted-foreground">
          See translation progress, find languages that need help, and join the community.
        </p>
      </section>

      {hasWorkspaces && (
        <section className="grid gap-4 sm:grid-cols-3">
          <StatCard
            icon={<FolderOpen className="h-5 w-5" />}
            label="Projects"
            value={data.stats.total_projects}
          />
          <StatCard
            icon={<Languages className="h-5 w-5" />}
            label="Languages"
            value={data.stats.total_languages}
          />
          <StatCard
            icon={<Globe className="h-5 w-5" />}
            label="Workspaces"
            value={data.workspaces.length}
          />
        </section>
      )}

      {hasWorkspaces ? (
        <section>
          <h2 className="mb-4 text-xl font-semibold">Public workspaces</h2>
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {data.workspaces.map((ws) => (
              <WorkspaceCard
                key={ws.slug}
                workspace={ws}
                onClick={() => navigate({ to: "/$workspace", params: { workspace: ws.slug } })}
              />
            ))}
          </div>
        </section>
      ) : (
        <section className="rounded-lg border border-dashed p-12 text-center">
          <Globe className="mx-auto h-10 w-10 text-muted-foreground/50" />
          <h2 className="mt-4 text-lg font-semibold">No public workspaces yet</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            When workspace owners make their dashboards public, they will appear here.
          </p>
        </section>
      )}
    </div>
  );
}

function StatCard({ icon, label, value }: { icon: React.ReactNode; label: string; value: number }) {
  return (
    <div className="flex items-center gap-3 rounded-lg border bg-card p-4">
      <div className="rounded-md bg-primary/10 p-2 text-primary">{icon}</div>
      <div>
        <p className="text-2xl font-bold">{value}</p>
        <p className="text-sm text-muted-foreground">{label}</p>
      </div>
    </div>
  );
}

function WorkspaceCard({
  workspace,
  onClick,
}: {
  workspace: PulseWorkspaceSummary;
  onClick: () => void;
}) {
  const pct = Math.round(workspace.percentage);

  return (
    <button
      onClick={onClick}
      className="group flex flex-col rounded-lg border bg-card p-5 text-left transition-colors hover:border-primary/40 hover:bg-accent/50"
    >
      <div className="flex items-center gap-3">
        {workspace.logo_url ? (
          <img src={workspace.logo_url} alt={workspace.name} className="h-10 w-10 rounded" />
        ) : (
          <div className="flex h-10 w-10 items-center justify-center rounded bg-primary text-primary-foreground text-lg font-bold">
            {workspace.name.charAt(0).toUpperCase()}
          </div>
        )}
        <div className="min-w-0 flex-1">
          <h3 className="truncate font-semibold group-hover:text-primary">{workspace.name}</h3>
          {workspace.description && (
            <p className="truncate text-sm text-muted-foreground">{workspace.description}</p>
          )}
        </div>
      </div>

      <div className="mt-4 flex items-center gap-4 text-sm text-muted-foreground">
        <span>{workspace.projects} projects</span>
        <span>{workspace.languages} languages</span>
      </div>

      <div className="mt-3 w-full">
        <div className="flex items-center justify-between text-xs">
          <span className="text-muted-foreground">Progress</span>
          <span className="font-medium">{pct}%</span>
        </div>
        <div className="mt-1 h-2 w-full overflow-hidden rounded-full bg-secondary">
          <div
            className="h-full rounded-full bg-primary transition-all"
            style={{ width: `${pct}%` }}
          />
        </div>
      </div>
    </button>
  );
}

function FrontPageSkeleton() {
  return (
    <div className="space-y-10">
      <div className="flex flex-col items-center gap-3">
        <div className="h-9 w-80 animate-pulse rounded bg-muted" />
        <div className="h-5 w-64 animate-pulse rounded bg-muted" />
      </div>
      <div className="grid gap-4 sm:grid-cols-3">
        {Array.from({ length: 3 }).map((_, i) => (
          <div key={i} className="h-20 animate-pulse rounded-lg border bg-muted" />
        ))}
      </div>
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {Array.from({ length: 3 }).map((_, i) => (
          <div key={i} className="h-40 animate-pulse rounded-lg border bg-muted" />
        ))}
      </div>
    </div>
  );
}
