import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Button,
  Alert,
  AlertDescription,
  useSetBreadcrumb,
  Loader2,
  CircleCheck,
} from "@neokapi/ui";
import { listSlugReservations, releaseSlugReservation } from "../api";

/**
 * SlugReservationsRoute — admin view for the workspace slug-rename grace
 * period. Lists active reservations and lets an operator release one early
 * so the slug becomes immediately reusable.
 */
export function SlugReservationsRoute() {
  useSetBreadcrumb("Slug Reservations");
  const queryClient = useQueryClient();
  const [released, setReleased] = useState<string | null>(null);
  const [error, setError] = useState<string>("");

  const { data, isLoading } = useQuery({
    queryKey: ["admin", "slug-reservations"],
    queryFn: () => listSlugReservations(),
  });

  const release = useMutation({
    mutationFn: (slug: string) => releaseSlugReservation(slug),
    onSuccess: (_, slug) => {
      setError("");
      setReleased(slug);
      void queryClient.invalidateQueries({ queryKey: ["admin", "slug-reservations"] });
    },
    onError: (err) => {
      setError(err instanceof Error ? err.message : "Release failed");
    },
  });

  const reservations = data ?? [];

  return (
    <div className="mx-auto w-full max-w-3xl space-y-4">
      <p className="text-sm text-muted-foreground">
        Active workspace slug rename reservations. Each entry holds a slug for the configured grace
        period (30 days) so it can&apos;t be reused for impersonation. Release a slug to free it for
        immediate reuse.
      </p>

      {released && (
        <Alert>
          <CircleCheck className="h-4 w-4" />
          <AlertDescription>
            Released <code>{released}</code>. It is now available for reuse.
          </AlertDescription>
        </Alert>
      )}

      {error && (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      {isLoading ? (
        <div className="flex items-center gap-2 text-muted-foreground text-sm">
          <Loader2 className="h-4 w-4 animate-spin" /> Loading…
        </div>
      ) : reservations.length === 0 ? (
        <div className="rounded-md border bg-card p-6 text-center text-sm text-muted-foreground">
          No active reservations.
        </div>
      ) : (
        <div className="overflow-x-auto rounded-md border">
          <table className="w-full text-sm">
            <thead className="bg-muted/50 text-left text-xs uppercase text-muted-foreground">
              <tr>
                <th className="px-4 py-2 font-medium">Slug</th>
                <th className="px-4 py-2 font-medium">Workspace</th>
                <th className="px-4 py-2 font-medium">Reserved until</th>
                <th className="px-4 py-2 font-medium">Created</th>
                <th className="px-4 py-2"></th>
              </tr>
            </thead>
            <tbody>
              {reservations.map((r) => {
                const isReleasing = release.isPending && release.variables === r.slug;
                return (
                  <tr key={r.slug} className="border-t">
                    <td className="px-4 py-2 font-mono">{r.slug}</td>
                    <td className="px-4 py-2 font-mono text-xs">{r.workspace_id}</td>
                    <td className="px-4 py-2">{new Date(r.reserved_until).toLocaleString()}</td>
                    <td className="px-4 py-2">{new Date(r.created_at).toLocaleString()}</td>
                    <td className="px-4 py-2 text-right">
                      <Button
                        size="sm"
                        variant="outline"
                        onClick={() => release.mutate(r.slug)}
                        disabled={isReleasing}
                      >
                        {isReleasing ? "Releasing…" : "Release"}
                      </Button>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
