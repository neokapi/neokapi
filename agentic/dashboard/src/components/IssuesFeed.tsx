import { Badge } from "@/components/ui/badge";
import { useApi } from "@/context/ApiContext";

function formatTimeAgo(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime();
  const mins = Math.floor(diff / 60_000);
  if (mins < 1) return "just now";
  if (mins < 60) return `${mins}m ago`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours}h ago`;
  return `${Math.floor(hours / 24)}d ago`;
}

export default function IssuesFeed() {
  const api = useApi();

  if (api.issues.length === 0) {
    return (
      <div className="space-y-3">
        <p className="text-sm text-muted-foreground">GitHub issues filed by agents</p>
        <div className="rounded-lg border px-6 py-12 text-center">
          <p className="text-sm font-medium text-muted-foreground">No issues filed</p>
          <p className="mt-1 text-xs text-muted-foreground/60">
            {api.connected
              ? "Agents haven't filed any issues yet."
              : "Connect to the API to see agent-filed issues."}
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-3">
      <p className="text-sm text-muted-foreground">
        {api.issues.length} issue{api.issues.length !== 1 ? "s" : ""} from agents
      </p>
      <div className="space-y-2">
        {api.issues.map((issue) => (
          <a
            key={issue.number}
            href={issue.html_url}
            target="_blank"
            rel="noopener noreferrer"
            className="block rounded-lg border p-3 transition-colors hover:bg-accent/30"
          >
            <div className="flex items-start justify-between gap-2">
              <div className="min-w-0 flex-1">
                <div className="text-sm font-medium truncate">
                  #{issue.number} {issue.title}
                </div>
                <div className="mt-1 flex items-center gap-2 text-[11px] text-muted-foreground">
                  <span>{issue.author}</span>
                  <span>&middot;</span>
                  <span>{formatTimeAgo(issue.created_at)}</span>
                </div>
              </div>
              <Badge
                variant={issue.state === "open" ? "default" : "secondary"}
                className="text-[10px] shrink-0"
              >
                {issue.state}
              </Badge>
            </div>
            {issue.labels.length > 0 && (
              <div className="mt-2 flex flex-wrap gap-1">
                {issue.labels.map((label) => (
                  <Badge key={label} variant="outline" className="text-[10px]">
                    {label}
                  </Badge>
                ))}
              </div>
            )}
          </a>
        ))}
      </div>
    </div>
  );
}
