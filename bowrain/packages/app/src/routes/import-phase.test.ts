import { describe, it, expect } from "vite-plus/test";
import { importPhaseFromStatus } from "./import-phase";

const base = { lastSync: "", errors: [] as string[] };

describe("importPhaseFromStatus", () => {
  it("is pending before the first status arrives", () => {
    expect(importPhaseFromStatus(undefined)).toBe("pending");
  });

  it("is pending while the connector has never synced", () => {
    expect(importPhaseFromStatus(base)).toBe("pending");
    // Go's zero time marshals as a year-1 timestamp; the panel treats it as
    // "never synced" and so must the import phase.
    expect(importPhaseFromStatus({ ...base, lastSync: "0001-01-01T00:00:00Z" })).toBe("pending");
  });

  it("is pending on an unparseable timestamp", () => {
    expect(importPhaseFromStatus({ ...base, lastSync: "not-a-date" })).toBe("pending");
  });

  it("is ready once a real first sync is stamped", () => {
    expect(importPhaseFromStatus({ ...base, lastSync: "2026-07-18T09:30:00Z" })).toBe("ready");
  });

  it("is failed when the connector reports a sync error", () => {
    expect(importPhaseFromStatus({ ...base, errors: ["clone failed: boom"] })).toBe("failed");
  });

  it("prefers failed over ready when both are present (stale sync + new error)", () => {
    expect(
      importPhaseFromStatus({ lastSync: "2026-07-18T09:30:00Z", errors: ["fetch failed"] }),
    ).toBe("failed");
  });
});
