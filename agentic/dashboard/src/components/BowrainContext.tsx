import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { ExternalLink } from 'lucide-react';
import { useFilter } from '@/context/FilterContext';
import { workspaces } from '@/data/workspaces';

export default function BowrainContext() {
  const { workspace } = useFilter();

  const activeWs = workspace
    ? workspaces.find((w) => w.id === workspace)
    : workspaces.find((w) => w.status === 'active');

  const wsName = activeWs?.name ?? 'Excalidraw';
  const wsSlug = activeWs?.slug ?? 'excalidraw-l10n';
  const bowrainUrl = `https://dev.bowrain.cloud/${wsSlug}`;

  // Mock localization data that would come from Bowrain
  const progressData = [
    { locale: 'fr-FR', translated: 420, total: 600 },
    { locale: 'de-DE', translated: 310, total: 600 },
  ];
  const tmEntries = 85;
  const lastActivity = '2h ago';

  return (
    <Card className="flex h-full flex-col">
      <CardHeader>
        <CardTitle className="text-sm">Bowrain Context</CardTitle>
      </CardHeader>
      <CardContent className="flex-1 space-y-3">
        <div className="text-xs text-muted-foreground">
          <span className="font-medium text-foreground">{wsName}</span>
          {' '}&middot; {activeWs?.upstream ?? 'excalidraw/excalidraw'}
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
                  style={{ width: `${(p.translated / p.total) * 100}%` }}
                />
              </div>
            </div>
          ))}
        </div>

        <div className="flex items-center justify-between text-xs text-muted-foreground">
          <span>TM entries: {tmEntries}</span>
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
