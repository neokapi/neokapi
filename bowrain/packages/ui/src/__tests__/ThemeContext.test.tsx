import { describe, it, expect, beforeEach } from "vitest";
import { render, screen, act } from "@testing-library/react";
import { ThemeProvider, useTheme, type Theme } from "../context/ThemeContext";

// Helper component that exposes theme state for assertions
function ThemeDisplay() {
  const { theme, setTheme } = useTheme();
  return (
    <div>
      <span data-testid="theme">{theme}</span>
      <button data-testid="set-glass" onClick={() => setTheme("glass")}>Glass</button>
      <button data-testid="set-light" onClick={() => setTheme("light")}>Light</button>
      <button data-testid="set-aurora" onClick={() => setTheme("aurora")}>Aurora</button>
    </div>
  );
}

describe("ThemeContext", () => {
  beforeEach(() => {
    localStorage.clear();
    delete document.documentElement.dataset.theme;
    document.documentElement.classList.remove("dark");
  });

  it("defaults to glass theme when no localStorage value", () => {
    render(
      <ThemeProvider>
        <ThemeDisplay />
      </ThemeProvider>,
    );
    expect(screen.getByTestId("theme").textContent).toBe("glass");
    expect(document.documentElement.dataset.theme).toBe("glass");
    expect(document.documentElement.classList.contains("dark")).toBe(true);
  });

  it("reads initial theme from localStorage", () => {
    localStorage.setItem("gokapi-theme", "aurora");
    render(
      <ThemeProvider>
        <ThemeDisplay />
      </ThemeProvider>,
    );
    expect(screen.getByTestId("theme").textContent).toBe("aurora");
    expect(document.documentElement.dataset.theme).toBe("aurora");
    expect(document.documentElement.classList.contains("dark")).toBe(true);
  });

  it("toggles from glass to light", () => {
    render(
      <ThemeProvider>
        <ThemeDisplay />
      </ThemeProvider>,
    );

    act(() => screen.getByTestId("set-light").click());

    expect(screen.getByTestId("theme").textContent).toBe("light");
    expect(document.documentElement.dataset.theme).toBe("light");
    expect(document.documentElement.classList.contains("dark")).toBe(false);
    expect(localStorage.getItem("gokapi-theme")).toBe("light");
  });

  it("toggles from light to aurora", () => {
    localStorage.setItem("gokapi-theme", "light");
    render(
      <ThemeProvider>
        <ThemeDisplay />
      </ThemeProvider>,
    );

    act(() => screen.getByTestId("set-aurora").click());

    expect(screen.getByTestId("theme").textContent).toBe("aurora");
    expect(document.documentElement.dataset.theme).toBe("aurora");
    expect(document.documentElement.classList.contains("dark")).toBe(true);
    expect(localStorage.getItem("gokapi-theme")).toBe("aurora");
  });

  it("toggles from aurora to glass", () => {
    localStorage.setItem("gokapi-theme", "aurora");
    render(
      <ThemeProvider>
        <ThemeDisplay />
      </ThemeProvider>,
    );

    act(() => screen.getByTestId("set-glass").click());

    expect(screen.getByTestId("theme").textContent).toBe("glass");
    expect(document.documentElement.dataset.theme).toBe("glass");
    expect(document.documentElement.classList.contains("dark")).toBe(true);
    expect(localStorage.getItem("gokapi-theme")).toBe("glass");
  });

  it("persists theme preference to localStorage", () => {
    render(
      <ThemeProvider>
        <ThemeDisplay />
      </ThemeProvider>,
    );

    act(() => screen.getByTestId("set-light").click());
    expect(localStorage.getItem("gokapi-theme")).toBe("light");

    act(() => screen.getByTestId("set-aurora").click());
    expect(localStorage.getItem("gokapi-theme")).toBe("aurora");

    act(() => screen.getByTestId("set-glass").click());
    expect(localStorage.getItem("gokapi-theme")).toBe("glass");
  });

  it("sets data-theme attribute on document element", () => {
    render(
      <ThemeProvider>
        <ThemeDisplay />
      </ThemeProvider>,
    );

    act(() => screen.getByTestId("set-light").click());
    expect(document.documentElement.dataset.theme).toBe("light");

    act(() => screen.getByTestId("set-aurora").click());
    expect(document.documentElement.dataset.theme).toBe("aurora");

    act(() => screen.getByTestId("set-glass").click());
    expect(document.documentElement.dataset.theme).toBe("glass");
  });

  it("glass and aurora set dark class, light does not", () => {
    render(
      <ThemeProvider>
        <ThemeDisplay />
      </ThemeProvider>,
    );

    // Default is glass (dark)
    expect(document.documentElement.classList.contains("dark")).toBe(true);

    act(() => screen.getByTestId("set-light").click());
    expect(document.documentElement.classList.contains("dark")).toBe(false);

    act(() => screen.getByTestId("set-aurora").click());
    expect(document.documentElement.classList.contains("dark")).toBe(true);
  });

  it("ignores invalid localStorage values", () => {
    localStorage.setItem("gokapi-theme", "purple");
    render(
      <ThemeProvider>
        <ThemeDisplay />
      </ThemeProvider>,
    );
    expect(screen.getByTestId("theme").textContent).toBe("glass");
  });

  it("throws when useTheme is called outside ThemeProvider", () => {
    expect(() => render(<ThemeDisplay />)).toThrow(
      "useTheme must be used within ThemeProvider",
    );
  });
});
