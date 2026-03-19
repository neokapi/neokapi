import { Github, Moon, Sun, Menu, X } from 'lucide-react';
import { useState } from 'react';
import { useTheme } from '../../context/ThemeContext';
import { Button } from '@/components/ui/button';

export default function Header() {
  const { theme, toggleTheme } = useTheme();
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false);

  return (
    <header className="sticky top-0 z-50 border-b bg-background/95 backdrop-blur-sm">
      <div className="mx-auto flex max-w-7xl items-center justify-between px-4 py-3 sm:px-6">
        <div className="flex items-center gap-3 sm:gap-4">
          <span className="text-xl font-semibold tracking-tight sm:text-2xl">
            bowrain
          </span>
          <span className="hidden rounded-full border px-3 py-0.5 font-mono text-xs text-muted-foreground sm:inline-block">
            agents.dev.bowrain.cloud
          </span>
        </div>

        {/* Desktop controls */}
        <div className="hidden items-center gap-1 sm:flex">
          <Button
            variant="ghost"
            size="icon"
            onClick={toggleTheme}
            aria-label="Toggle theme"
          >
            {theme === 'dark' ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
          </Button>
          <a
            href="https://github.com/neokapi/neokapi"
            target="_blank"
            rel="noopener noreferrer"
            aria-label="GitHub"
            className="inline-flex size-8 items-center justify-center rounded-lg hover:bg-muted"
          >
            <Github className="h-4 w-4" />
          </a>
        </div>

        {/* Mobile hamburger */}
        <Button
          variant="ghost"
          size="icon"
          className="sm:hidden"
          onClick={() => setMobileMenuOpen(!mobileMenuOpen)}
          aria-label="Menu"
        >
          {mobileMenuOpen ? <X className="h-5 w-5" /> : <Menu className="h-5 w-5" />}
        </Button>
      </div>

      {/* Mobile menu */}
      {mobileMenuOpen && (
        <div className="border-t bg-card px-4 py-3 sm:hidden">
          <div className="flex items-center gap-3">
            <Button
              variant="ghost"
              size="sm"
              onClick={() => { toggleTheme(); setMobileMenuOpen(false); }}
            >
              {theme === 'dark' ? <Sun className="mr-2 h-4 w-4" /> : <Moon className="mr-2 h-4 w-4" />}
              {theme === 'dark' ? 'Light mode' : 'Dark mode'}
            </Button>
            <a
              href="https://github.com/neokapi/neokapi"
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex h-7 items-center gap-2 rounded-lg px-2.5 text-sm hover:bg-muted"
            >
              <Github className="h-4 w-4" />
              GitHub
            </a>
          </div>
        </div>
      )}
    </header>
  );
}
