import { useCallback, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  useApi,
  Badge,
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  type ModelSweepMeasurement,
} from "@neokapi/ui";
import { modelRecommendationsQueryOptions } from "../../queries";

/**
 * Model quality — measured steerability. Bowrain periodically translates a
 * small trap-fixture set derived from this project's own content twice per
 * candidate model (with the project's full brand context, and bare) and
 * scores both deterministically. The panel shows, per locale, each model's
 * context adherence and lift, marks the recommended winner, and offers an
 * explicit refresh — gated on the instance-wide model_sweeps.enabled flag,
 * surfaced by the API as `enabled` so the button can disable with a reason.
 * No polling: fresh measurements appear on the next visit or refetch.
 */
export function ModelQualityCard({ ws, projectId }: { ws: string; projectId: string }) {
  const adapter = useApi();
  const queryClient = useQueryClient();
  const { data, isPending } = useQuery(modelRecommendationsQueryOptions(adapter, ws, projectId));
  const [refreshState, setRefreshState] = useState<"idle" | "queueing" | "queued" | "failed">(
    "idle",
  );

  const enabled = data?.enabled === true;
  const locales = data?.locales ?? [];

  const handleRefresh = useCallback(async () => {
    setRefreshState("queueing");
    try {
      await adapter.refreshModelRecommendations(ws, projectId);
      setRefreshState("queued");
      void queryClient.invalidateQueries({ queryKey: ["modelRecommendations", ws, projectId] });
    } catch {
      setRefreshState("failed");
    }
  }, [adapter, ws, projectId, queryClient]);

  return (
    <Card>
      <CardHeader>
        <CardTitle>Model quality</CardTitle>
        <CardDescription>
          How much each candidate model actually follows this project's brand context (voice,
          terminology, and protected terms), measured on segments from your own content. Lift is the
          gain over running the same model with no context.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {isPending ? (
          <p className="text-sm text-muted-foreground">Loading measurements…</p>
        ) : locales.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No measurements yet. Refresh to run the first sweep across this project's target
            languages.
          </p>
        ) : (
          locales.map((group) => (
            <div key={group.locale} className="space-y-2">
              <p className="text-sm font-medium">{group.locale}</p>
              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="border-b border-border text-left text-xs text-muted-foreground">
                      <th className="py-1.5 pr-3 font-medium">Model</th>
                      <th className="py-1.5 pr-3 font-medium">Adherence</th>
                      <th className="py-1.5 pr-3 font-medium">Lift</th>
                      <th className="py-1.5 pr-3 font-medium">Measured</th>
                      <th className="py-1.5 font-medium" />
                    </tr>
                  </thead>
                  <tbody>
                    {group.models.map((m) => (
                      <ModelRow key={m.model} measurement={m} />
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          ))
        )}

        <div className="flex items-center justify-between border-t border-border/50 pt-3">
          <p className="text-xs text-muted-foreground">
            {/* Say nothing about the gate until it is actually known. */}
            {!isPending &&
              (enabled
                ? "Sweeps run on the platform provider and never use your credits."
                : "Model sweeps are disabled on this instance.")}
            {refreshState === "queued" && " Sweep queued. Results appear once it completes."}
            {refreshState === "failed" && " Refresh failed. Try again."}
          </p>
          <Button
            size="sm"
            variant="outline"
            onClick={handleRefresh}
            disabled={!enabled || refreshState === "queueing"}
            aria-label="Refresh model measurements"
          >
            {refreshState === "queueing" ? "Queueing…" : "Refresh"}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

function ModelRow({ measurement: m }: { measurement: ModelSweepMeasurement }) {
  return (
    <tr className="border-b border-border/40 last:border-0">
      <td className="py-1.5 pr-3 font-mono text-xs">{m.model}</td>
      <td className="py-1.5 pr-3">{formatPercent(m.adherence)}</td>
      <td className="py-1.5 pr-3">{formatLift(m.lift)}</td>
      <td className="py-1.5 pr-3 text-xs text-muted-foreground">
        {new Date(m.measured_at).toLocaleDateString()}
      </td>
      <td className="py-1.5 text-right">
        {m.recommended && <Badge variant="secondary">Recommended</Badge>}
      </td>
    </tr>
  );
}

function formatPercent(v: number): string {
  return `${Math.round(v * 100)}%`;
}

function formatLift(v: number): string {
  const pts = Math.round(v * 100);
  return `${pts >= 0 ? "+" : ""}${pts} pts`;
}
