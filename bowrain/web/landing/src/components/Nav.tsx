import { Menu, Moon, Sun, X } from "lucide-react";
import { useState } from "react";
import { t } from "@neokapi/kapi-react/runtime";
import { GithubIcon } from "./GithubIcon";
import { Logo } from "./Logo";
import { APP_URL, GITHUB_URL, SIGNUP_URL, docsUrl } from "../links";
import { isDark, toggleMode } from "../theme";

const LINKS = [
  { href: "#product", label: t("Product") },
  { href: "#loop", label: t("How it works") },
  { href: "#try", label: t("Try it") },
  { href: "#pricing", label: t("Pricing") },
];

export function Nav() {
  const [open, setOpen] = useState(false);
  const [dark, setDark] = useState(isDark());

  return (
    <nav className="fixed top-0 left-0 right-0 z-50 border-b border-border/60 bg-background/80 backdrop-blur-xl">
      <div className="mx-auto flex h-14 max-w-6xl items-center justify-between px-6">
        <a href="#" className="flex items-center gap-2 text-lg font-semibold tracking-tight">
          <Logo className="h-7 w-7" />
          bowrain
        </a>

        <div className="hidden items-center gap-7 md:flex">
          {LINKS.map((l) => (
            <a
              key={l.href}
              href={l.href}
              translate="no"
              className="text-sm text-muted-foreground transition hover:text-foreground"
            >
              {l.label}
            </a>
          ))}
          <a
            href={docsUrl()}
            className="text-sm text-muted-foreground transition hover:text-foreground"
          >
            Docs
          </a>
        </div>

        <div className="hidden items-center gap-2 md:flex">
          <button
            type="button"
            onClick={() => setDark(toggleMode())}
            aria-label="Toggle color theme"
            className="flex h-8 w-8 items-center justify-center rounded-lg text-muted-foreground transition hover:bg-secondary hover:text-foreground"
          >
            {dark ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
          </button>
          <a
            href={GITHUB_URL}
            target="_blank"
            rel="noopener"
            aria-label="GitHub"
            className="flex h-8 w-8 items-center justify-center rounded-lg text-muted-foreground transition hover:bg-secondary hover:text-foreground"
          >
            <GithubIcon className="h-4 w-4" />
          </a>
          <a
            href={APP_URL}
            className="rounded-lg px-3 py-1.5 text-sm text-muted-foreground transition hover:bg-secondary hover:text-foreground"
          >
            Sign in
          </a>
          <a
            href={SIGNUP_URL}
            className="rounded-lg bg-primary px-3.5 py-1.5 text-sm font-medium text-primary-foreground transition hover:opacity-90"
          >
            Get started
          </a>
        </div>

        <button
          onClick={() => setOpen(!open)}
          className="text-muted-foreground md:hidden"
          aria-label="Toggle menu"
        >
          {open ? <X className="h-5 w-5" /> : <Menu className="h-5 w-5" />}
        </button>
      </div>

      {open && (
        <div className="border-t border-border/60 bg-background/95 px-6 py-4 md:hidden">
          <div className="flex flex-col gap-3">
            {LINKS.map((l) => (
              <a
                key={l.href}
                href={l.href}
                onClick={() => setOpen(false)}
                translate="no"
                className="text-sm text-muted-foreground"
              >
                {l.label}
              </a>
            ))}
            <a
              href={docsUrl()}
              onClick={() => setOpen(false)}
              className="text-sm text-muted-foreground"
            >
              Docs
            </a>
            <div className="mt-2 flex items-center gap-3">
              <a
                href={SIGNUP_URL}
                className="rounded-lg bg-primary px-3.5 py-1.5 text-sm font-medium text-primary-foreground"
              >
                Get started
              </a>
              <button
                type="button"
                onClick={() => setDark(toggleMode())}
                aria-label="Toggle color theme"
                className="flex h-8 w-8 items-center justify-center rounded-lg text-muted-foreground"
              >
                {dark ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
              </button>
            </div>
          </div>
        </div>
      )}
    </nav>
  );
}
