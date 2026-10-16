import { describe, it, expect } from "vite-plus/test";
import { render, screen } from "./testUtils";

import { WorkspaceRules } from "../components/WorkspaceRules";
import { WORKSPACE_RULES } from "../stories/fixtures/standings";

const all = (name: string) => Array.from(document.querySelectorAll(`[data-slot='${name}']`));
const day = (iso: string) =>
  new Date(iso).toLocaleDateString(undefined, { day: "numeric", month: "long" });

describe("the rules that hold in every project", () => {
  it("reads each rule as a sentence with who decided it, where and when", () => {
    render(<WorkspaceRules rules={WORKSPACE_RULES} />);
    expect(screen.getByText("Holds in every project")).toBeInTheDocument();
    const rows = all("workspace-rule");
    expect(rows).toHaveLength(2);
    expect(rows[0].textContent).toContain("Write sign in, not log in");
    expect(all("workspace-rule-how").map((p) => p.textContent)).toEqual([
      `kept by you · decided in Acme Help Center · ${day(WORKSPACE_RULES[0].at)}`,
      `merged in #412 · decided in KapiMart · ${day(WORKSPACE_RULES[1].at)}`,
    ]);
  });

  it("draws nothing when no rule holds in every project", () => {
    render(<WorkspaceRules rules={[]} />);
    expect(all("workspace-rules")).toHaveLength(0);
    expect(screen.queryByText("Holds in every project")).not.toBeInTheDocument();
  });
});
