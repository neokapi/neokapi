import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { PulseProjectCard } from "../../components/pulse";

describe("PulseProjectCard", () => {
  const defaultProps = {
    name: "Web Application",
    sourceLanguage: "en-US",
    targetLanguages: ["fr-FR", "de-DE"],
    totalWords: 20000,
    translatedWords: 15000,
    percentage: 75,
  };

  it("renders project name", () => {
    render(<PulseProjectCard {...defaultProps} />);
    expect(screen.getByText("Web Application")).toBeTruthy();
  });

  it("shows language pair", () => {
    render(<PulseProjectCard {...defaultProps} />);
    expect(screen.getByText("English → French, German")).toBeTruthy();
  });

  it("calls onClick when clicked", () => {
    const onClick = vi.fn();
    render(<PulseProjectCard {...defaultProps} onClick={onClick} />);
    fireEvent.click(screen.getByRole("button"));
    expect(onClick).toHaveBeenCalledOnce();
  });
});
