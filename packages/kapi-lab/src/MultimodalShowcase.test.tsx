// @vitest-environment jsdom
import { afterEach, describe, it, expect } from "vitest";
import { cleanup, render, screen, fireEvent } from "@testing-library/react";
import MultimodalShowcase from "./MultimodalShowcase";

afterEach(cleanup);

describe("MultimodalShowcase", () => {
  it("renders the image chapter first with its OCR canvas", () => {
    render(<MultimodalShowcase />);
    expect(screen.getByTestId("multimodal-showcase")).toBeTruthy();
    // Chapter 1 = image → MediaCanvas present.
    expect(screen.getByTestId("media-canvas")).toBeTruthy();
    expect(screen.getByText(/Image — OCR/)).toBeTruthy();
  });

  it("advances to the audio chapter (subtitle timeline) via Next", () => {
    render(<MultimodalShowcase />);
    fireEvent.click(screen.getByText("Next →"));
    expect(screen.getByText(/Audio — speech/)).toBeTruthy();
    // Cue text from the audio fixture is shown.
    expect(screen.getByText(/Welcome to the show\./)).toBeTruthy();
  });

  it("jumps directly to a chapter via its tab", () => {
    render(<MultimodalShowcase />);
    fireEvent.click(screen.getByText("3. Video"));
    expect(screen.getByText(/Video — speech \+ on-screen text/)).toBeTruthy();
  });

  it("respects initialChapter", () => {
    render(<MultimodalShowcase initialChapter={1} />);
    expect(screen.getByText(/Audio — speech/)).toBeTruthy();
  });
});
