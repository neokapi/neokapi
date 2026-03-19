import { Card, CardContent } from '@/components/ui/card';
import { useApi } from '@/context/ApiContext';
import { sessions } from '@/data/sessions';

export default function StatsRow() {
  const api = useApi();

  const now = Date.now();
  const dayMs = 24 * 3_600_000;

  // When connected, derive stats from real audit log; otherwise use mock sessions
  if (api.connected) {
    const todayEvents = api.auditLog.filter(
      (e) => now - new Date(e.created_at).getTime() < dayMs
    );
    const totalEvents = api.auditLog.length;
    const translationEvents = api.auditLog.filter(
      (e) => e.event_type === 'block.target.updated'
    );
    const totalBlocks = api.progress.reduce((sum, p) => sum + p.total, 0);
    const translatedBlocks = api.progress.reduce((sum, p) => sum + p.translated, 0);
    const coverage = totalBlocks > 0 ? Math.round((translatedBlocks / totalBlocks) * 100) : 0;

    const stats = [
      { label: 'Events Today', value: String(todayEvents.length), detail: 'audit entries' },
      { label: 'Coverage', value: `${coverage}%`, detail: `${translatedBlocks}/${totalBlocks} blocks` },
      { label: 'Translations', value: String(translationEvents.length), detail: 'block updates' },
      { label: 'Total Events', value: String(totalEvents), detail: 'all time' },
    ];

    return <StatsGrid stats={stats} />;
  }

  // Fallback to mock data
  const todaySessions = sessions.filter(
    (s) => now - new Date(s.startTime).getTime() < dayMs
  );
  const totalSessions = sessions.length;
  const succeededSessions = sessions.filter((s) => s.status === 'succeeded').length;
  const successRate = totalSessions > 0
    ? Math.round((succeededSessions / totalSessions) * 100)
    : 0;
  const aiSpendToday = todaySessions.reduce((sum, s) => sum + s.costUsd, 0);

  const stats = [
    { label: 'Jobs Today', value: String(todaySessions.length), detail: 'executions' },
    { label: 'Success Rate', value: `${successRate}%`, detail: 'all time' },
    { label: 'AI Spend', value: `$${aiSpendToday.toFixed(2)}`, detail: 'today' },
    { label: 'Queue Depth', value: '3', detail: 'pending' },
  ];

  return <StatsGrid stats={stats} />;
}

function StatsGrid({ stats }: { stats: { label: string; value: string; detail: string }[] }) {
  return (
    <div className="grid grid-cols-2 gap-4 md:grid-cols-4">
      {stats.map((stat) => (
        <Card key={stat.label}>
          <CardContent className="pt-6">
            <div className="text-2xl font-bold tabular-nums">{stat.value}</div>
            <p className="text-xs text-muted-foreground">{stat.label}</p>
            <p className="text-[10px] text-muted-foreground/60">{stat.detail}</p>
          </CardContent>
        </Card>
      ))}
    </div>
  );
}
