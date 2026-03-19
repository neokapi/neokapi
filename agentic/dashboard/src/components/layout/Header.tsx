import { Github, Moon, Sun, Menu, X } from 'lucide-react';
import { useState } from 'react';
import { useTheme } from '../../context/ThemeContext';

export default function Header() {
  const { theme, toggleTheme } = useTheme();
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false);

  return (
    <header
      className="sticky top-0 z-50"
      style={{
        backgroundColor: `rgb(var(--bg-base) / 0.95)`,
        backdropFilter: 'blur(12px)',
        borderBottom: '1px solid rgb(var(--border))',
        boxShadow: `0 1px 0 rgb(var(--accent) / 0.1)`,
      }}
    >
      <div className="mx-auto flex max-w-7xl items-center justify-between px-4 py-3 sm:px-6">
        <div className="flex items-center gap-3 sm:gap-4">
          <span
            className="font-display text-xl font-normal italic tracking-tight sm:text-2xl"
            style={{ color: 'rgb(var(--accent))' }}
          >
            bowrain
          </span>
          <span
            className="hidden rounded-full px-3 py-0.5 font-mono text-xs sm:inline-block"
            style={{
              color: 'rgb(var(--text-muted))',
              border: '1px solid rgb(var(--border))',
            }}
          >
            agents.dev.bowrain.cloud
          </span>
        </div>

        {/* Desktop controls */}
        <div className="hidden items-center gap-3 sm:flex">
          <button
            onClick={toggleTheme}
            className="rounded-lg p-2 transition-colors"
            style={{ color: 'rgb(var(--text-secondary))' }}
            onMouseEnter={(e) => {
              (e.currentTarget as HTMLElement).style.backgroundColor = 'rgb(var(--bg-elevated))';
              (e.currentTarget as HTMLElement).style.color = 'rgb(var(--text-primary))';
            }}
            onMouseLeave={(e) => {
              (e.currentTarget as HTMLElement).style.backgroundColor = 'transparent';
              (e.currentTarget as HTMLElement).style.color = 'rgb(var(--text-secondary))';
            }}
            aria-label="Toggle theme"
          >
            {theme === 'dark' ? <Sun size={18} /> : <Moon size={18} />}
          </button>
          <a
            href="https://github.com/neokapi/neokapi"
            target="_blank"
            rel="noopener noreferrer"
            className="rounded-lg p-2 transition-colors"
            style={{ color: 'rgb(var(--text-secondary))' }}
            onMouseEnter={(e) => {
              (e.currentTarget as HTMLElement).style.backgroundColor = 'rgb(var(--bg-elevated))';
              (e.currentTarget as HTMLElement).style.color = 'rgb(var(--text-primary))';
            }}
            onMouseLeave={(e) => {
              (e.currentTarget as HTMLElement).style.backgroundColor = 'transparent';
              (e.currentTarget as HTMLElement).style.color = 'rgb(var(--text-secondary))';
            }}
            aria-label="GitHub"
          >
            <Github size={18} />
          </a>
        </div>

        {/* Mobile hamburger */}
        <button
          onClick={() => setMobileMenuOpen(!mobileMenuOpen)}
          className="rounded-lg p-2 sm:hidden"
          style={{ color: 'rgb(var(--text-secondary))' }}
          aria-label="Menu"
        >
          {mobileMenuOpen ? <X size={20} /> : <Menu size={20} />}
        </button>
      </div>

      {/* Mobile menu */}
      {mobileMenuOpen && (
        <div
          className="border-t px-4 py-3 sm:hidden"
          style={{
            borderColor: 'rgb(var(--border))',
            backgroundColor: 'rgb(var(--bg-surface))',
          }}
        >
          <div className="flex items-center gap-3">
            <button
              onClick={() => { toggleTheme(); setMobileMenuOpen(false); }}
              className="flex min-h-[44px] items-center gap-2 rounded-lg px-3 py-2 text-sm"
              style={{ color: 'rgb(var(--text-secondary))' }}
            >
              {theme === 'dark' ? <Sun size={18} /> : <Moon size={18} />}
              {theme === 'dark' ? 'Light mode' : 'Dark mode'}
            </button>
            <a
              href="https://github.com/neokapi/neokapi"
              target="_blank"
              rel="noopener noreferrer"
              className="flex min-h-[44px] items-center gap-2 rounded-lg px-3 py-2 text-sm"
              style={{ color: 'rgb(var(--text-secondary))' }}
            >
              <Github size={18} />
              GitHub
            </a>
          </div>
        </div>
      )}
    </header>
  );
}
