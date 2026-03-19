import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { ExternalLink } from 'lucide-react';
import { useFilter } from '@/context/FilterContext';
import { useApi } from '@/context/ApiContext';
import { workspaces as staticWorkspaces } from '@/data/workspaces';

export default function BowrainContext() {
  const { workspace } = useFilter();
  const api = useApi();

  // Prefer real API data, fall back to static data
  const activeStaticWs = workspace
    ? staticWorkspaces.find((w) => w.id === workspace)
    : staticWorkspaces.find((w) => w.status === 'active');

  const wsName = api.connected
    ? (api.workspaces[0]?.name ?? activeStaticWs?.name ?? 'Excalidraw')
    : (activeStaticWs?.name ?? 'Excalidraw');
  const wsSlug = api.connected
    ? (api.workspaces[0]?.slug ?? 'excalidraw-l10n')
    : (activeStaticWs?.slug ?? 'excalidraw-l10n');
  const bowrainUrl = `https://dev.bowrain.cloud/${wsSlug}`;

  // Use real progress data if connected, otherwise static mock
  const progressData = api.connected && api.progress.length > 0
    ? api.progress
    : [
        { locale: 'fr-FR', translated: 420, total: 600 },
        { locale: 'de-DE', translated: 310, total: 600 },
      ];

  const lastActivity = api.connected && api.auditLog.length > 0
    ? formatTimeAgo(api.auditLog[0].created_at)
    : '—';

  return (
    <Card className="flex h-full flex-col">
      <CardHeader>
        <CardTitle className="text-sm">
          Bowrain Context
          {api.connected && (
            <span className="ml-2 inline-block h-2 w-2 rounded-full bg-green-500" title="Connected to Bowrain API" />
          )}
          {!api.connected && !api.loading && (
            <span className="ml-2 inline-block h-2 w-2 rounded-full bg-yellow-500" title="Using cached data" />
          )}
        </CardTitle>
      </CardHeader>
      <CardContent className="flex-1 space-y-3">
        <div className="text-xs text-muted-foreground">
          <span className="font-medium text-foreground">{wsName}</span>
          {' '}&middot; {activeStaticWs?.upstream ?? 'excalidraw/excalidraw'}
        </div>

        {/* Translation progress */}
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

        <div className="flex items-center justify-between text-xs text-muted-foreground">
          <span>
            {api.connected
              ? `${api.auditLog.length} events`
              : 'TM entries: 85'}
          </span>
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
