import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen, fireEvent } from "@testing-library/react";
import { BravoConfigPanel } from "../components/bravo/BravoConfigPanel";
import type { BravoConfig, BravoToolInfo } from "../types/api";

const baseConfig: BravoConfig = {
  workspace_id: "ws-1",
  enabled: true,
  allowed_tools: [],
  denied_tools: ["execute_script"],
  require_approval: ["connector_push"],
  code_exec_enabled: false,
  max_concurrent: 3,
};

const tools: BravoToolInfo[] = [
  { name: "list_projects", require_approval: false },
  { name: "run_flow", require_approval: false },
  { name: "connector_push", require_approval: true },
  { name: "execute_script", require_approval: false },
  { name: "tm_search", require_approval: false },
];

describe("BravoConfigPanel", () => {
  it("renders global settings", () => {
    render(<BravoConfigPanel config={baseConfig} tools={tools} onSave={vi.fn()} />);

    expect(screen.getByText("Enable @bravo")).toBeDefined();
    expect(screen.getByText("Code execution")).toBeDefined();
    expect(screen.getByText("Max concurrent")).toBeDefined();
  });

  it("renders all tool names", () => {
    render(<BravoConfigPanel config={baseConfig} tools={tools} onSave={vi.fn()} />);

    expect(screen.getByText("list_projects")).toBeDefined();
    expect(screen.getByText("run_flow")).toBeDefined();
    expect(screen.getByText("connector_push")).toBeDefined();
    expect(screen.getByText("execute_script")).toBeDefined();
    expect(screen.getByText("tm_search")).toBeDefined();
  });

  it("shows correct initial policies from config", () => {
    const { container } = render(<BravoConfigPanel config={baseConfig} tools={tools} onSave={vi.fn()} />);

    // Policy badges (buttons, not select options)
    const policyBadges = container.querySelectorAll("button[title^='Click to change']");
    const badgeTexts = Array.from(policyBadges).map((b) => b.textContent);

    // list_projects=Allow, run_flow=Allow, connector_push=Approve, execute_script=Deny, tm_search=Allow
    expect(badgeTexts.filter((t) => t === "Allow").length).toBe(3);
    expect(badgeTexts.filter((t) => t === "Approve").length).toBe(1);
    expect(badgeTexts.filter((t) => t === "Deny").length).toBe(1);
  });

  it("does not show save button when nothing changed", () => {
    render(<BravoConfigPanel config={baseConfig} tools={tools} onSave={vi.fn()} />);

    expect(screen.queryByText("Save changes")).toBeNull();
  });

  it("shows save button when a tool policy is changed", () => {
    const { container } = render(<BravoConfigPanel config={baseConfig} tools={tools} onSave={vi.fn()} />);

    // Click first policy badge to cycle it
    const badges = container.querySelectorAll("button[title^='Click to change']");
    fireEvent.click(badges[0]);

    expect(screen.getByText("Save changes")).toBeDefined();
  });

  it("cycles tool policy: Allow → Approve → Deny → Allow", () => {
    const { container } = render(<BravoConfigPanel config={baseConfig} tools={tools} onSave={vi.fn()} />);

    // Get policy badge buttons (not select options)
    const getBadges = () =>
      Array.from(container.querySelectorAll("button[title^='Click to change']"));

    // list_projects starts as "Allow" (first badge)
    expect(getBadges()[0].textContent).toBe("Allow");

    // Click: Allow → Approve
    fireEvent.click(getBadges()[0]);
    expect(getBadges()[0].textContent).toBe("Approve");

    // Click: Approve → Deny
    fireEvent.click(getBadges()[0]);
    expect(getBadges()[0].textContent).toBe("Deny");

    // Click: Deny → Allow
    fireEvent.click(getBadges()[0]);
    expect(getBadges()[0].textContent).toBe("Allow");
  });

  it("saves correct denied_tools and require_approval arrays", () => {
    const onSave = vi.fn();
    const { container } = render(<BravoConfigPanel config={baseConfig} tools={tools} onSave={onSave} />);

    // Change list_projects from Allow → Approve (click once)
    const badges = container.querySelectorAll("button[title^='Click to change']");
    fireEvent.click(badges[0]);

    // Click save
    fireEvent.click(screen.getByText("Save changes"));

    expect(onSave).toHaveBeenCalledTimes(1);
    const saved = onSave.mock.calls[0][0] as Partial<BravoConfig>;

    // execute_script should still be denied
    expect(saved.denied_tools).toContain("execute_script");
    // connector_push should still require approval
    expect(saved.require_approval).toContain("connector_push");
    // list_projects was changed to approve
    expect(saved.require_approval).toContain("list_projects");
  });

  it("allows changing policy via select dropdown", () => {
    const onSave = vi.fn();
    render(<BravoConfigPanel config={baseConfig} tools={tools} onSave={onSave} />);

    // Find the select for the first tool (list_projects)
    const selects = screen.getAllByRole("combobox");
    fireEvent.change(selects[0], { target: { value: "deny" } });

    // Should show save button
    fireEvent.click(screen.getByText("Save changes"));

    const saved = onSave.mock.calls[0][0] as Partial<BravoConfig>;
    expect(saved.denied_tools).toContain("list_projects");
  });

  it("shows save button when global settings change", () => {
    render(<BravoConfigPanel config={baseConfig} tools={tools} onSave={vi.fn()} />);

    // Change max concurrent
    const input = screen.getByDisplayValue("3");
    fireEvent.change(input, { target: { value: "5" } });

    expect(screen.getByText("Save changes")).toBeDefined();
  });

  it("disables save button when saving", () => {
    render(
      <BravoConfigPanel
        config={{ ...baseConfig, max_concurrent: 5 }}
        tools={tools}
        onSave={vi.fn()}
        saving
      />,
    );

    // Change something to show the button
    const input = screen.getByDisplayValue("5");
    fireEvent.change(input, { target: { value: "3" } });

    const saveBtn = screen.getByText("Saving...").closest("button");
    expect(saveBtn?.disabled).toBe(true);
  });

  it("renders tool policies heading with count", () => {
    render(<BravoConfigPanel config={baseConfig} tools={tools} onSave={vi.fn()} />);

    expect(screen.getByText("Tool policies (5)")).toBeDefined();
  });
});
