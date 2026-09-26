import { describe, it, expect } from "vitest";
import { rungFor } from "../components/CollectionsPanel";
import type { LocaleCoverage } from "../types/api";

const scope = (over: Partial<LocaleCoverage>): LocaleCoverage => ({
  locale: "nb",
  collection: "docs",
  total: 10,
  pct: { translated: 100, established: 0 },
  gated: false,
  shippable: true,
  ...over,
});

// A coverage cell reads "Shippable" only for a scope that clears a ship gate. A
// scope no gate matches shows how far along the ladder it has come instead.
describe("rungFor", () => {
  it("never calls a scope no gate matches shippable", () => {
    const rung = rungFor(scope({ shipState: "not_gated" }));
    expect(rung.key).not.toBe("shippable");
    expect(rung.label).toBe("Draft");
  });

  it("shows a reviewed scope no gate matches as in review", () => {
    const rung = rungFor(
      scope({ shipState: "not_gated", pct: { translated: 100, established: 40 } }),
    );
    expect(rung.label).toBe("In review");
  });

  it("calls a scope that clears its gate shippable", () => {
    expect(rungFor(scope({ gated: true, shipState: "shippable" })).key).toBe("shippable");
  });
});
