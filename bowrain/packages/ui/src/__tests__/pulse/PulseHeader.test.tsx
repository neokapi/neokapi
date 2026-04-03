import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { PulseHeader } from "../../components/pulse";

describe("PulseHeader", () => {
  it("renders workspace name", () => {
    render(<PulseHeader workspaceName="Acme Corp" />);
    expect(screen.getByText("Acme Corp")).toBeTruthy();
  });

  it("shows initial letter when no logo provided", () => {
    render(<PulseHeader workspaceName="Acme Corp" />);
    expect(screen.getByText("A")).toBeTruthy();
  });

  it("renders img element when logoUrl is provided", () => {
    const { container } = render(
      <PulseHeader workspaceName="Acme Corp" logoUrl="https://example.com/logo.png" />,
    );
    const img = container.querySelector("img");
    expect(img).toBeTruthy();
    expect(img?.getAttribute("src")).toBe("https://example.com/logo.png");
    expect(img?.getAttribute("alt")).toBe("Acme Corp");
  });

  it("has a theme toggle button", () => {
    render(<PulseHeader workspaceName="Acme Corp" />);
    expect(screen.getByRole("button", { name: "Toggle theme" })).toBeTruthy();
  });
});
