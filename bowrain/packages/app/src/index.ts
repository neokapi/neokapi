// @neokapi/bowrain-app — the shared Bowrain SPA shell.
//
// The web entry (bowrain/apps/web) and, in Phase 2, the Wails desktop shell
// mount the same route tree by rendering <BowrainApp> with their own ApiAdapter
// and PlatformAdapter. The stylesheet lives at "@neokapi/bowrain-app/styles.css".

export { BowrainApp, type BowrainAppProps } from "./BowrainApp";
export { createBowrainRouter } from "./routes";
export type { RouterContext, WorkspaceRouteContext } from "./routes";
export { PlatformProvider, usePlatform, webPlatform, type PlatformAdapter } from "./platform";
