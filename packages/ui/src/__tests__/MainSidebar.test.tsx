import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen, act } from "@testing-library/react";
import { ThemeProvider } from "../context/ThemeContext";
import { MainSidebar } from "../components/MainSidebar";
import type { Workspace } from "../types/api";

function mockMatchMedia(prefersDark: boolean) {
  Object.defineProperty(window, "matchMedia", {
    writable: true,
    value: vi.fn().mockImplementation((query: string) => ({
      matches: query === "(prefers-color-scheme: dark)" ? prefersDark : false,
      media: query,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    })),
  });
}

const acme: Workspace = { id: "1", name: "Acme Inc", slug: "acme", description: "", logo_url: "", role: "owner" };

function renderSidebar(props: Partial<Parameters<typeof MainSidebar>[0]> = {}) {
  const defaults = {
    workspace: acme,
    activeView: "translate" as const,
    onViewChange: vi.fn(),
    collapsed: false,
    onCollapsedChange: vi.fn(),
  };
  const merged = { ...defaults, ...props };
  return {
    ...render(
      <ThemeProvider>
        <MainSidebar {...merged} />
      </ThemeProvider>,
    ),
    props: merged,
  };
}

describe("MainSidebar", () => {
  beforeEach(() => {
    localStorage.clear();
    delete document.documentElement.dataset.theme;
    mockMatchMedia(true);
  });

  // -- Navigation --

  it("renders all four navigation items", () => {
    renderSidebar();
    expect(screen.getByTestId("nav-translate")).toBeInTheDocument();
    expect(screen.getByTestId("nav-termbase")).toBeInTheDocument();
    expect(screen.getByTestId("nav-memory")).toBeInTheDocument();
    expect(screen.getByTestId("nav-settings")).toBeInTheDocument();
  });

  it("calls onViewChange when a nav item is clicked", () => {
    const { props } = renderSidebar();
    act(() => screen.getByTestId("nav-termbase").click());
    expect(props.onViewChange).toHaveBeenCalledWith("termbase");
  });

  it("calls onViewChange with correct view for each nav item", () => {
    const { props } = renderSidebar();

    act(() => screen.getByTestId("nav-memory").click());
    expect(props.onViewChange).toHaveBeenCalledWith("memory");

    act(() => screen.getByTestId("nav-settings").click());
    expect(props.onViewChange).toHaveBeenCalledWith("settings");
  });

  // -- Workspace name --

  it("shows workspace name in the header", () => {
    renderSidebar({ workspace: acme });
    expect(screen.getByText("Acme Inc")).toBeInTheDocument();
  });

  it("shows 'No workspace' when workspace is null", () => {
    renderSidebar({ workspace: null });
    expect(screen.getByText("No workspace")).toBeInTheDocument();
  });

  // -- Collapse --

  it("has zero width when collapsed", () => {
    renderSidebar({ collapsed: true });
    const nav = screen.getByRole("navigation");
    expect(nav.style.width).toBe("0px");
  });

  it("has 220px width when not collapsed", () => {
    renderSidebar({ collapsed: false });
    const nav = screen.getByRole("navigation");
    expect(nav.style.width).toBe("220px");
  });

  it("calls onCollapsedChange when collapse button is clicked", () => {
    const { props } = renderSidebar({ collapsed: false });
    // The collapse button is the last button in the footer (after theme toggle)
    const buttons = screen.getAllByRole("button");
    const collapseBtn = buttons[buttons.length - 1];
    act(() => collapseBtn.click());
    expect(props.onCollapsedChange).toHaveBeenCalledWith(true);
  });

  // -- Theme toggle --

  it("renders a theme toggle button", () => {
    renderSidebar();
    expect(screen.getByTestId("theme-toggle")).toBeInTheDocument();
  });

  it("clicking toggle switches from dark to light", () => {
    localStorage.setItem("gokapi-theme", "dark");
    renderSidebar();

    const toggle = screen.getByTestId("theme-toggle");
    expect(document.documentElement.dataset.theme).toBe("dark");

    act(() => toggle.click());
    expect(document.documentElement.dataset.theme).toBe("light");
    expect(localStorage.getItem("gokapi-theme")).toBe("light");
  });

  it("clicking toggle switches from light to dark", () => {
    localStorage.setItem("gokapi-theme", "light");
    renderSidebar();

    const toggle = screen.getByTestId("theme-toggle");
    expect(document.documentElement.dataset.theme).toBe("light");

    act(() => toggle.click());
    expect(document.documentElement.dataset.theme).toBe("dark");
    expect(localStorage.getItem("gokapi-theme")).toBe("dark");
  });
});
