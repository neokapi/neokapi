import { useQuery } from "@tanstack/react-query";
import { getMetrics } from "../api";
import { MetricsCards } from "../components/MetricsCards";

export function DashboardRoute() {
  const { data: metrics, isLoading } = useQuery({
    queryKey: ["admin", "metrics"],
    queryFn: () => getMetrics(),
  });

  return (
    <div className="space-y-6">
      <h2 className="text-xl font-semibold">Dashboard</h2>
      <MetricsCards metrics={metrics ?? null} loading={isLoading} />
    </div>
  );
}
