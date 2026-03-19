import { Card, CardContent } from '@/components/ui/card';
import { sessions } from '@/data/sessions';

export default function StatsRow() {
  const now = Date.now();
  const dayMs = 24 * 3_600_000;

  const todaySessions = sessions.filter(
    (s) => now - new Date(s.startTime).getTime() < dayMs
  );

  const jobsToday = todaySessions.length;

  const totalSessions = sessions.length;
  const succeededSessions = sessions.filter((s) => s.status === 'succeeded').length;
  const successRate = totalSessions > 0
    ? Math.round((succeededSessions / totalSessions) * 100)
    : 0;

  const aiSpendToday = todaySessions.reduce((sum, s) => sum + s.costUsd, 0);

  // Mock queue depth
  const queueDepth = 3;

  const stats = [
    { label: 'Jobs Today', value: String(jobsToday), detail: 'executions' },
    { label: 'Success Rate', value: `${successRate}%`, detail: 'all time' },
    { label: 'AI Spend', value: `$${aiSpendToday.toFixed(2)}`, detail: 'today' },
    { label: 'Queue Depth', value: String(queueDepth), detail: 'pending' },
  ];

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
