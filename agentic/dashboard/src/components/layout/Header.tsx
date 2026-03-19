import { Github, Moon, Sun } from 'lucide-react';
import { useState } from 'react';

export default function Header() {
  const [darkMode, setDarkMode] = useState(true);

  return (
    <header className="sticky top-0 z-50 border-b border-[var(--color-border)] bg-[var(--color-bg-primary)]/95 backdrop-blur-sm">
      <div className="mx-auto flex max-w-7xl items-center justify-between px-6 py-3">
        <div className="flex items-center gap-4">
          <span className="font-[family-name:var(--font-mono)] text-lg font-bold tracking-tight text-[var(--color-accent-amber)]">
            bowrain
          </span>
          <span className="hidden rounded-full border border-[var(--color-border)] px-3 py-0.5 font-[family-name:var(--font-mono)] text-xs text-[var(--color-text-muted)] sm:inline-block">
            agents.dev.bowrain.cloud
          </span>
        </div>
        <div className="flex items-center gap-3">
          <button
            onClick={() => setDarkMode(!darkMode)}
            className="rounded-lg p-2 text-[var(--color-text-secondary)] transition-colors hover:bg-[var(--color-bg-elevated)] hover:text-[var(--color-text-primary)]"
            aria-label="Toggle theme"
          >
            {darkMode ? <Sun size={18} /> : <Moon size={18} />}
          </button>
          <a
            href="https://github.com/neokapi/neokapi"
            target="_blank"
            rel="noopener noreferrer"
            className="rounded-lg p-2 text-[var(--color-text-secondary)] transition-colors hover:bg-[var(--color-bg-elevated)] hover:text-[var(--color-text-primary)]"
            aria-label="GitHub"
          >
            <Github size={18} />
          </a>
        </div>
      </div>
    </header>
  );
}
