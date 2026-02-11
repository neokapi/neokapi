import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen, act } from "@testing-library/react";
import { ThemeProvider, useTheme, type Theme } from "../context/ThemeContext";

// Helper component that exposes theme state for assertions
function ThemeDisplay() {
  const { theme, resolvedTheme, setTheme } = useTheme();
  return (
    <div>
      <span data-testid="theme">{theme}</span>
      <span data-testid="resolved">{resolvedTheme}</span>
      <button data-testid="set-light" onClick={() => setTheme("light")}>Light</button>
      <button data-testid="set-dark" onClick={() => setTheme("dark")}>Dark</button>
      <button data-testid="set-system" onClick={() => setTheme("system")}>System</button>
    </div>
  );
}

// Mock matchMedia for system theme detection
function mockMatchMedia(prefersDark: boolean) {
  const listeners: Array<(e: { matches: boolean }) => void> = [];
  Object.defineProperty(window, "matchMedia", {
    writable: true,
    value: vi.fn().mockImplementation((query: string) => ({
      matches: query === "(prefers-color-scheme: dark)" ? prefersDark : false,
      media: query,
      addEventListener: (_event: string, cb: (e: { matches: boolean }) => void) => {
        listeners.push(cb);
      },
      removeEventListener: (_event: string, cb: (e: { matches: boolean }) => void) => {
        const idx = listeners.indexOf(cb);
        if (idx >= 0) listeners.splice(idx, 1);
      },
    })),
  });
  return {
    /** Simulate OS theme change */
    setPrefersDark(dark: boolean) {
      // Update the mock before firing listeners
      Object.defineProperty(window, "matchMedia", {
        writable: true,
        value: vi.fn().mockImplementation((query: string) => ({
          matches: query === "(prefers-color-scheme: dark)" ? dark : false,
          media: query,
          addEventListener: (_event: string, cb: (e: { matches: boolean }) => void) => {
            listeners.push(cb);
          },
          removeEventListener: (_event: string, cb: (e: { matches: boolean }) => void) => {
            const idx = listeners.indexOf(cb);
            if (idx >= 0) listeners.splice(idx, 1);
          },
        })),
      });
      for (const fn of [...listeners]) fn({ matches: dark });
    },
  };
}

describe("ThemeContext", () => {
  beforeEach(() => {
    localStorage.clear();
    delete document.documentElement.dataset.theme;
    mockMatchMedia(false); // default: system prefers light
  });

  it("defaults to system theme when no localStorage value", () => {
    render(
      <ThemeProvider>
        <ThemeDisplay />
      </ThemeProvider>,
    );
    expect(screen.getByTestId("theme").textContent).toBe("system");
    expect(screen.getByTestId("resolved").textContent).toBe("light");
    expect(document.documentElement.dataset.theme).toBe("light");
  });

  it("resolves system theme to dark when OS prefers dark", () => {
    mockMatchMedia(true);
    render(
      <ThemeProvider>
        <ThemeDisplay />
      </ThemeProvider>,
    );
    expect(screen.getByTestId("theme").textContent).toBe("system");
    expect(screen.getByTestId("resolved").textContent).toBe("dark");
    expect(document.documentElement.dataset.theme).toBe("dark");
  });

  it("reads initial theme from localStorage", () => {
    localStorage.setItem("gokapi-theme", "dark");
    render(
      <ThemeProvider>
        <ThemeDisplay />
      </ThemeProvider>,
    );
    expect(screen.getByTestId("theme").textContent).toBe("dark");
    expect(screen.getByTestId("resolved").textContent).toBe("dark");
    expect(document.documentElement.dataset.theme).toBe("dark");
  });

  it("toggles from dark to light", () => {
    localStorage.setItem("gokapi-theme", "dark");
    render(
      <ThemeProvider>
        <ThemeDisplay />
      </ThemeProvider>,
    );

    act(() => screen.getByTestId("set-light").click());

    expect(screen.getByTestId("theme").textContent).toBe("light");
    expect(screen.getByTestId("resolved").textContent).toBe("light");
    expect(document.documentElement.dataset.theme).toBe("light");
    expect(localStorage.getItem("gokapi-theme")).toBe("light");
  });

  it("toggles from light to dark", () => {
    localStorage.setItem("gokapi-theme", "light");
    render(
      <ThemeProvider>
        <ThemeDisplay />
      </ThemeProvider>,
    );

    act(() => screen.getByTestId("set-dark").click());

    expect(screen.getByTestId("theme").textContent).toBe("dark");
    expect(screen.getByTestId("resolved").textContent).toBe("dark");
    expect(document.documentElement.dataset.theme).toBe("dark");
    expect(localStorage.getItem("gokapi-theme")).toBe("dark");
  });

  it("persists theme preference to localStorage", () => {
    render(
      <ThemeProvider>
        <ThemeDisplay />
      </ThemeProvider>,
    );

    act(() => screen.getByTestId("set-dark").click());
    expect(localStorage.getItem("gokapi-theme")).toBe("dark");

    act(() => screen.getByTestId("set-light").click());
    expect(localStorage.getItem("gokapi-theme")).toBe("light");

    act(() => screen.getByTestId("set-system").click());
    expect(localStorage.getItem("gokapi-theme")).toBe("system");
  });

  it("sets data-theme attribute on document element", () => {
    render(
      <ThemeProvider>
        <ThemeDisplay />
      </ThemeProvider>,
    );

    act(() => screen.getByTestId("set-dark").click());
    expect(document.documentElement.dataset.theme).toBe("dark");

    act(() => screen.getByTestId("set-light").click());
    expect(document.documentElement.dataset.theme).toBe("light");
  });

  it("switching to system mode resolves to current OS preference", () => {
    mockMatchMedia(true); // OS prefers dark
    localStorage.setItem("gokapi-theme", "light");

    render(
      <ThemeProvider>
        <ThemeDisplay />
      </ThemeProvider>,
    );

    expect(screen.getByTestId("resolved").textContent).toBe("light");

    act(() => screen.getByTestId("set-system").click());
    expect(screen.getByTestId("resolved").textContent).toBe("dark");
    expect(document.documentElement.dataset.theme).toBe("dark");
  });

  it("ignores invalid localStorage values", () => {
    localStorage.setItem("gokapi-theme", "purple");
    render(
      <ThemeProvider>
        <ThemeDisplay />
      </ThemeProvider>,
    );
    expect(screen.getByTestId("theme").textContent).toBe("system");
  });

  it("throws when useTheme is called outside ThemeProvider", () => {
    expect(() => render(<ThemeDisplay />)).toThrow(
      "useTheme must be used within ThemeProvider",
    );
  });
});
