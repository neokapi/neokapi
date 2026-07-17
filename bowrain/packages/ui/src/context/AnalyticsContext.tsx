import { createContext, useContext, useMemo, type ReactNode } from "react";

/**
 * Product-analytics capture seam for shared components (epic 018, workstream
 * B). Components call `useAnalytics().capture(event, props)` at mutation/submit
 * call sites; the host app decides where the events go. The web shell bridges
 * this to PostHog through the platform adapter (`platform.analytics.capture`);
 * the desktop shell mounts no provider, so every capture is a no-op — the same
 * omit-and-no-op contract as the platform seam's `analytics` member.
 *
 * Captures are fire-and-forget and must never block or fail a user operation.
 * Event names come from `analytics-events.ts` (never inline literals) and
 * props carry ids/locales/enum-ish values only — no content, no free text.
 */
export type AnalyticsCaptureFn = (event: string, props?: Record<string, unknown>) => void;

export interface AnalyticsContextValue {
  capture: AnalyticsCaptureFn;
}

const noop: AnalyticsCaptureFn = () => {};

const AnalyticsContext = createContext<AnalyticsContextValue>({ capture: noop });

export function AnalyticsProvider({
  capture,
  children,
}: {
  /** Undefined = analytics unwired for this host (all captures no-op). */
  capture?: AnalyticsCaptureFn;
  children: ReactNode;
}) {
  const value = useMemo<AnalyticsContextValue>(() => ({ capture: capture ?? noop }), [capture]);
  return <AnalyticsContext.Provider value={value}>{children}</AnalyticsContext.Provider>;
}

/** Always safe to call — resolves to a no-op capture without a provider. */
export function useAnalytics(): AnalyticsContextValue {
  return useContext(AnalyticsContext);
}
