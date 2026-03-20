import { useQuery } from "@tanstack/react-query";
import { useSetBreadcrumb } from "@neokapi/ui";
import { getMetrics } from "../api";
import { MetricsCards } from "../components/MetricsCards";

export function DashboardRoute() {
  useSetBreadcrumb("Dashboard");

  const { data: metrics, isLoading } = useQuery({
    queryKey: ["admin", "metrics"],
    queryFn: () => getMetrics(),
  });

  return (
    <div className="mx-auto w-full max-w-5xl space-y-6">
      <MetricsCards metrics={metrics ?? null} loading={isLoading} />
    </div>
  );
}
