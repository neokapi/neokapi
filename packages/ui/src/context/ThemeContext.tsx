import { createContext, useContext, useState, useEffect, useCallback, type ReactNode } from "react";

export type Theme = "dark" | "light" | "system";

interface ThemeContextValue {
  theme: Theme;
  setTheme: (theme: Theme) => void;
}

const ThemeContext = createContext<ThemeContextValue | null>(null);

const VALID_THEMES: Theme[] = ["dark", "light", "system"];

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

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setThemeState] = useState<Theme>(() => {
    const stored = localStorage.getItem("gokapi-theme");
    return stored ? migrateTheme(stored) : "system";
  });

  const setTheme = useCallback((t: Theme) => {
    setThemeState(t);
    localStorage.setItem("gokapi-theme", t);
  }, []);

  useEffect(() => {
    const applyTheme = () => {
      let isDark: boolean;
      if (theme === "system") {
        isDark = window.matchMedia("(prefers-color-scheme: dark)").matches;
      } else {
        isDark = theme === "dark";
      }
      document.documentElement.classList.toggle("dark", isDark);
    };

    applyTheme();

    if (theme === "system") {
      const mq = window.matchMedia("(prefers-color-scheme: dark)");
      mq.addEventListener("change", applyTheme);
      return () => mq.removeEventListener("change", applyTheme);
    }
  }, [theme]);

  return (
    <ThemeContext value={{ theme, setTheme }}>
      {children}
    </ThemeContext>
  );
}

export function useTheme(): ThemeContextValue {
  const ctx = useContext(ThemeContext);
  if (!ctx) throw new Error("useTheme must be used within ThemeProvider");
  return ctx;
}
