import { useParams, useNavigate } from "@tanstack/react-router";
import { PulseOverview } from "@neokapi/ui/components/pulse";
import { usePulseOverview } from "../hooks/use-pulse";

export function WorkspaceOverviewPage() {
  const { workspace } = useParams({ strict: false }) as { workspace: string };
  const navigate = useNavigate();
  const { data, isLoading, error } = usePulseOverview(workspace);

  if (isLoading) {
    return <LoadingSkeleton />;
  }

  if (error || !data) {
    return (
      <div className="flex min-h-[400px] items-center justify-center">
        <div className="text-center">
          <h2 className="text-lg font-semibold">Dashboard not available</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            This workspace dashboard may be private or not found.
          </p>
        </div>
      </div>
    );
  }

  return (
    <PulseOverview
      stats={data.stats}
      projects={data.projects}
      languages={data.top_languages}
      onProjectClick={(id: string) =>
        navigate({
          to: "/$workspace/projects/$pid",
          params: { workspace, pid: id },
        })
      }
    />
  );
}

function LoadingSkeleton() {
  return (
    <div className="space-y-8">
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {Array.from({ length: 4 }).map((_, i) => (
          <div key={i} className="h-24 animate-pulse rounded-lg border bg-muted" />
        ))}
      </div>
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {Array.from({ length: 3 }).map((_, i) => (
          <div key={i} className="h-32 animate-pulse rounded-lg border bg-muted" />
        ))}
      </div>
    </div>
  );
}
