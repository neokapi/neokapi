import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { PulseSettings } from "../../components/pulse";

const BASE = "https://pulse.example.com";

describe("PulseSettings", () => {
  it("renders all three visibility options", () => {
    render(
      <PulseSettings
        workspaceSlug="acme"
        visibility="private"
        pulseBaseUrl={BASE}
        onVisibilityChange={vi.fn()}
      />,
    );
    expect(screen.getByText("Private")).toBeTruthy();
    expect(screen.getByText("Unlisted")).toBeTruthy();
    expect(screen.getByText("Public")).toBeTruthy();
  });

  it("marks the current visibility as selected", () => {
    render(
      <PulseSettings
        workspaceSlug="acme"
        visibility="unlisted"
        pulseBaseUrl={BASE}
        onVisibilityChange={vi.fn()}
      />,
    );
    const radios = screen.getAllByRole("radio");
    expect(radios[0]).toHaveAttribute("aria-checked", "false");
    expect(radios[1]).toHaveAttribute("aria-checked", "true");
    expect(radios[2]).toHaveAttribute("aria-checked", "false");
  });

  it("calls onVisibilityChange when selecting a different option", async () => {
    const user = userEvent.setup();
    const handler = vi.fn(async () => {});
    render(
      <PulseSettings
        workspaceSlug="acme"
        visibility="private"
        pulseBaseUrl={BASE}
        onVisibilityChange={handler}
      />,
    );
    await user.click(screen.getByText("Public"));
    expect(handler).toHaveBeenCalledWith("public");
  });

  it("does not call handler when clicking the already-selected option", async () => {
    const user = userEvent.setup();
    const handler = vi.fn(async () => {});
    render(
      <PulseSettings
        workspaceSlug="acme"
        visibility="private"
        pulseBaseUrl={BASE}
        onVisibilityChange={handler}
      />,
    );
    await user.click(screen.getByText("Private"));
    expect(handler).not.toHaveBeenCalled();
  });

  it("hides dashboard URL when visibility is private", () => {
    render(
      <PulseSettings
        workspaceSlug="acme"
        visibility="private"
        pulseBaseUrl={BASE}
        onVisibilityChange={vi.fn()}
      />,
    );
    expect(screen.queryByText("Dashboard URL")).toBeNull();
  });

  it("shows slug URL when visibility is public", () => {
    render(
      <PulseSettings
        workspaceSlug="acme"
        visibility="public"
        pulseBaseUrl={BASE}
        onVisibilityChange={vi.fn()}
      />,
    );
    expect(screen.getByText("Dashboard URL")).toBeTruthy();
    expect(screen.getByText(`${BASE}/acme`)).toBeTruthy();
  });

  it("shows access key URL when visibility is unlisted", () => {
    render(
      <PulseSettings
        workspaceSlug="acme"
        visibility="unlisted"
        accessKey="abc123secret"
        pulseBaseUrl={BASE}
        onVisibilityChange={vi.fn()}
      />,
    );
    expect(screen.getByText(`${BASE}/abc123secret`)).toBeTruthy();
  });

  it("falls back to slug when unlisted but no access key", () => {
    render(
      <PulseSettings
        workspaceSlug="acme"
        visibility="unlisted"
        pulseBaseUrl={BASE}
        onVisibilityChange={vi.fn()}
      />,
    );
    expect(screen.getByText(`${BASE}/acme`)).toBeTruthy();
  });

  it("has an external link to the dashboard when accessible", () => {
    render(
      <PulseSettings
        workspaceSlug="acme"
        visibility="public"
        pulseBaseUrl={BASE}
        onVisibilityChange={vi.fn()}
      />,
    );
    const link = screen.getByLabelText("Open Pulse dashboard");
    expect(link).toBeTruthy();
    expect(link.getAttribute("href")).toBe(`${BASE}/acme`);
    expect(link.getAttribute("target")).toBe("_blank");
  });
});
