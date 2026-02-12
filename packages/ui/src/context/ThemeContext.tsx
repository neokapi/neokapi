import { createContext, useContext, useState, useEffect, useCallback, type ReactNode } from "react";

export type Theme = "glass" | "light" | "aurora";

interface ThemeContextValue {
  theme: Theme;
  setTheme: (theme: Theme) => void;
}

const ThemeContext = createContext<ThemeContextValue | null>(null);

const VALID_THEMES: Theme[] = ["glass", "light", "aurora"];

function isValidTheme(value: string): value is Theme {
  return VALID_THEMES.includes(value as Theme);
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setThemeState] = useState<Theme>(() => {
    const stored = localStorage.getItem("gokapi-theme");
    return stored && isValidTheme(stored) ? stored : "glass";
  });

  const setTheme = useCallback((t: Theme) => {
    setThemeState(t);
    localStorage.setItem("gokapi-theme", t);
  }, []);

  useEffect(() => {
    // Glass and Aurora are dark-based, Light is light-based
    const isDark = theme !== "light";
    document.documentElement.classList.toggle("dark", isDark);
    document.documentElement.dataset.theme = theme;
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
