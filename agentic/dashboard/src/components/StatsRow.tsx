import { Card, CardContent } from '@/components/ui/card';
import { workspaces } from '../data/workspaces';
import { sessions } from '../data/sessions';
import { issues } from '../data/issues';

export default function StatsRow() {
  const activeWorkspaces = workspaces.filter((w) => w.status === 'active').length;

  const now = Date.now();
  const weekMs = 7 * 24 * 3_600_000;
  const dayMs = 24 * 3_600_000;

  const sessionsThisWeek = sessions.filter(
    (s) => now - new Date(s.startTime).getTime() < weekMs
  ).length;

  const toolCallsToday = sessions
    .filter((s) => now - new Date(s.startTime).getTime() < dayMs)
    .reduce((sum, s) => sum + s.toolCalls.length, 0);

  const totalIssues = issues.filter((i) => i.status === 'open').length;

  const stats = [
    { label: 'Active Workspaces', value: activeWorkspaces },
    { label: 'Sessions This Week', value: sessionsThisWeek },
    { label: 'Tool Calls Today', value: toolCallsToday },
    { label: 'Open Issues', value: totalIssues },
  ];

  return (
    <div className="grid grid-cols-2 gap-4 md:grid-cols-4">
      {stats.map((stat) => (
        <Card key={stat.label}>
          <CardContent className="p-4">
            <div className="font-mono text-2xl font-bold">{stat.value}</div>
            <div className="mt-1 text-xs text-muted-foreground">{stat.label}</div>
          </CardContent>
        </Card>
      ))}
    </div>
  );
}
