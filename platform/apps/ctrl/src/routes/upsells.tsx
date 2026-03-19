import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { getUpsells } from "../api";
import { UpsellTable } from "../components/UpsellTable";

export function UpsellsRoute() {
  const navigate = useNavigate();

  const { data: upsells, isLoading } = useQuery({
    queryKey: ["admin", "upsells"],
    queryFn: () => getUpsells(),
  });

  return (
    <div className="space-y-4">
      <h2 className="text-xl font-semibold">Upsell Opportunities</h2>
      <p className="text-sm text-muted-foreground">
        Workspaces that are likely candidates for an upgrade, ranked by priority.
      </p>

      <UpsellTable
        upsells={upsells ?? []}
        loading={isLoading}
        onRowClick={(id) =>
          void navigate({ to: "/workspaces/$workspaceId", params: { workspaceId: id } })
        }
      />
    </div>
  );
}
