import { useQuery } from "@tanstack/react-query";
import { useSetBreadcrumb, Card, CardHeader, CardTitle, CardContent, Button } from "@neokapi/ui";
import { RefreshCw, ExternalLink } from "lucide-react";
import {
  getHealth,
  type ComponentStatus,
  type AdminHealth,
  type CircuitBreakerStatus,
} from "../api";

// Map a component/overall status to a colored dot + label. "unconfigured" is
// intentionally neutral (grey), not a failure — many components are optional.
function statusStyle(status: string): { dot: string; text: string } {
  switch (status) {
    case "up":
    case "ready":
      return { dot: "bg-emerald-500", text: "text-emerald-600 dark:text-emerald-400" };
    case "degraded":
      return { dot: "bg-amber-500", text: "text-amber-600 dark:text-amber-400" };
    case "down":
    case "unhealthy":
      return { dot: "bg-red-500", text: "text-red-600 dark:text-red-400" };
    default: // unconfigured / unknown
      return { dot: "bg-muted-foreground/40", text: "text-muted-foreground" };
  }
}

function StatusPill({ status }: { status: string }) {
  const s = statusStyle(status);
  return (
    <span className={`inline-flex items-center gap-1.5 text-sm font-medium ${s.text}`}>
      <span className={`size-2 rounded-full ${s.dot}`} />
      {status}
    </span>
  );
}

function ComponentRow({ name, comp }: { name: string; comp: ComponentStatus }) {
  return (
    <div className="flex items-center justify-between border-b border-border/60 py-2 last:border-0">
      <div className="min-w-0">
        <div className="flex items-center gap-2">
          <span className="font-medium capitalize">{name.replace(/_/g, " ")}</span>
          {comp.type && <span className="text-xs text-muted-foreground">({comp.type})</span>}
        </div>
        {comp.error && <p className="mt-0.5 truncate text-xs text-red-500">{comp.error}</p>}
        {comp.providers && comp.providers.length > 0 && (
          <p className="mt-0.5 text-xs text-muted-foreground">
            {comp.providers
              .map(
                (p) =>
                  `${p.name}${p.model ? ` (${p.model})` : ""}${p.configured ? "" : " (not configured)"}`,
              )
              .join(", ")}
          </p>
        )}
      </div>
      <div className="flex shrink-0 items-center gap-3 pl-3">
        {comp.latency_ms !== undefined && (
          <span className="text-xs tabular-nums text-muted-foreground">{comp.latency_ms} ms</span>
        )}
        <StatusPill status={comp.status} />
      </div>
    </div>
  );
}

// A breaker state maps onto the same three colours the component rows use, so
// "open" reads as red wherever it appears.
function breakerStatusLabel(b: CircuitBreakerStatus): string {
  if (!b.enabled) return "unconfigured";
  switch (b.state) {
    case "open":
      return "down";
    case "half_open":
      return "degraded";
    default:
      return "up";
  }
}

function BreakerRow({ breaker }: { breaker: CircuitBreakerStatus }) {
  return (
    <div className="flex items-center justify-between border-b border-border/60 py-2 last:border-0">
      <div className="min-w-0">
        <div className="flex items-center gap-2">
          <span className="truncate font-mono text-sm font-medium">{breaker.dependency}</span>
          <span className="text-xs text-muted-foreground">({breaker.kind})</span>
        </div>
        <p className="mt-0.5 text-xs text-muted-foreground">
          {!breaker.enabled
            ? "Breakers are switched off, so calls are never rejected."
            : breaker.state === "open"
              ? `Calls are being rejected without being attempted. Next recovery probe in ~${breaker.retry_after_seconds ?? 0}s.`
              : breaker.state === "half_open"
                ? "Probing recovery: a bounded number of calls is being let through."
                : "Calls are passing through normally."}
        </p>
      </div>
      <div className="flex shrink-0 items-center gap-3 pl-3">
        <span className="font-mono text-xs text-muted-foreground">{breaker.state}</span>
        <StatusPill status={breakerStatusLabel(breaker)} />
      </div>
    </div>
  );
}

