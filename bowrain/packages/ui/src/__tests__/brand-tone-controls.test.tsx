import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ToneSpectrumSelector } from "../brand/ToneSpectrumSelector";
import { formalitySpectrum, emotionSpectrum } from "../brand/data/tone-spectrums";

// ---------------------------------------------------------------------------
// ToneSpectrumSelector
// ---------------------------------------------------------------------------

describe("ToneSpectrumSelector", () => {
  it("renders all options with correct labels", () => {
    render(<ToneSpectrumSelector options={formalitySpectrum} value="neutral" onChange={vi.fn()} />);
    for (const option of formalitySpectrum) {
      expect(screen.getByText(option.label)).toBeInTheDocument();
    }
  });

  it("selected option has aria-checked true", () => {
    render(<ToneSpectrumSelector options={formalitySpectrum} value="formal" onChange={vi.fn()} />);
    const formalButton = screen.getByRole("radio", { name: "Formal" });
    expect(formalButton).toHaveAttribute("aria-checked", "true");

    const casualButton = screen.getByRole("radio", { name: "Casual" });
    expect(casualButton).toHaveAttribute("aria-checked", "false");
  });

  it("clicking an option calls onChange with the value", async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();
    render(
      <ToneSpectrumSelector options={formalitySpectrum} value="neutral" onChange={handleChange} />,
    );
    await user.click(screen.getByText("Formal"));
    expect(handleChange).toHaveBeenCalledWith("formal");
  });

  it("shows description and example text for selected option", () => {
    const selected = formalitySpectrum.find((o) => o.value === "casual")!;
    render(<ToneSpectrumSelector options={formalitySpectrum} value="casual" onChange={vi.fn()} />);
    expect(screen.getByText(selected.description)).toBeInTheDocument();
    expect(screen.getByText(`\u201C${selected.exampleText}\u201D`)).toBeInTheDocument();
  });

  it("renders label when provided", () => {
    render(
      <ToneSpectrumSelector
        options={formalitySpectrum}
        value="neutral"
        onChange={vi.fn()}
        label="Formality"
      />,
    );
    expect(screen.getByText("Formality")).toBeInTheDocument();
  });

  it("does not render label when not provided", () => {
    render(<ToneSpectrumSelector options={formalitySpectrum} value="neutral" onChange={vi.fn()} />);
    // No label text should appear outside of the option buttons
    expect(screen.queryByText("Formality")).toBeNull();
  });

  it("works with different spectrum data sets", () => {
    render(<ToneSpectrumSelector options={emotionSpectrum} value="warm" onChange={vi.fn()} />);
    expect(screen.getByText("Warm")).toBeInTheDocument();
    expect(screen.getByText("Neutral")).toBeInTheDocument();
    expect(screen.getByText("Authoritative")).toBeInTheDocument();
  });
});
