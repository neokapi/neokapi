import { describe, it, expect, beforeEach, afterEach } from "vite-plus/test";
import { render, screen, act } from "@testing-library/react";
import { ThemeProvider, useTheme, getCookieDomain, getThemeCookie } from "../context/ThemeContext";

// Helper component that exposes theme state for assertions
function ThemeDisplay() {
  const { theme, setTheme } = useTheme();
  return (
    <div>
      <span data-testid="theme">{theme}</span>
      <button data-testid="set-dark" onClick={() => setTheme("dark")}>
        Dark
      </button>
      <button data-testid="set-light" onClick={() => setTheme("light")}>
        Light
      </button>
      <button data-testid="set-system" onClick={() => setTheme("system")}>
        System
      </button>
    </div>
  );
}

/** Clear the neokapi-theme cookie (set max-age=0). */
function clearThemeCookie() {
  document.cookie = "neokapi-theme=;path=/;max-age=0";
}

describe("ThemeContext", () => {
  beforeEach(() => {
    localStorage.clear();
    clearThemeCookie();
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
    localStorage.setItem("neokapi-theme", "dark");
    render(
      <ThemeProvider>
        <ThemeDisplay />
      </ThemeProvider>,
    );
    expect(screen.getByTestId("theme").textContent).toBe("dark");
    expect(document.documentElement.classList.contains("dark")).toBe(true);
  });

  it("toggles from dark to light", () => {
    localStorage.setItem("neokapi-theme", "dark");
    render(
      <ThemeProvider>
        <ThemeDisplay />
      </ThemeProvider>,
    );

    act(() => screen.getByTestId("set-light").click());

    expect(screen.getByTestId("theme").textContent).toBe("light");
    expect(document.documentElement.classList.contains("dark")).toBe(false);
    expect(localStorage.getItem("neokapi-theme")).toBe("light");
  });

  it("toggles from light to dark", () => {
    localStorage.setItem("neokapi-theme", "light");
    render(
      <ThemeProvider>
        <ThemeDisplay />
      </ThemeProvider>,
    );

    act(() => screen.getByTestId("set-dark").click());

    expect(screen.getByTestId("theme").textContent).toBe("dark");
    expect(document.documentElement.classList.contains("dark")).toBe(true);
    expect(localStorage.getItem("neokapi-theme")).toBe("dark");
  });

  it("persists theme preference to localStorage", () => {
    render(
      <ThemeProvider>
        <ThemeDisplay />
      </ThemeProvider>,
    );

    act(() => screen.getByTestId("set-light").click());
    expect(localStorage.getItem("neokapi-theme")).toBe("light");

    act(() => screen.getByTestId("set-dark").click());
    expect(localStorage.getItem("neokapi-theme")).toBe("dark");

    act(() => screen.getByTestId("set-system").click());
    expect(localStorage.getItem("neokapi-theme")).toBe("system");
  });

  it("dark sets dark class, light does not", () => {
    localStorage.setItem("neokapi-theme", "dark");
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

  it("ignores invalid localStorage values and defaults to system", () => {
    localStorage.setItem("neokapi-theme", "purple");
    render(
      <ThemeProvider>
        <ThemeDisplay />
      </ThemeProvider>,
    );
    expect(screen.getByTestId("theme").textContent).toBe("system");
  });

  it("throws when useTheme is called outside ThemeProvider", () => {
    expect(() => render(<ThemeDisplay />)).toThrow("useTheme must be used within ThemeProvider");
  });

  // -- Cookie sync --

  it("reads initial theme from cookie when localStorage is empty", () => {
    document.cookie = "neokapi-theme=dark;path=/";
    render(
      <ThemeProvider>
        <ThemeDisplay />
      </ThemeProvider>,
    );
    expect(screen.getByTestId("theme").textContent).toBe("dark");
    expect(document.documentElement.classList.contains("dark")).toBe(true);
  });

  it("prefers cookie over localStorage", () => {
    document.cookie = "neokapi-theme=light;path=/";
    localStorage.setItem("neokapi-theme", "dark");
    render(
      <ThemeProvider>
        <ThemeDisplay />
      </ThemeProvider>,
    );
    expect(screen.getByTestId("theme").textContent).toBe("light");
  });

  it("writes theme to cookie when setTheme is called", () => {
    render(
      <ThemeProvider>
        <ThemeDisplay />
      </ThemeProvider>,
    );

    act(() => screen.getByTestId("set-dark").click());
    expect(getThemeCookie()).toBe("dark");

    act(() => screen.getByTestId("set-light").click());
    expect(getThemeCookie()).toBe("light");
  });

  it("syncs cookie value to localStorage on mount", () => {
    document.cookie = "neokapi-theme=dark;path=/";
    render(
      <ThemeProvider>
        <ThemeDisplay />
      </ThemeProvider>,
    );
    expect(localStorage.getItem("neokapi-theme")).toBe("dark");
  });

  it("migrates legacy cookie value", () => {
    document.cookie = "neokapi-theme=glass;path=/";
    render(
      <ThemeProvider>
        <ThemeDisplay />
      </ThemeProvider>,
    );
    expect(screen.getByTestId("theme").textContent).toBe("dark");
  });
});

describe("getCookieDomain", () => {
  const originalHostname = window.location.hostname;
  afterEach(() => {
    Object.defineProperty(window, "location", {
      value: Object.assign({}, window.location, { hostname: originalHostname }),
      writable: true,
    });
  });

  it("returns undefined for localhost", () => {
    Object.defineProperty(window, "location", {
      value: Object.assign({}, window.location, { hostname: "localhost" }),
      writable: true,
    });
    expect(getCookieDomain()).toBeUndefined();
  });

  it("returns undefined for IP addresses", () => {
    Object.defineProperty(window, "location", {
      value: Object.assign({}, window.location, { hostname: "127.0.0.1" }),
      writable: true,
    });
    expect(getCookieDomain()).toBeUndefined();
  });

  it("returns parent domain for subdomain hostnames", () => {
    Object.defineProperty(window, "location", {
      value: Object.assign({}, window.location, { hostname: "auth.bowrain.mymac" }),
      writable: true,
    });
    expect(getCookieDomain()).toBe(".bowrain.mymac");
  });

  it("returns dotted hostname for two-part hostnames", () => {
    Object.defineProperty(window, "location", {
      value: Object.assign({}, window.location, { hostname: "bowrain.mymac" }),
      writable: true,
    });
    expect(getCookieDomain()).toBe(".bowrain.mymac");
  });
});
