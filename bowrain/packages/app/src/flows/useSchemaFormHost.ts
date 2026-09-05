import { useCallback, useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { useApi } from "@neokapi/ui";
import type { ProviderConfig, SchemaFormCredential, SchemaFormHost } from "@neokapi/ui";

const NO_PROVIDERS: ProviderConfig[] = [];

/**
 * The schema-form host for the platform's flow steps.
 *
 * A credential picker in a step's options offers the workspace's saved
 * provider configurations, the credentials a server-run flow can use. The
 * picker reads the list synchronously, so it is fetched once and cached; a
 * reader without the permission to list providers sees the picker as a text
 * input. The platform has no file dialog, so `onBrowse` stays unset and path
 * widgets degrade to text inputs as well.
 */
export function useSchemaFormHost(workspaceSlug: string): SchemaFormHost {
  const api = useApi();
  const { data } = useQuery({
    queryKey: ["providers", workspaceSlug],
    queryFn: () => api.listProviderConfigs(workspaceSlug),
    staleTime: 30_000,
    // Listing providers takes a permission, and a refusal is the same on retry.
    retry: false,
  });
  const providers = data ?? NO_PROVIDERS;

  const credentials = useCallback<NonNullable<SchemaFormHost["credentials"]>>(
    (resourceKind?: string): SchemaFormCredential[] => {
      // resourceKind, when present, scopes the list to a provider type
      // (e.g. "anthropic"). Otherwise every saved provider is offered.
      const scoped = resourceKind
        ? providers.filter((p) => p.provider_type === resourceKind)
        : providers;
      return scoped.map((p) => ({
        value: p.name,
        label: p.model ? `${p.name} (${p.model})` : p.name,
      }));
    },
    [providers],
  );

  return useMemo<SchemaFormHost>(() => ({ credentials }), [credentials]);
}
