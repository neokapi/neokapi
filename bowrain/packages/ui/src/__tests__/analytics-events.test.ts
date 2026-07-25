/**
 * Drift gate for the client-side analytics taxonomy (epic 018) — the TS
 * mirror of bowrain/analytics/events_test.go: every event name defined in
 * src/analytics-events.ts must be documented in the analytics event
 * reference, and every name must be snake_case domain_action.
 */
import { describe, it, expect } from "vite-plus/test";
import { readFileSync } from "node:fs";
import { resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { AnalyticsEvents } from "../analytics-events";

// bowrain/packages/ui/src/__tests__ → repo root is five levels up.
const docPath = resolve(
  dirname(fileURLToPath(import.meta.url)),
  "../../../../..",
  "web/docs/contribute/implementation/analytics-events.md",
);

describe("analytics event taxonomy", () => {
  it("documents every client event in the analytics event reference", () => {
    const doc = readFileSync(docPath, "utf8");
    for (const [key, event] of Object.entries(AnalyticsEvents)) {
      expect(doc, `event ${key} (${event}) is missing from ${docPath}`).toContain(`\`${event}\``);
    }
  });

  it("uses snake_case domain_action names", () => {
    for (const [key, event] of Object.entries(AnalyticsEvents)) {
      expect(event, `event ${key} must be snake_case`).toMatch(/^[a-z0-9]+(_[a-z0-9]+)+$/);
    }
  });
});
