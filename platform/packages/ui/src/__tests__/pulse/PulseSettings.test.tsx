import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { PulseSettings } from "../../components/pulse";

describe("PulseSettings", () => {
  it("renders all three visibility options", () => {
    render(
      <PulseSettings workspaceSlug="acme" visibility="private" onVisibilityChange={vi.fn()} />,
    );
    expect(screen.getByText("Private")).toBeTruthy();
    expect(screen.getByText("Unlisted")).toBeTruthy();
    expect(screen.getByText("Public")).toBeTruthy();
  });

  it("marks the current visibility as selected", () => {
    render(
      <PulseSettings workspaceSlug="acme" visibility="unlisted" onVisibilityChange={vi.fn()} />,
    );
    const radios = screen.getAllByRole("radio");
    expect(radios[0]).toHaveAttribute("aria-checked", "false"); // private
    expect(radios[1]).toHaveAttribute("aria-checked", "true"); // unlisted
    expect(radios[2]).toHaveAttribute("aria-checked", "false"); // public
  });

  it("calls onVisibilityChange when selecting a different option", async () => {
    const user = userEvent.setup();
    const handler = vi.fn(async () => {});
    render(
      <PulseSettings workspaceSlug="acme" visibility="private" onVisibilityChange={handler} />,
    );
    await user.click(screen.getByText("Public"));
    expect(handler).toHaveBeenCalledWith("public");
  });

  it("does not call handler when clicking the already-selected option", async () => {
    const user = userEvent.setup();
    const handler = vi.fn(async () => {});
    render(
      <PulseSettings workspaceSlug="acme" visibility="private" onVisibilityChange={handler} />,
    );
    await user.click(screen.getByText("Private"));
    expect(handler).not.toHaveBeenCalled();
  });

  it("hides dashboard URL when visibility is private", () => {
    render(
      <PulseSettings workspaceSlug="acme" visibility="private" onVisibilityChange={vi.fn()} />,
    );
    expect(screen.queryByText("Dashboard URL")).toBeNull();
  });

  it("shows dashboard URL when visibility is unlisted", () => {
    render(
      <PulseSettings workspaceSlug="acme" visibility="unlisted" onVisibilityChange={vi.fn()} />,
    );
    expect(screen.getByText("Dashboard URL")).toBeTruthy();
    expect(screen.getByText("https://pulse.bowrain.cloud/acme")).toBeTruthy();
  });

  it("shows dashboard URL when visibility is public", () => {
    render(<PulseSettings workspaceSlug="acme" visibility="public" onVisibilityChange={vi.fn()} />);
    expect(screen.getByText("https://pulse.bowrain.cloud/acme")).toBeTruthy();
  });

  it("uses custom pulseBaseUrl when provided", () => {
    render(
      <PulseSettings
        workspaceSlug="acme"
        visibility="public"
        pulseBaseUrl="https://pulse.example.com"
        onVisibilityChange={vi.fn()}
      />,
    );
    expect(screen.getByText("https://pulse.example.com/acme")).toBeTruthy();
  });

  it("has an external link to the dashboard when accessible", () => {
    render(<PulseSettings workspaceSlug="acme" visibility="public" onVisibilityChange={vi.fn()} />);
    const link = screen.getByLabelText("Open Pulse dashboard");
    expect(link).toBeTruthy();
    expect(link.getAttribute("href")).toBe("https://pulse.bowrain.cloud/acme");
    expect(link.getAttribute("target")).toBe("_blank");
  });
});
