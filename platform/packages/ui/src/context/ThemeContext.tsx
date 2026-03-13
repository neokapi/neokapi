import { createContext, useContext, useState, useEffect, useCallback, type ReactNode } from "react";

export type Theme = "dark" | "light" | "system";

interface ThemeContextValue {
  theme: Theme;
  setTheme: (theme: Theme) => void;
}

const ThemeContext = createContext<ThemeContextValue | null>(null);

const VALID_THEMES: Theme[] = ["dark", "light", "system"];
const STORAGE_KEY = "neokapi-theme";
const COOKIE_NAME = "neokapi-theme";
const COOKIE_MAX_AGE = 365 * 24 * 60 * 60; // 1 year in seconds

function isValidTheme(value: string): value is Theme {
  return VALID_THEMES.includes(value as Theme);
}

/** Migrate legacy theme values from the old glass/aurora system. */
function migrateTheme(stored: string): Theme {
  if (isValidTheme(stored)) return stored;
  // "glass" and "aurora" were both dark themes — map to "dark"
  if (stored === "glass" || stored === "aurora") return "dark";
  return "system";
}

/**
 * Compute the cookie Domain attribute so the theme cookie is shared across
 * subdomains (e.g. bowrain.mymac ↔ auth.bowrain.mymac). Returns undefined
 * for localhost / IP addresses where a domain attribute is not useful.
 */
export function getCookieDomain(): string | undefined {
  const host = window.location.hostname;
  if (host === "localhost" || /^\d+\.\d+\.\d+\.\d+$/.test(host) || host === "[::1]") {
    return undefined;
  }
  const parts = host.split(".");
  if (parts.length < 2) return undefined;
  return "." + parts.slice(-2).join(".");
}

export function getThemeCookie(): string | null {
  const match = document.cookie.match(new RegExp(`(?:^|;\\s*)${COOKIE_NAME}=([^;]*)`));
  return match ? decodeURIComponent(match[1]) : null;
}

export function setThemeCookie(value: string): void {
  const domain = getCookieDomain();
  const secure = window.location.protocol === "https:";
  let cookie = `${COOKIE_NAME}=${encodeURIComponent(value)};path=/;max-age=${COOKIE_MAX_AGE};samesite=lax`;
  if (domain) cookie += `;domain=${domain}`;
  if (secure) cookie += ";secure";
  document.cookie = cookie;
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setThemeState] = useState<Theme>(() => {
    const raw = getThemeCookie() ?? localStorage.getItem(STORAGE_KEY);
    return raw ? migrateTheme(raw) : "system";
  });

  const setTheme = useCallback((t: Theme) => {
    setThemeState(t);
    localStorage.setItem(STORAGE_KEY, t);
    setThemeCookie(t);
  }, []);

  useEffect(() => {
    // Sync to both stores so cookie ↔ localStorage stay consistent even when
    // the initial value came from only one source (e.g. first visit on a new
    // subdomain where only the cookie exists).
    localStorage.setItem(STORAGE_KEY, theme);
    setThemeCookie(theme);

    const applyTheme = () => {
      let isDark: boolean;
      if (theme === "system") {
        isDark = window.matchMedia("(prefers-color-scheme: dark)").matches;
      } else {
        isDark = theme === "dark";
      }
      document.documentElement.classList.toggle("dark", isDark);
      // Tell native browser controls (date pickers, scrollbars, etc.) to use the right scheme.
      document.documentElement.style.colorScheme = isDark ? "dark" : "light";
      // Activate shadcn-glass-ui semantic tokens (--semantic-*, --orb-*, --bg-*, --sidebar-*).
      document.documentElement.setAttribute("data-theme", isDark ? "aurora" : "light");
    };

    applyTheme();

    if (theme === "system") {
      const mq = window.matchMedia("(prefers-color-scheme: dark)");
      mq.addEventListener("change", applyTheme);
      return () => mq.removeEventListener("change", applyTheme);
    }
  }, [theme]);

  return <ThemeContext value={{ theme, setTheme }}>{children}</ThemeContext>;
}

export function useTheme(): ThemeContextValue {
  const ctx = useContext(ThemeContext);
  if (!ctx) throw new Error("useTheme must be used within ThemeProvider");
  return ctx;
}
