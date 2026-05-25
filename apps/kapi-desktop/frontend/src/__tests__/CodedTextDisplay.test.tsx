import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { CodedTextDisplay } from "@neokapi/ui-primitives";
import type { Run } from "@neokapi/ui-primitives";

describe("CodedTextDisplay", () => {
  it("renders plain text when no runs are provided", () => {
    render(<CodedTextDisplay text="Hello world" />);
    expect(screen.getByText("Hello world")).toBeInTheDocument();
  });

  it("renders plain text when runs is undefined", () => {
    render(<CodedTextDisplay text="Fallback text" />);
    expect(screen.getByText("Fallback text")).toBeInTheDocument();
  });

  it("renders plain text when runs array is empty", () => {
    render(<CodedTextDisplay text="Plain text" runs={[]} />);
    expect(screen.getByText("Plain text")).toBeInTheDocument();
  });

  it("renders a tag chip when an opening run is present", () => {
    const runs: Run[] = [
      { text: "Hello " },
      { pcOpen: { id: "b1", type: "bold", data: "<b>", equiv: "b", disp: "b" } },
      { text: " world" },
    ];

    render(<CodedTextDisplay text="Hello world" runs={runs} />);

    // The tag chip should render with data-tag-chip attribute
    const chip = document.querySelector("[data-tag-chip]");
    expect(chip).toBeInTheDocument();
    // The container should contain both text segments
    const wrapper = chip!.parentElement!;
    expect(wrapper.textContent).toContain("Hello");
    expect(wrapper.textContent).toContain("world");
  });

  it("renders multiple tag chips for paired runs", () => {
    const runs: Run[] = [
      { text: "Hello " },
      { pcOpen: { id: "b1", type: "bold", data: "<b>", equiv: "b" } },
      { text: "bold" },
      { pcClose: { id: "b1", type: "bold", data: "</b>", equiv: "b" } },
      { text: " end" },
    ];

    render(<CodedTextDisplay text="Hello bold end" runs={runs} />);

    const chips = document.querySelectorAll("[data-tag-chip]");
    expect(chips).toHaveLength(2);
  });

  it("applies custom className", () => {
    const { container } = render(<CodedTextDisplay text="test" className="text-red-500" />);
    const span = container.querySelector("span");
    expect(span?.className).toContain("text-red-500");
  });

  it("renders placeholder runs", () => {
    const runs: Run[] = [
      { text: "Before " },
      { ph: { id: "img1", type: "image", data: "<img/>", equiv: "img", disp: "img" } },
      { text: " after" },
    ];

    render(<CodedTextDisplay text="Before after" runs={runs} />);

    const chip = document.querySelector("[data-tag-chip]");
    expect(chip).toBeInTheDocument();
  });
});
