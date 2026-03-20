import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { ExternalLink } from 'lucide-react';
import { useApi } from '@/context/ApiContext';

export default function BowrainContext() {
  const api = useApi();

  const ws = api.workspaces[0];
  const wsName = ws?.name ?? 'No workspace';
  const wsSlug = ws?.slug ?? '';
  const bowrainUrl = wsSlug
    ? `https://dev.bowrain.cloud/${wsSlug}`
    : 'https://dev.bowrain.cloud';

  const progressData = api.progress;

  const lastActivity = api.auditLog.length > 0
    ? formatTimeAgo(api.auditLog[0].created_at)
    : '--';

  return (
    <Card className="flex h-full flex-col">
      <CardHeader>
        <CardTitle className="text-sm">
          Bowrain Context
          {api.connected && (
            <span className="ml-2 inline-block h-2 w-2 rounded-full bg-green-500" title="Connected to Bowrain API" />
          )}
          {!api.connected && !api.loading && (
            <span className="ml-2 inline-block h-2 w-2 rounded-full bg-yellow-500" title="Not connected" />
          )}
        </CardTitle>
      </CardHeader>
      <CardContent className="flex-1 space-y-3">
        <div className="text-xs text-muted-foreground">
          <span className="font-medium text-foreground">{wsName}</span>
          {ws && <span> &middot; {wsSlug}</span>}
        </div>

        {/* Translation progress */}
        {progressData.length > 0 ? (
          <div className="space-y-2">
            {progressData.map((p) => (
              <div key={p.locale} className="space-y-1">
                <div className="flex items-center justify-between text-[11px]">
                  <span className="font-mono text-muted-foreground">{p.locale}</span>
                  <span className="font-mono tabular-nums">
                    {p.translated}/{p.total} blocks
                  </span>
                </div>
                <div className="h-1.5 w-full rounded-full bg-muted">
                  <div
                    className="h-full rounded-full bg-chart-1"
                    style={{ width: `${p.total > 0 ? (p.translated / p.total) * 100 : 0}%` }}
                  />
                </div>
              </div>
            ))}
          </div>
        ) : (
          <p className="text-xs text-muted-foreground/60">
            {api.loading ? 'Loading...' : 'No translation data available'}
          </p>
        )}

        <div className="flex items-center justify-between text-xs text-muted-foreground">
          <span>{api.auditLog.length} events</span>
          <span>Last: {lastActivity}</span>
        </div>

        <Button
          variant="outline"
          size="sm"
          className="w-full gap-1.5"
          render={
            <a
              href={bowrainUrl}
              target="_blank"
              rel="noopener noreferrer"
            />
          }
        >
          Open in Bowrain
          <ExternalLink className="h-3.5 w-3.5" />
        </Button>
      </CardContent>
    </Card>
  );
}

function formatTimeAgo(isoDate: string): string {
  const diff = Date.now() - new Date(isoDate).getTime();
  const mins = Math.floor(diff / 60_000);
  if (mins < 1) return 'just now';
  if (mins < 60) return `${mins}m ago`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}
