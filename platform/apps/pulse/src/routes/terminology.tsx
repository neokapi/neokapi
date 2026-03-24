import { useParams, useSearch, useNavigate } from "@tanstack/react-router";
import { TermExplorerPublic } from "@neokapi/ui/components/pulse";
import { usePulseTerms } from "../hooks/use-pulse";

export function TerminologyPage() {
  const { workspace } = useParams({ strict: false }) as { workspace: string };
  const search = useSearch({ strict: false }) as { q?: string };
  const navigate = useNavigate();

  const params = new URLSearchParams();
  if (search.q) params.set("q", search.q);

  const { data, isLoading } = usePulseTerms(workspace, params);

  if (isLoading) {
    return <div className="h-64 animate-pulse rounded-lg border bg-muted" />;
  }

  return (
    <div className="space-y-6">
      <h1 className="text-xl font-bold">Terminology</h1>
      <TermExplorerPublic terms={data?.terms ?? []} />
    </div>
  );
}
