import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen, act } from "@testing-library/react";
import { ThemeProvider, useTheme, type Theme } from "../context/ThemeContext";

// Helper component that exposes theme state for assertions
function ThemeDisplay() {
  const { theme, setTheme } = useTheme();
  return (
    <div>
      <span data-testid="theme">{theme}</span>
      <button data-testid="set-dark" onClick={() => setTheme("dark")}>Dark</button>
      <button data-testid="set-light" onClick={() => setTheme("light")}>Light</button>
      <button data-testid="set-system" onClick={() => setTheme("system")}>System</button>
    </div>
  );
}

describe("ThemeContext", () => {
  beforeEach(() => {
    localStorage.clear();
    document.documentElement.classList.remove("dark");
  });

  it("defaults to system theme when no localStorage value", () => {
    render(
      <ThemeProvider>
        <ThemeDisplay />
      </ThemeProvider>,
    );
    expect(screen.getByTestId("theme").textContent).toBe("system");
  });

  it("reads initial theme from localStorage", () => {
    localStorage.setItem("gokapi-theme", "dark");
    render(
      <ThemeProvider>
        <ThemeDisplay />
      </ThemeProvider>,
    );
    expect(screen.getByTestId("theme").textContent).toBe("dark");
    expect(document.documentElement.classList.contains("dark")).toBe(true);
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
    expect(document.documentElement.classList.contains("dark")).toBe(false);
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
    expect(document.documentElement.classList.contains("dark")).toBe(true);
    expect(localStorage.getItem("gokapi-theme")).toBe("dark");
  });

  it("persists theme preference to localStorage", () => {
    render(
      <ThemeProvider>
        <ThemeDisplay />
      </ThemeProvider>,
    );

    act(() => screen.getByTestId("set-light").click());
    expect(localStorage.getItem("gokapi-theme")).toBe("light");

    act(() => screen.getByTestId("set-dark").click());
    expect(localStorage.getItem("gokapi-theme")).toBe("dark");

    act(() => screen.getByTestId("set-system").click());
    expect(localStorage.getItem("gokapi-theme")).toBe("system");
  });

  it("dark sets dark class, light does not", () => {
    localStorage.setItem("gokapi-theme", "dark");
    render(
      <ThemeProvider>
        <ThemeDisplay />
      </ThemeProvider>,
    );

    expect(document.documentElement.classList.contains("dark")).toBe(true);

    act(() => screen.getByTestId("set-light").click());
    expect(document.documentElement.classList.contains("dark")).toBe(false);

    act(() => screen.getByTestId("set-dark").click());
    expect(document.documentElement.classList.contains("dark")).toBe(true);
  });

  it("migrates legacy glass value to dark", () => {
    localStorage.setItem("gokapi-theme", "glass");
    render(
      <ThemeProvider>
        <ThemeDisplay />
      </ThemeProvider>,
    );
    expect(screen.getByTestId("theme").textContent).toBe("dark");
  });

  it("migrates legacy aurora value to dark", () => {
    localStorage.setItem("gokapi-theme", "aurora");
    render(
      <ThemeProvider>
        <ThemeDisplay />
      </ThemeProvider>,
    );
    expect(screen.getByTestId("theme").textContent).toBe("dark");
  });

  it("ignores invalid localStorage values and defaults to system", () => {
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
