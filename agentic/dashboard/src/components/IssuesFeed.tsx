import { Badge } from "@/components/ui/badge";
import { useApi } from "@/context/ApiContext";

export default function IssuesFeed() {
  const api = useApi();

  // Issues are not yet available from the Bowrain API.
  // Show a clean empty state rather than mock data.
  return (
    <div className="space-y-3">
      <p className="text-sm text-muted-foreground">GitHub issues from agent-feedback repo</p>

      <div className="rounded-lg border px-6 py-12 text-center">
        <p className="text-sm font-medium text-muted-foreground">No issues available</p>
        <p className="mt-1 text-xs text-muted-foreground/60">
          {api.connected
            ? "Issue tracking is not yet connected to the dashboard."
            : "Connect to the Bowrain API to see agent-filed issues."}
        </p>
        <div className="mt-3 flex justify-center gap-2">
          <Badge variant="outline" className="text-[10px]">
            agent-feedback
          </Badge>
          <Badge variant="outline" className="text-[10px]">
            GitHub
          </Badge>
        </div>
      </div>
    </div>
  );
}
