import { Badge } from "@/components/ui/badge";
import { GitCommit, FileText } from "lucide-react";
import { useEffect, useState } from "react";
import { fetchMemoryLog, type MemoryLogEntry } from "@/lib/api";

export default function MemoryTab() {
  const [entries, setEntries] = useState<MemoryLogEntry[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    fetchMemoryLog(50).then((data) => {
      setEntries(data);
      setLoading(false);
    });
  }, []);

  if (loading) {
    return (
      <div className="space-y-3">
        <p className="text-sm text-muted-foreground">
          Git log of agent memory — what agents learned per session
        </p>
        <p className="text-xs text-muted-foreground/60">Loading...</p>
      </div>
    );
  }

  if (entries.length === 0) {
    return (
      <div className="space-y-3">
        <p className="text-sm text-muted-foreground">
          Git log of agent memory — what agents learned per session
        </p>
        <div className="rounded-lg border px-6 py-12 text-center">
          <p className="text-sm font-medium text-muted-foreground">
            No memory commits yet
          </p>
          <p className="mt-1 text-xs text-muted-foreground/60">
            Agent memory entries will appear here after agents run sessions and
            commit observations to the fleet repo.
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-3">
      <p className="text-sm text-muted-foreground">
        Git log of agent memory — what agents learned per session
      </p>
      <div className="space-y-2">
        {entries.map((entry, i) => (
          <div
            key={`${entry.sha}-${i}`}
            className="flex items-start gap-3 rounded-lg border p-3"
          >
            <GitCommit className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" />
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2">
                <Badge variant="outline" className="text-[10px]">
                  {entry.agent}
                </Badge>
                <code className="text-[10px] text-muted-foreground">
                  {entry.sha}
                </code>
                <span className="text-[10px] text-muted-foreground/60">
                  {formatTimestamp(entry.timestamp)}
                </span>
              </div>
              <p className="mt-1 text-xs">{entry.message}</p>
              {entry.files && entry.files.length > 0 && (
                <div className="mt-1 flex flex-wrap gap-1">
                  {entry.files.map((f) => (
                    <span
                      key={f}
                      className="inline-flex items-center gap-1 text-[10px] text-muted-foreground/60"
                    >
                      <FileText className="h-3 w-3" />
                      {f.split("/").pop()}
                    </span>
                  ))}
                </div>
              )}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

function formatTimestamp(ts: string): string {
  try {
    const d = new Date(ts);
    const now = Date.now();
    const diff = now - d.getTime();
    if (diff < 3600_000) return `${Math.round(diff / 60_000)}m ago`;
    if (diff < 86400_000) return `${Math.round(diff / 3600_000)}h ago`;
    return d.toLocaleDateString();
  } catch {
    return ts;
  }
}
