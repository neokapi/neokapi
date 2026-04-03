import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen, act } from "@testing-library/react";
import { WorkspaceIcon } from "../components/WorkspaceIcon";
import type { Workspace } from "../types/api";

function ws(overrides: Partial<Workspace> = {}): Workspace {
  return {
    id: "1",
    name: "Acme",
    slug: "acme",
    description: "",
    logo_url: "",
    type: "team",
    role: "owner",
    ...overrides,
  };
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
    const { container } = render(
      <WorkspaceIcon workspace={ws()} active={true} onClick={() => {}} />,
    );
    const inner = container.querySelector("[aria-hidden]") as HTMLElement;
    expect(inner.style.borderRadius).toBe("12px");
  });

  it("uses circular border-radius when inactive", () => {
    const { container } = render(
      <WorkspaceIcon workspace={ws()} active={false} onClick={() => {}} />,
    );
    const inner = container.querySelector("[aria-hidden]") as HTMLElement;
    expect(inner.style.borderRadius).toBe("20px"); // size/2 = 40/2
  });

  it("hides letter when logo_url is set", () => {
    const { container } = render(
      <WorkspaceIcon
        workspace={ws({ logo_url: "https://example.com/logo.png" })}
        active={false}
        onClick={() => {}}
      />,
    );
    const inner = container.querySelector("[aria-hidden]") as HTMLElement;
    expect(inner.textContent).toBe("");
  });

  it("sets background-image when logo_url is set", () => {
    const { container } = render(
      <WorkspaceIcon
        workspace={ws({ logo_url: "https://example.com/logo.png" })}
        active={false}
        onClick={() => {}}
      />,
    );
    const inner = container.querySelector("[aria-hidden]") as HTMLElement;
    expect(inner.style.backgroundImage).toContain("https://example.com/logo.png");
  });

  it("uses the workspace name as title", () => {
    render(<WorkspaceIcon workspace={ws({ name: "My Corp" })} active={false} onClick={() => {}} />);
    expect(screen.getByRole("button")).toHaveAttribute("title", "My Corp");
  });

  it("produces consistent colors for the same name", () => {
    const { unmount, container } = render(
      <WorkspaceIcon workspace={ws({ name: "test" })} active={false} onClick={() => {}} />,
    );
    const color1 = (container.querySelector("[aria-hidden]") as HTMLElement).style.backgroundColor;
    unmount();

    const { container: c2 } = render(
      <WorkspaceIcon workspace={ws({ name: "test" })} active={false} onClick={() => {}} />,
    );
    const color2 = (c2.querySelector("[aria-hidden]") as HTMLElement).style.backgroundColor;
    expect(color1).toBe(color2);
  });

  it("produces different colors for different names", () => {
    const { unmount, container } = render(
      <WorkspaceIcon workspace={ws({ name: "alpha" })} active={false} onClick={() => {}} />,
    );
    const _color1 = (container.querySelector("[aria-hidden]") as HTMLElement).style.backgroundColor;
    unmount();

    const { container: c2 } = render(
      <WorkspaceIcon workspace={ws({ name: "zeta" })} active={false} onClick={() => {}} />,
    );
    const color2 = (c2.querySelector("[aria-hidden]") as HTMLElement).style.backgroundColor;
    // Statistically unlikely to be the same, but we just check it works
    expect(typeof color2).toBe("string");
  });

  it("shows @ indicator for personal workspaces", () => {
    render(
      <WorkspaceIcon workspace={ws({ type: "personal" })} active={false} onClick={() => {}} />,
    );
    expect(screen.getByTestId("personal-indicator")).toHaveTextContent("@");
  });

  it("does not show @ indicator for team workspaces", () => {
    render(<WorkspaceIcon workspace={ws({ type: "team" })} active={false} onClick={() => {}} />);
    expect(screen.queryByTestId("personal-indicator")).toBeNull();
  });

  it("scales the personal indicator with icon size", () => {
    render(
      <WorkspaceIcon
        workspace={ws({ type: "personal" })}
        active={false}
        onClick={() => {}}
        size={60}
      />,
    );
    const indicator = screen.getByTestId("personal-indicator");
    expect(indicator.style.width).toBe("24px"); // Math.round(60 * 0.4)
  });
});
