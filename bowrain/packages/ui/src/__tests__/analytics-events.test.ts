/**
 * Shape gate for the client-side analytics taxonomy (epic 018) — the TS mirror
 * of bowrain/analytics/events_test.go: every event name defined in
 * src/analytics-events.ts must be snake_case domain_action.
 */
import { describe, it, expect } from "vite-plus/test";
import { AnalyticsEvents } from "../analytics-events";

describe("analytics event taxonomy", () => {
  it("uses snake_case domain_action names", () => {
    for (const [key, event] of Object.entries(AnalyticsEvents)) {
      expect(event, `event ${key} must be snake_case`).toMatch(/^[a-z0-9]+(_[a-z0-9]+)+$/);
    }
  });
});
