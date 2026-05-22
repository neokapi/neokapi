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

import { useCallback, useEffect, useMemo, useState } from "react";
import type { SchemaFormHost, SchemaFormCredential } from "@neokapi/ui-primitives";
import type { ProviderConfig } from "../types/api";
import { api } from "./useApi";
import { useWailsEvent } from "./useWailsEvent";

export function useSchemaFormHost(): SchemaFormHost {
  const [providers, setProviders] = useState<ProviderConfig[]>([]);

  const refresh = useCallback(() => {
    api
      .listProviders()
      .then((list) => {
        if (list) setProviders(list);
      })
      .catch(() => {
        // No credential store available (Storybook/vitest) — leave empty so the
        // picker degrades to a text input.
      });
  }, []);

  useEffect(() => {
    refresh();
  }, [refresh]);

  // Saving/removing a provider re-keys the registries; keep the cache fresh.
  useWailsEvent("registries-changed", refresh);

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
