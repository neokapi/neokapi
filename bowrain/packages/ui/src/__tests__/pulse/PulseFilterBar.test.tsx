import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { PulseFilterBar } from "../../components/pulse";

describe("PulseFilterBar", () => {
  it("renders filter tokens", () => {
    render(
      <PulseFilterBar
        filters={[
          { key: "language", value: "fr-FR" },
          { key: "project", value: "web-app" },
        ]}
        onRemove={() => {}}
        onClear={() => {}}
      />,
    );
    expect(screen.getByText("fr-FR")).toBeTruthy();
    expect(screen.getByText("web-app")).toBeTruthy();
  });

  it("calls onRemove when clicking X", () => {
    const onRemove = vi.fn();
    render(
      <PulseFilterBar
        filters={[{ key: "language", value: "fr-FR" }]}
        onRemove={onRemove}
        onClear={() => {}}
      />,
    );
    const buttons = screen.getAllByRole("button");
    fireEvent.click(buttons[0]);
    expect(onRemove).toHaveBeenCalledWith("language");
  });

  it("shows Clear all when filters present", () => {
    render(
      <PulseFilterBar
        filters={[{ key: "language", value: "fr-FR" }]}
        onRemove={() => {}}
        onClear={() => {}}
      />,
    );
    expect(screen.getByText("Clear all")).toBeTruthy();
  });

  it("renders presets when no filters", () => {
    render(
      <PulseFilterBar
        filters={[]}
        onRemove={() => {}}
        onClear={() => {}}
        presets={[{ label: "This week", filters: [{ key: "time", value: "this-week" }] }]}
      />,
    );
    expect(screen.getByText("This week")).toBeTruthy();
  });
});
