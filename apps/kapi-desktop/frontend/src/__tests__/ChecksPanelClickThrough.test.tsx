import { render, screen } from "./testUtils";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { ChecksPanel } from "../components/ChecksPanel";
import { ErrorProvider } from "../components/ErrorBanner";
import type { CheckRunResult } from "../types/api";

const result: CheckRunResult = {
  pass: false,
  score: 72,
  files: [
    {
      path: "/p/docs/help/billing.json",
      findings: [
        {
          category: "placeholder",
          severity: "critical",
          message: "The target drops {count}.",
          block_id: "b2",
          field: "target",
          fixable: false,
          rule: "{count}",
          point: "",
          collection: "App",
        },
        {
          category: "voice",
          severity: "major",
          message: 'Say "sign in" rather than "log in".',
          original_text: "log in",
          block_id: "b1",
          field: "source",
          replacement: "sign in",
          fixable: true,
          rule: "log in",
          point: "support/docs",
          collection: "Docs",
        },
      ],
    },
  ],
};

describe("ChecksPanel click-through", () => {
  it("names the rule that fired", () => {
    render(
      <ErrorProvider>
        <ChecksPanel tabID="t1" result={result} />
      </ErrorProvider>,
    );
    const rules = screen.getAllByTestId("finding-rule");
    expect(rules[0]).toHaveTextContent("{count}");
    expect(rules[1]).toHaveTextContent("log in");
  });

  it("opens Context at the finding's point, carrying the rule", async () => {
    const onOpenContext = vi.fn();
    render(
      <ErrorProvider>
        <ChecksPanel tabID="t1" result={result} onOpenContext={onOpenContext} />
      </ErrorProvider>,
    );
    const points = screen.getAllByTestId("finding-point");
    // The backend sorts critical first, so the placeholder finding leads.
    expect(points[1]).toHaveTextContent("support/docs");
    await userEvent.click(points[1]);
    expect(onOpenContext).toHaveBeenCalledWith({
      coordinate: "support/docs",
      collection: "Docs",
      path: "/p/docs/help/billing.json",
      rule: "log in",
    });
  });

  it("names the project's own point rather than leaving it blank", async () => {
    const onOpenContext = vi.fn();
    render(
      <ErrorProvider>
        <ChecksPanel tabID="t1" result={result} onOpenContext={onOpenContext} />
      </ErrorProvider>,
    );
    const points = screen.getAllByTestId("finding-point");
    expect(points[0]).toHaveTextContent("project default");
    await userEvent.click(points[0]);
    expect(onOpenContext).toHaveBeenCalledWith({
      coordinate: undefined,
      collection: "App",
      path: "/p/docs/help/billing.json",
      rule: "{count}",
    });
  });

  it("renders without the click-through when no handler is given", () => {
    render(
      <ErrorProvider>
        <ChecksPanel tabID="t1" result={result} />
      </ErrorProvider>,
    );
    expect(screen.queryByTestId("finding-point")).not.toBeInTheDocument();
    expect(screen.getAllByTestId("finding-rule")).toHaveLength(2);
  });
});
