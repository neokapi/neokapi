import { useMemo } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { ExternalLink } from "lucide-react";
import { useFilter } from "@/context/FilterContext";
import { useApi } from "@/context/ApiContext";

function formatTime(iso: string): string {
  return new Date(iso).toLocaleTimeString("en-US", {
    hour: "2-digit",
    minute: "2-digit",
  });
}

function dateLabel(iso: string): string {
  const date = new Date(iso);
  const today = new Date();
  const yesterday = new Date(today);
  yesterday.setDate(yesterday.getDate() - 1);

  if (date.toDateString() === today.toDateString()) return "Today";
  if (date.toDateString() === yesterday.toDateString()) return "Yesterday";
  return date.toLocaleDateString("en-US", {
    weekday: "long",
    month: "short",
    day: "numeric",
  });
}

function getDisplayName(actor: string): string {
  return actor || "System";
}

function formatEventDescription(eventType: string, data: string): string {
  try {
    const parsed = JSON.parse(data);
    switch (eventType) {
      case "block.target.updated":
      case "block.updated":
        return `Updated block ${parsed.block_id ?? ""}`;
      case "stream.created":
        return `Created stream ${parsed.stream ?? ""}${parsed.parent ? ` in ${parsed.parent}` : ""}`;
      case "connector.push.completed":
        return `Push completed: ${parsed.items ?? 0} items (push ${parsed.push_id ?? ""})`;
      case "item.created":
        return `Created item ${parsed.item_name ?? ""} (${parsed.format ?? ""})`;
      default:
        return eventType;
    }
  } catch {
    return eventType;
  }
}

export default function ActivityTab() {
  const { agent, search } = useFilter();
  const api = useApi();

  const entries = useMemo(() => {
    // Sort audit log descending by time
    let items = [...api.auditLog].sort(
      (a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime(),
    );
    if (agent) items = items.filter((e) => e.actor === agent);
    if (search) {
      const q = search.toLowerCase();
      items = items.filter(
        (e) =>
          e.event_type.toLowerCase().includes(q) ||
          e.data.toLowerCase().includes(q) ||
          getDisplayName(e.actor).toLowerCase().includes(q),
      );
    }
    return items;
  }, [api.auditLog, agent, search]);

  // Group by date
  const grouped = useMemo(() => {
    const groups: { label: string; entries: typeof entries }[] = [];
    let currentLabel = "";
    for (const entry of entries) {
      const label = dateLabel(entry.created_at);
      if (label !== currentLabel) {
        groups.push({ label, entries: [] });
        currentLabel = label;
      }
      groups[groups.length - 1].entries.push(entry);
    }
    return groups;
  }, [entries]);

  if (api.loading) {
    return <p className="py-8 text-center text-sm text-muted-foreground">Loading activity...</p>;
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <p className="text-sm text-muted-foreground">
          {api.connected
            ? `${entries.length} audit log entries from the Bowrain API`
            : "Not connected to Bowrain API"}
        </p>
        <Button
          variant="ghost"
          size="sm"
          render={
            <a
              href="https://dev.bowrain.cloud/activity"
              target="_blank"
              rel="noopener noreferrer"
            />
          }
          className="gap-1.5 text-muted-foreground hover:text-foreground"
        >
          View full feed in Bowrain
          <ExternalLink className="h-3.5 w-3.5" />
        </Button>
      </div>

      {grouped.length === 0 ? (
        <p className="py-8 text-center text-sm text-muted-foreground">No activity found.</p>
      ) : (
        grouped.map((group) => (
          <div key={group.label}>
            <div className="sticky top-0 z-10 mb-2 bg-background/95 backdrop-blur">
              <span className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">
                {group.label}
              </span>
            </div>
            <div className="space-y-1.5">
              {group.entries.map((entry) => (
                <div
                  key={entry.id}
                  className="flex items-start gap-3 rounded-md px-3 py-2 transition-colors hover:bg-accent/30"
                >
                  <span className="font-mono text-[11px] text-muted-foreground/60 mt-0.5 shrink-0 w-14">
                    {formatTime(entry.created_at)}
                  </span>
                  <Badge variant="secondary" className="text-[10px] shrink-0 mt-0.5">
                    {getDisplayName(entry.actor)}
                  </Badge>
                  <div className="min-w-0 flex-1">
                    <p className="text-xs text-foreground/80 leading-relaxed">
                      {formatEventDescription(entry.event_type, entry.data)}
                    </p>
                    <Badge variant="outline" className="text-[9px] mt-1">
                      {entry.event_type}
                    </Badge>
                  </div>
                </div>
              ))}
            </div>
          </div>
        ))
      )}
    </div>
  );
}
