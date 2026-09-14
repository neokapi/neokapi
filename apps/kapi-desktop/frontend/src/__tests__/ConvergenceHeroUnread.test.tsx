import { render } from "./testUtils";
import { describe, it, expect, vi } from "vitest";
import { ConvergenceHero } from "../components/ConvergenceHero";
import type { CheckWarning, ConvergePlan, ConvergenceReport } from "../types/api";

const plan: ConvergePlan = {
  plan: {
    flow: "translate",
    scopes: [{ locale: "fr", collection: "app", missingTarget: 1, tmExact: 0, aiRemaining: 1 }],
    totals: { missingTarget: 1, tmExact: 0, aiRemaining: 1, tokenEstimate: 4 },
    note: "",
  },
  changedFiles: 0,
  removedFiles: 0,
  storeMissing: false,
  versionStale: false,
};

const report: ConvergenceReport = {
  locales: [
    {
      locale: "fr",
      collection: "app",
      total: 1,
      pct: { translated: 0 },
      gated: false,
      shippable: true,
    },
  ],
  review: [],
};

const unread: CheckWarning[] = [
  {
    code: "format.no_reader",
    source: "pkg/doc.idml",
    message:
      'no reader for format "okf_idml" is installed, so pkg/doc.idml was not measured; install the plugin that supplies it (kapi plugins install okf_idml)',
  },
];

// A project with a collection in a plugin format the machine lacks converges the
// rest. The hero names the files it left out, so converging it never reads as
// covering the whole project.
describe("ConvergenceHero over content with no installed reader", () => {
  it("names the files the report could not measure", () => {
    render(
      <ConvergenceHero
        tabID="t1"
        onBringUpToDate={vi.fn()}
        convergence={{ ...report, warnings: unread }}
        plan={plan}
      />,
    );
    const notice = document.querySelector("[data-slot='hero-unread']");
    expect(notice).not.toBeNull();
    expect(notice?.textContent).toContain("pkg/doc.idml");
    expect(notice?.textContent).toContain("kapi plugins install okf_idml");
  });

  it("shows no notice when every declared file was read", () => {
    render(
      <ConvergenceHero tabID="t1" onBringUpToDate={vi.fn()} convergence={report} plan={plan} />,
    );
    expect(document.querySelector("[data-slot='hero-unread']")).toBeNull();
  });
});