function BreakersCard({ breakers }: { breakers: CircuitBreakerStatus[] }) {
  const open = breakers.filter((b) => b.state === "open");
  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between">
        <div>
          <CardTitle>Circuit Breakers</CardTitle>
          <p className="mt-1 text-sm text-muted-foreground">
            Outbound dependencies the platform has called. An open circuit means calls are being
            declined rather than attempted, so jobs queue and wait instead of failing.
          </p>
        </div>
        <StatusPill status={open.length > 0 ? "degraded" : "up"} />
      </CardHeader>
      <CardContent>
        {breakers.map((b) => (
          <BreakerRow key={b.dependency} breaker={b} />
        ))}
      </CardContent>
    </Card>
  );
}

function LinkButton({ href, label }: { href?: string; label: string }) {
  if (!href) return null;
  return (
    <Button asChild variant="outline" size="sm">
      <a href={href} target="_blank" rel="noreferrer">
        {label}
        <ExternalLink className="size-3.5" />
      </a>
    </Button>
  );
}

function ServiceCard({
  title,
  status,
  version,
  components,
}: {
  title: string;
  status?: string;
  version?: string;
  components?: Record<string, ComponentStatus>;
}) {
  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between">
        <CardTitle className="flex items-center gap-3">
          {title}
          {version && <span className="text-xs font-normal text-muted-foreground">v{version}</span>}
        </CardTitle>
        {status && <StatusPill status={status} />}
      </CardHeader>
      <CardContent>
        {components && Object.keys(components).length > 0 ? (
          Object.entries(components)
            .sort(([a], [b]) => a.localeCompare(b))
            .map(([name, comp]) => <ComponentRow key={name} name={name} comp={comp} />)
        ) : (
          <p className="text-sm text-muted-foreground">No component detail reported.</p>
        )}
      </CardContent>
    </Card>
  );
}

export function HealthRoute() {
  useSetBreadcrumb("Health");

  const { data, isLoading, isFetching, refetch } = useQuery<AdminHealth>({
    queryKey: ["admin", "health"],
    queryFn: () => getHealth(),
    // Health is live state — refetch on an interval and on focus.
    refetchInterval: 15_000,
    refetchOnWindowFocus: true,
  });

  return (
    <div className="mx-auto w-full max-w-4xl space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold">System Health</h1>
          <p className="text-sm text-muted-foreground">
            Live readiness of the platform services. Drill into logs and traces via the links below.
          </p>
        </div>
        <Button variant="outline" size="sm" onClick={() => void refetch()} disabled={isFetching}>
          <RefreshCw className={`size-3.5 ${isFetching ? "animate-spin" : ""}`} />
          Refresh
        </Button>
      </div>

      {isLoading && <p className="text-sm text-muted-foreground">Checking services…</p>}

      {data && (
        <>
          <ServiceCard
            title="Server"
            status={data.server.status}
            version={data.server.version}
            components={data.server.components}
          />

          {data.worker && (
            <ServiceCard
              title="Worker"
              status={data.worker.reachable ? data.worker.status : "down"}
              components={
                data.worker.reachable
                  ? (data.worker.components as Record<string, ComponentStatus>)
                  : undefined
              }
            />
          )}
          {data.worker && !data.worker.reachable && (
            <p className="-mt-3 text-xs text-red-500">
              Worker unreachable{data.worker.error ? `: ${data.worker.error}` : ""}
            </p>
          )}

          {data.circuit_breakers && data.circuit_breakers.length > 0 && (
            <BreakersCard breakers={data.circuit_breakers} />
          )}

          {(data.links.sentry || data.links.posthog || data.links.cloudwatch) && (
            <Card>
              <CardHeader>
                <CardTitle>Observability</CardTitle>
              </CardHeader>
              <CardContent className="flex flex-wrap gap-2">
                <LinkButton href={data.links.sentry} label="Sentry · errors & traces" />
                <LinkButton href={data.links.posthog} label="PostHog · product & replays" />
                <LinkButton href={data.links.cloudwatch} label="CloudWatch · infra & alarms" />
              </CardContent>
            </Card>
          )}
        </>
      )}
    </div>
  );
}
