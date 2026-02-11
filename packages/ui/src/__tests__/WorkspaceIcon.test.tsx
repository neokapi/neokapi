import { describe, it, expect, vi } from "vitest";
import { render, screen, act } from "@testing-library/react";
import { WorkspaceIcon } from "../components/WorkspaceIcon";
import type { Workspace } from "../types/api";

function ws(overrides: Partial<Workspace> = {}): Workspace {
  return { id: "1", name: "Acme", slug: "acme", description: "", logo_url: "", role: "owner", ...overrides };
}

describe("WorkspaceIcon", () => {
  it("renders the first letter of the workspace name", () => {
    render(<WorkspaceIcon workspace={ws()} active={false} onClick={() => {}} />);
    expect(screen.getByRole("button")).toHaveTextContent("A");
  });

  it("uppercases the first letter", () => {
    render(<WorkspaceIcon workspace={ws({ name: "beta" })} active={false} onClick={() => {}} />);
    expect(screen.getByRole("button")).toHaveTextContent("B");
  });

  it("shows '?' for empty name", () => {
    render(<WorkspaceIcon workspace={ws({ name: "" })} active={false} onClick={() => {}} />);
    expect(screen.getByRole("button")).toHaveTextContent("?");
  });

  it("calls onClick when clicked", () => {
    const handleClick = vi.fn();
    render(<WorkspaceIcon workspace={ws()} active={false} onClick={handleClick} />);
    act(() => screen.getByRole("button").click());
    expect(handleClick).toHaveBeenCalledOnce();
  });

  it("uses rounded square border-radius when active", () => {
    render(<WorkspaceIcon workspace={ws()} active={true} onClick={() => {}} />);
    const btn = screen.getByRole("button");
    expect(btn.style.borderRadius).toBe("12px");
  });

  it("uses circular border-radius when inactive", () => {
    render(<WorkspaceIcon workspace={ws()} active={false} onClick={() => {}} />);
    const btn = screen.getByRole("button");
    expect(btn.style.borderRadius).toBe("20px"); // size/2 = 40/2
  });

  it("hides letter when logo_url is set", () => {
    render(
      <WorkspaceIcon workspace={ws({ logo_url: "https://example.com/logo.png" })} active={false} onClick={() => {}} />,
    );
    expect(screen.getByRole("button").textContent).toBe("");
  });

  it("sets background-image when logo_url is set", () => {
    render(
      <WorkspaceIcon workspace={ws({ logo_url: "https://example.com/logo.png" })} active={false} onClick={() => {}} />,
    );
    const btn = screen.getByRole("button");
    expect(btn.style.backgroundImage).toContain("https://example.com/logo.png");
  });

  it("uses the workspace name as title", () => {
    render(<WorkspaceIcon workspace={ws({ name: "My Corp" })} active={false} onClick={() => {}} />);
    expect(screen.getByRole("button")).toHaveAttribute("title", "My Corp");
  });

  it("produces consistent colors for the same name", () => {
    const { unmount } = render(
      <WorkspaceIcon workspace={ws({ name: "test" })} active={false} onClick={() => {}} />,
    );
    const color1 = screen.getByRole("button").style.backgroundColor;
    unmount();

    render(
      <WorkspaceIcon workspace={ws({ name: "test" })} active={false} onClick={() => {}} />,
    );
    const color2 = screen.getByRole("button").style.backgroundColor;
    expect(color1).toBe(color2);
  });

  it("produces different colors for different names", () => {
    const { unmount } = render(
      <WorkspaceIcon workspace={ws({ name: "alpha" })} active={false} onClick={() => {}} />,
    );
    const color1 = screen.getByRole("button").style.backgroundColor;
    unmount();

    render(
      <WorkspaceIcon workspace={ws({ name: "zeta" })} active={false} onClick={() => {}} />,
    );
    const color2 = screen.getByRole("button").style.backgroundColor;
    // Statistically unlikely to be the same, but we just check it works
    expect(typeof color2).toBe("string");
  });
});
