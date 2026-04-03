import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { StarterPackPicker } from "../brand/StarterPackPicker";
import { StarterPackCard } from "../brand/StarterPackCard";
import { starterPacks } from "../brand/data/starter-packs";

// ---------------------------------------------------------------------------
// StarterPackCard
// ---------------------------------------------------------------------------

describe("StarterPackCard", () => {
  const pack = starterPacks[0]; // Professional B2B

  it("renders pack label and tagline", () => {
    render(<StarterPackCard pack={pack} onClick={vi.fn()} />);
    expect(screen.getByText(pack.label)).toBeInTheDocument();
    expect(screen.getByText(pack.tagline)).toBeInTheDocument();
  });

  it("renders personality tags", () => {
    render(<StarterPackCard pack={pack} onClick={vi.fn()} />);
    for (const tag of pack.personalityTags) {
      expect(screen.getByText(tag)).toBeInTheDocument();
    }
  });

  it("renders formality badge", () => {
    render(<StarterPackCard pack={pack} onClick={vi.fn()} />);
    expect(screen.getByText(pack.formality)).toBeInTheDocument();
  });

  it("calls onClick with the pack when clicked", async () => {
    const user = userEvent.setup();
    const handleClick = vi.fn();
    render(<StarterPackCard pack={pack} onClick={handleClick} />);
    await user.click(screen.getByText(pack.label));
    expect(handleClick).toHaveBeenCalledWith(pack);
  });

  it("renders sample text", () => {
    render(<StarterPackCard pack={pack} onClick={vi.fn()} />);
    expect(screen.getByText(`\u201C${pack.sampleText}\u201D`)).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// StarterPackPicker
// ---------------------------------------------------------------------------

describe("StarterPackPicker", () => {
  const defaultProps = {
    open: true,
    onOpenChange: vi.fn(),
    onSelect: vi.fn(),
    onScratch: vi.fn(),
  };

  it("renders all 5 pack cards plus Start from Scratch when open", () => {
    render(<StarterPackPicker {...defaultProps} />);
    for (const pack of starterPacks) {
      expect(screen.getByText(pack.label)).toBeInTheDocument();
    }
    expect(screen.getByText("Start from Scratch")).toBeInTheDocument();
  });

  it("clicking a pack card calls onSelect with the pack object", async () => {
    const user = userEvent.setup();
    const onSelect = vi.fn();
    render(<StarterPackPicker {...defaultProps} onSelect={onSelect} />);
    await user.click(screen.getByText(starterPacks[1].label));
    expect(onSelect).toHaveBeenCalledWith(starterPacks[1]);
  });

  it("clicking Start from Scratch calls onScratch", async () => {
    const user = userEvent.setup();
    const onScratch = vi.fn();
    render(<StarterPackPicker {...defaultProps} onScratch={onScratch} />);
    await user.click(screen.getByText("Start from Scratch"));
    expect(onScratch).toHaveBeenCalled();
  });

  it("does not render dialog content when open is false", () => {
    render(<StarterPackPicker {...defaultProps} open={false} />);
    expect(screen.queryByText("Choose a Starting Point")).toBeNull();
    expect(screen.queryByText("Start from Scratch")).toBeNull();
  });

  it("renders the dialog title and description when open", () => {
    render(<StarterPackPicker {...defaultProps} />);
    expect(screen.getByText("Choose a Starting Point")).toBeInTheDocument();
    expect(
      screen.getByText("Pick a template to get started quickly, or create from scratch."),
    ).toBeInTheDocument();
  });
});
