import { useQuery } from "@tanstack/react-query";
import { useSetBreadcrumb, PageContainer } from "@neokapi/ui";
import { getMetrics } from "../api";
import { MetricsCards } from "../components/MetricsCards";

export function DashboardRoute() {
  useSetBreadcrumb("Dashboard");

  const { data: metrics, isLoading } = useQuery({
    queryKey: ["admin", "metrics"],
    queryFn: () => getMetrics(),
  });

  return (
    <PageContainer width="wide" className="space-y-6">
      <MetricsCards metrics={metrics ?? null} loading={isLoading} />
    </PageContainer>
  );
}
