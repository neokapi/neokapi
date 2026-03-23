import { Card, CardContent } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Filter } from "lucide-react";
import { useFilter } from "@/context/FilterContext";
import { useApi, type AgentProfile } from "@/context/ApiContext";
import { fetchAgentSoul } from "@/lib/api";
import { useState } from "react";
import Markdown from "react-markdown";

interface AgentCardProps {
  agent: AgentProfile;
}

function formatRelativeTime(iso: string): string {
  const diffMs = Date.now() - new Date(iso).getTime();
  const diffMins = Math.floor(diffMs / 60_000);
  if (diffMins < 1) return "just now";
  if (diffMins < 60) return `${diffMins}m ago`;
  const diffHours = Math.floor(diffMins / 60);
  if (diffHours < 24) return `${diffHours}h ago`;
  return `${Math.floor(diffHours / 24)}d ago`;
}

function statusColor(status: string): string {
  switch (status) {
    case "healthy":
      return "bg-green-500";
    case "degraded":
      return "bg-yellow-500";
    case "failing":
      return "bg-red-500";
    default:
      return "bg-muted-foreground/40";
  }
}

export default function AgentCard({ agent }: AgentCardProps) {
  const { addToken } = useFilter();
  const api = useApi();
  const [dialogOpen, setDialogOpen] = useState(false);
  const [soul, setSoul] = useState<string | null>(null);
  const [lastAuthor, setLastAuthor] = useState("");
  const [lastDate, setLastDate] = useState("");
  const [loading, setLoading] = useState(false);

  function handleFilter(e: React.MouseEvent) {
    e.stopPropagation();
    if (api.workspaces.length > 0) {
      const ws = api.workspaces[0];
      addToken({ key: "workspace", value: ws.slug, label: ws.name });
    }
    addToken({ key: "agent", value: agent.agent, label: agent.agent });
  }

  function handleCardClick() {
    setDialogOpen(true);
    if (!soul && !loading) {
      setLoading(true);
      fetchAgentSoul(agent.agent).then((resp) => {
        setSoul(resp.soul);
        setLastAuthor(resp.last_author);
        setLastDate(resp.last_date);
        setLoading(false);
      });
    }
  }

  const successRate =
    agent.total_sessions > 0
      ? Math.round((agent.successful_count / agent.total_sessions) * 100)
      : 0;

  return (
    <>
      <Card
        className="min-w-[220px] max-w-[280px] flex-shrink-0 cursor-pointer transition-colors hover:bg-accent/30"
        onClick={handleCardClick}
      >
        <CardContent className="space-y-2.5 pt-4">
          {/* Name + Role */}
          <div className="flex items-start justify-between gap-2">
            <div className="min-w-0">
              <div className="text-sm font-semibold truncate">
                {agent.agent}
              </div>
            </div>
            <div className="flex items-center gap-1.5 shrink-0">
              <button
                className="inline-flex h-5 w-5 items-center justify-center rounded-md text-muted-foreground hover:bg-accent hover:text-foreground"
                onClick={handleFilter}
                title="Filter by this agent"
              >
                <Filter className="h-3 w-3" />
              </button>
              <Badge variant="secondary" className="text-[10px]">
                {agent.role}
              </Badge>
            </div>
          </div>

          {/* Sessions stats */}
          <div className="flex items-center gap-2 text-[11px] text-muted-foreground">
            <span className="font-mono tabular-nums">
              {agent.total_sessions} sessions
            </span>
            <span>&middot;</span>
            <span className="font-mono tabular-nums">
              {successRate}% success
            </span>
          </div>

          {/* Tokens used */}
          {agent.total_tokens_used > 0 && (
            <div className="text-[11px] text-muted-foreground/70">
              {(agent.total_tokens_used / 1000).toFixed(1)}k tokens used
            </div>
          )}

          {/* Status line */}
          <div className="flex items-center gap-1.5 text-xs">
            <span
              className={`inline-block h-2 w-2 rounded-full ${statusColor(agent.last_status)}`}
            />
            {agent.last_session_at ? (
              <span className="text-muted-foreground">
                Last: {formatRelativeTime(agent.last_session_at)}
              </span>
            ) : (
              <span className="text-muted-foreground">No sessions</span>
            )}
          </div>
        </CardContent>
      </Card>

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>
              {agent.agent}
              <Badge variant="secondary" className="ml-2 text-[10px]">
                {agent.role}
              </Badge>
            </DialogTitle>
            {lastDate && (
              <DialogDescription>
                Last modified {formatRelativeTime(lastDate)}
                {lastAuthor && ` by ${lastAuthor}`}
              </DialogDescription>
            )}
          </DialogHeader>
          <ScrollArea className="max-h-[70vh]">
            {loading ? (
              <p className="text-sm text-muted-foreground py-8 text-center">
                Loading persona...
              </p>
            ) : soul ? (
              <Markdown
                components={{
                  h1: ({ children }) => (
                    <h1 className="mb-3 text-lg font-bold">{children}</h1>
                  ),
                  h2: ({ children }) => (
                    <h2 className="mt-4 mb-2 text-sm font-semibold">{children}</h2>
                  ),
                  h3: ({ children }) => (
                    <h3 className="mt-3 mb-1 text-sm font-medium">{children}</h3>
                  ),
                  p: ({ children }) => (
                    <p className="mb-2 text-sm leading-relaxed text-foreground/80">{children}</p>
                  ),
                  ul: ({ children }) => (
                    <ul className="mb-2 ml-4 list-disc space-y-1 text-sm text-foreground/80">{children}</ul>
                  ),
                  ol: ({ children }) => (
                    <ol className="mb-2 ml-4 list-decimal space-y-1 text-sm text-foreground/80">{children}</ol>
                  ),
                  li: ({ children }) => <li>{children}</li>,
                  strong: ({ children }) => (
                    <strong className="font-semibold text-foreground">{children}</strong>
                  ),
                  code: ({ children }) => (
                    <code className="rounded bg-muted px-1 py-0.5 text-xs font-mono">{children}</code>
                  ),
                }}
              >
                {soul}
              </Markdown>
            ) : (
              <p className="text-sm text-muted-foreground py-8 text-center">
                No persona file found for this agent.
              </p>
            )}
          </ScrollArea>
        </DialogContent>
      </Dialog>
    </>
  );
}
