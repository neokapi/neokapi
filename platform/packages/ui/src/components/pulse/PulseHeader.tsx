import { Moon, Sun } from "lucide-react";
import { useState, useEffect } from "react";

interface PulseHeaderProps {
  workspaceName: string;
  logoUrl?: string;
}

export function PulseHeader({ workspaceName, logoUrl }: PulseHeaderProps) {
  const [dark, setDark] = useState(
    () => typeof window !== "undefined" && document.documentElement.classList.contains("dark"),
  );

  useEffect(() => {
    document.documentElement.classList.toggle("dark", dark);
  }, [dark]);

  return (
    <header className="sticky top-0 z-50 border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
      <div className="mx-auto flex h-14 max-w-7xl items-center justify-between px-4">
        <div className="flex items-center gap-3">
          {logoUrl ? (
            <img src={logoUrl} alt={workspaceName} className="h-8 w-8 rounded" />
          ) : (
            <div className="flex h-8 w-8 items-center justify-center rounded bg-primary text-primary-foreground text-sm font-bold">
              {workspaceName.charAt(0).toUpperCase()}
            </div>
          )}
          <div className="flex items-center gap-2">
            <span className="font-semibold">{workspaceName}</span>
            <span className="text-xs text-muted-foreground">Pulse</span>
          </div>
        </div>
        <button
          onClick={() => setDark(!dark)}
          className="rounded-md p-2 text-muted-foreground hover:bg-muted"
          aria-label="Toggle theme"
        >
          {dark ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
        </button>
      </div>
    </header>
  );
}
