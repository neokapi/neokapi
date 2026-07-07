/**
 * Builds the schema-form host for Kapi Desktop.
 *
 * The shared `@neokapi/ui-primitives` SchemaForm exposes host-injectable
 * capabilities (file/folder pickers, credential pickers) via a `SchemaFormHost`.
 * Hosts without a filesystem or credential store (the docs website, Storybook)
 * omit the host and the widgets degrade to plain text inputs. Kapi Desktop, a
 * Wails app, wires both capabilities to the native backend:
 *
 *   - `onBrowse` -> the generic `BrowsePath` Wails dialog (the same dialog API
 *     that backs OpenProjectDialog / AddFilesDialog).
 *   - `credentials` -> the OS-keychain-backed provider vault surfaced by
 *     `ListProviders`. Because the picker reads credentials synchronously, the
 *     provider list is prefetched and cached, then refreshed when the backend
 *     emits a registries change.
 */

import { useCallback, useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import type { SchemaFormHost, SchemaFormCredential } from "@neokapi/ui-primitives";
import { api } from "./useApi";
import { qk } from "../lib/queryKeys";
import { useInvalidateOnEvent } from "./useInvalidateOnEvent";

export function useSchemaFormHost(): SchemaFormHost {
  // The credential picker reads the provider list synchronously, so it is
  // prefetched via react-query (shared cache with any other provider reader) and
  // refreshed when a registries change re-keys the vault. Outside Wails the
  // list is empty and the picker degrades to a text input.
  const { data } = useQuery({
    queryKey: qk.providers(),
    queryFn: () => api.listProviders(),
    staleTime: 30_000,
  });
  const providers = data ?? [];

  // Saving/removing a provider re-keys the registries; keep the cache fresh.
  useInvalidateOnEvent("registries-changed", [qk.providers()]);

  const onBrowse = useCallback<NonNullable<SchemaFormHost["onBrowse"]>>(async (request) => {
    const picked = await api.browsePath({
      kind: request.kind,
      field: request.field,
      currentValue: request.currentValue,
      title: request.title,
      forSaveAs: request.forSaveAs,
      filters: request.filters,
      accepts: request.accepts,
    });
    // `call` returns null outside Wails and "" when the user cancels; the
    // widget treats both as "no selection".
    return picked && picked !== "" ? picked : null;
  }, []);

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

  return useMemo<SchemaFormHost>(() => ({ onBrowse, credentials }), [onBrowse, credentials]);
}
