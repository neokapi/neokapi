import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { CodedTextDisplay } from "@neokapi/ui-primitives";
import type { SpanInfo } from "@neokapi/ui-primitives";

describe("CodedTextDisplay", () => {
  it("renders plain text when no spans are provided", () => {
    render(<CodedTextDisplay text="Hello world" />);
    expect(screen.getByText("Hello world")).toBeInTheDocument();
  });

  it("renders plain text when spans is undefined", () => {
    render(<CodedTextDisplay text="Fallback text" codedText="Coded\uE001text" />);
    // Without spans, falls back to plain text
    expect(screen.getByText("Fallback text")).toBeInTheDocument();
  });

  it("renders plain text when spans array is empty", () => {
    render(<CodedTextDisplay text="Plain text" codedText="Coded\uE001text" spans={[]} />);
    expect(screen.getByText("Plain text")).toBeInTheDocument();
  });

  it("renders tag chips when spans are present", () => {
    const spans: SpanInfo[] = [
      {
        span_type: "opening",
        type: "bold",
        id: "b1",
        data: "<b>",
        display_text: "b",
      },
    ];
    // codedText: "Hello " + MARKER_OPENING + " world"
    const codedText = "Hello \uE001 world";

    render(<CodedTextDisplay text="Hello world" codedText={codedText} spans={spans} />);

    // The tag chip should render with data-tag-chip attribute
    const chip = document.querySelector("[data-tag-chip]");
    expect(chip).toBeInTheDocument();
    // The container should contain both text segments
    const wrapper = chip!.parentElement!;
    expect(wrapper.textContent).toContain("Hello");
    expect(wrapper.textContent).toContain("world");
  });

  it("renders multiple tag chips for multiple spans", () => {
    const spans: SpanInfo[] = [
      { span_type: "opening", type: "bold", id: "b1", data: "<b>" },
      { span_type: "closing", type: "bold", id: "b1", data: "</b>" },
    ];
    const codedText = "Hello \uE001bold\uE002 end";

    render(<CodedTextDisplay text="Hello bold end" codedText={codedText} spans={spans} />);

    const chips = document.querySelectorAll("[data-tag-chip]");
    expect(chips).toHaveLength(2);
  });

  it("applies custom className", () => {
    const { container } = render(<CodedTextDisplay text="test" className="text-red-500" />);
    const span = container.querySelector("span");
    expect(span?.className).toContain("text-red-500");
  });

  it("renders placeholder spans", () => {
    const spans: SpanInfo[] = [
      {
        span_type: "placeholder",
        type: "image",
        id: "img1",
        data: "<img/>",
        display_text: "img",
      },
    ];
    const codedText = "Before \uE003 after";

    render(<CodedTextDisplay text="Before after" codedText={codedText} spans={spans} />);

    const chip = document.querySelector("[data-tag-chip]");
    expect(chip).toBeInTheDocument();
  });
});
