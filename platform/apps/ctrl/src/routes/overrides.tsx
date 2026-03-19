import { useQuery } from "@tanstack/react-query";
import { Badge } from "@neokapi/ui";
import { listAllOverrides } from "../api";

export function OverridesRoute() {
  const { data: overrides, isLoading } = useQuery({
    queryKey: ["admin", "overrides"],
    queryFn: () => listAllOverrides(),
  });

  if (isLoading) {
    return (
      <div className="space-y-4">
        <h2 className="text-xl font-semibold">Feature Overrides</h2>
        <p className="text-sm text-muted-foreground">Loading...</p>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <h2 className="text-xl font-semibold">Feature Overrides</h2>
      <p className="text-sm text-muted-foreground">
        All per-workspace feature overrides across the platform.
      </p>

      <div className="rounded-lg border">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b bg-muted/50">
              <th className="px-4 py-2 text-left font-medium">Workspace</th>
              <th className="px-4 py-2 text-left font-medium">Feature</th>
              <th className="px-4 py-2 text-left font-medium">Status</th>
              <th className="px-4 py-2 text-left font-medium">Reason</th>
              <th className="px-4 py-2 text-left font-medium">Created By</th>
              <th className="px-4 py-2 text-left font-medium">Expires</th>
            </tr>
          </thead>
          <tbody className="divide-y">
            {(overrides ?? []).map((override) => (
              <tr key={override.id} className="hover:bg-muted/30">
                <td className="px-4 py-2">{override.workspace_name ?? override.workspace_id}</td>
                <td className="px-4 py-2 font-mono text-xs">{override.feature}</td>
                <td className="px-4 py-2">
                  <Badge variant={override.enabled ? "default" : "secondary"}>
                    {override.enabled ? "Enabled" : "Disabled"}
                  </Badge>
                </td>
                <td className="px-4 py-2 text-muted-foreground">{override.reason ?? "--"}</td>
                <td className="px-4 py-2 text-muted-foreground">{override.created_by}</td>
                <td className="px-4 py-2 text-muted-foreground">
                  {override.expires_at
                    ? new Date(override.expires_at).toLocaleDateString()
                    : "Never"}
                </td>
              </tr>
            ))}
            {(!overrides || overrides.length === 0) && (
              <tr>
                <td colSpan={6} className="px-4 py-6 text-center text-muted-foreground">
                  No feature overrides configured.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
