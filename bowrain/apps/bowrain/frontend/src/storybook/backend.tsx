/**
 * Storybook decorators that mock the desktop's Wails-coupled data layer.
 *
 * The desktop's container pages read server state through two seams, both of
 * which normally reach a live Wails runtime that Storybook has no access to:
 *
 *  1. The `@neokapi/ui` `useApi()` adapter surface — the app implements it with
 *     `WailsApiAdapter` (which proxies the server's REST endpoints over Wails).
 *     Pages: BrandPage, MembersPage. Mocked with a stub `ApiAdapter` supplied
 *     through `ApiProvider`; their react-query `queryFn`s are adapter methods,
 *     so they resolve to fixture data against the preview's QueryClient.
 *
 *  2. The generated Wails `Backend` bindings, called directly inside
 *     react-query `queryFn`s (`Backend.ListFormats()`, `Backend.ListConnectors()`
 *     …). Pages: SettingsPage, ConnectorPanel. Mocked by pre-seeding the
 *     react-query cache with the page's query keys, so the query resolves from
 *     the cache and the binding is never invoked.
 *
 * Both build on the shared, Apache-licensed primitives in
 * `@neokapi/storybook-config` (`makeQueryClientDecorator` + `withQueryClient`);
 * the bowrain-specific glue — the `ApiAdapter`/`Workspace` types and the app's
 * `qk` query keys — lives here.
 */
import type { ComponentType, ReactElement } from "react";
import { ApiProvider, WorkspaceProvider, type ApiAdapter, type Workspace } from "@neokapi/ui";
import { makeQueryClientDecorator, type QuerySeedEntry } from "@neokapi/storybook-config/preview";

/** A connected team workspace fixture (owner role). */
export const mockTeamWorkspace: Workspace = {
  id: "ws-acme",
  name: "Acme",
  slug: "acme",
  description: "",
  logo_url: "",
  type: "team",
  role: "owner",
};

/** The disconnected personal workspace — server/team features are absent. */
export const mockPersonalWorkspace: Workspace = {
  ...mockTeamWorkspace,
  id: "ws-personal",
  name: "Personal",
  slug: "personal",
  type: "personal",
};

/**
 * Build a mock `ApiAdapter` (the `@neokapi/ui` `useApi()` surface the app
 * implements with `WailsApiAdapter`). Only the methods a story exercises need
 * providing; the partial is cast, mirroring the test suites' `makeAdapter`.
 */
export function mockApiAdapter(overrides: Partial<ApiAdapter> = {}): ApiAdapter {
  return { ...overrides } as unknown as ApiAdapter;
}

export interface WorkspaceApiOptions {
  /** Mock adapter backing `useApi()`. Defaults to an empty adapter. */
  adapter?: ApiAdapter;
  /** Active workspace for `useWorkspace()`. Defaults to a connected team. */
  workspace?: Workspace;
}

/**
 * Decorator for the `useApi()`/`useWorkspace()` container pages (BrandPage,
 * MembersPage). Wraps the story in `ApiProvider` (mock adapter) +
 * `WorkspaceProvider`. The preview already supplies a QueryClient and
 * TooltipProvider, so the adapter-backed queries resolve to the mock data.
 */
export function withWorkspaceApi(options: WorkspaceApiOptions = {}) {
  const { adapter = mockApiAdapter(), workspace = mockTeamWorkspace } = options;
  return function WithWorkspaceApi(Story: ComponentType): ReactElement {
    return (
      <ApiProvider adapter={adapter}>
        <WorkspaceProvider initialWorkspace={workspace}>
          <Story />
        </WorkspaceProvider>
      </ApiProvider>
    );
  };
}

/**
 * Decorator for the Wails-`Backend` container pages (SettingsPage,
 * ConnectorPanel) whose react-query `queryFn`s call the generated bindings
 * directly. Pre-seeds the given query keys with fixture data via the shared
 * `makeQueryClientDecorator`, so the bindings are never invoked and the page
 * renders its loaded state without a Wails runtime. Pass seed entries built
 * from the app's `qk` query-key factory.
 */
export function withBackend(seed: QuerySeedEntry[] = []) {
  return makeQueryClientDecorator({ seed });
}
