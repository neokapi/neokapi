import { Card, CardContent } from "@/components/ui/card";
import { useApi } from "@/context/ApiContext";

export default function StatsRow() {
  const api = useApi();

  if (api.loading) {
    const placeholders = Array.from({ length: 4 }, (_, i) => ({
      label: "...",
      value: "--",
      detail: "loading",
      key: `ph-${i}`,
    }));
    return <StatsGrid stats={placeholders} />;
  }

  const now = Date.now();
  const dayMs = 24 * 3_600_000;

  const todayEvents = api.auditLog.filter((e) => now - new Date(e.created_at).getTime() < dayMs);
  const totalEvents = api.auditLog.length;
  const translationEvents = api.auditLog.filter((e) => e.event_type === "block.target.updated");
  const totalBlocks = api.progress.reduce((sum, p) => sum + p.total, 0);
  const translatedBlocks = api.progress.reduce((sum, p) => sum + p.translated, 0);
  const coverage = totalBlocks > 0 ? Math.round((translatedBlocks / totalBlocks) * 100) : 0;

  const stats = [
    {
      label: "Events Today",
      value: String(todayEvents.length),
      detail: "audit entries",
      key: "events-today",
    },
    {
      label: "Coverage",
      value: `${coverage}%`,
      detail: `${translatedBlocks}/${totalBlocks} blocks`,
      key: "coverage",
    },
    {
      label: "Translations",
      value: String(translationEvents.length),
      detail: "block updates",
      key: "translations",
    },
    { label: "Total Events", value: String(totalEvents), detail: "all time", key: "total-events" },
  ];

  return <StatsGrid stats={stats} />;
}

function StatsGrid({
  stats,
}: {
  stats: { label: string; value: string; detail: string; key: string }[];
}) {
  return (
    <div className="grid grid-cols-2 gap-4 md:grid-cols-4">
      {stats.map((stat) => (
        <Card key={stat.key}>
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
