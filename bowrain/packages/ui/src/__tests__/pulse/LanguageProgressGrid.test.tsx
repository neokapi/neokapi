import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { LanguageProgressGrid } from "../../components/pulse";
import { mockLanguages } from "../../stories/pulse/pulse-fixtures";

describe("LanguageProgressGrid", () => {
  it("renders all language cards", () => {
    render(<LanguageProgressGrid languages={mockLanguages} />);
    expect(screen.getByText("fr-FR")).toBeTruthy();
    expect(screen.getByText("de-DE")).toBeTruthy();
    expect(screen.getByText("ja-JP")).toBeTruthy();
    expect(screen.getByText("es-ES")).toBeTruthy();
    expect(screen.getByText("pt-BR")).toBeTruthy();
  });

  it("shows empty state when no languages provided", () => {
    render(<LanguageProgressGrid languages={[]} />);
    expect(screen.getByText("No languages configured yet.")).toBeTruthy();
  });
});
