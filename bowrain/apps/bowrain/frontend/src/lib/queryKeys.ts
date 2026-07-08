/**
 * Query-key factory for the Bowrain desktop react-query usage.
 *
 * Centralising the keys keeps invalidation (`hooks/useInvalidateOnEvent`) and
 * the query sites in agreement — a backend event names the domain to
 * invalidate, and the components read the same key here.
 *
 * The shared @neokapi/ui brand-hub views own their own keys (`["concepts", ws]`
 * etc., invalidated by BrandHubFreshness); this factory covers only the
 * desktop-owned read sites that used to hand-roll useEffect + useState.
 */
export const qk = {
  // Static registries (System Info tab)
  formats: () => ["formats"] as const,
  tools: () => ["tools"] as const,
  flows: (projectId: string) => ["flows", projectId] as const,
  flowDefinitions: (projectId: string) => ["flow-definitions", projectId] as const,
  toolSchema: (name: string) => ["tool-schema", name] as const,
  plugins: () => ["plugins"] as const,
  version: () => ["version"] as const,

  // AI providers (Settings)
  providerConfigs: () => ["provider-configs"] as const,

  // Connectors
  connectorTypes: () => ["connector-types"] as const,
  connectors: () => ["connectors"] as const,
  connectorContent: (id: string) => ["connector-content", id] as const,
  connectorStatus: (id: string) => ["connector-status", id] as const,

  // Governance (members) — workspace-scoped
  members: (ws: string) => ["members", ws] as const,

  // Brand review surface (correction-learning loop, AD-019) — workspace-scoped
  brandProfiles: (ws: string) => ["brand-profiles", ws] as const,
  brandCandidates: (ws: string, profileId: string, all: boolean) =>
    ["brand-candidates", ws, profileId, all] as const,
  brandDrift: (ws: string, projectId: string) => ["brand-drift", ws, projectId] as const,

  // Server connect
  defaultServerURL: () => ["default-server-url"] as const,

  // Editor collaboration session
  collabSession: () => ["collab-session"] as const,
} as const;
