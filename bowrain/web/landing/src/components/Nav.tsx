import { Menu, Moon, Sun, X } from "lucide-react";
import { useState } from "react";
import { t } from "@neokapi/i18n-react/runtime";
import { GithubIcon } from "./GithubIcon";
import { Logo } from "./Logo";
import { APP_URL, GITHUB_URL, SIGNUP_URL, docsUrl } from "../links";
import { isDark, toggleMode } from "../theme";
import { useAuthStatus, type AuthStatus } from "../useAuthStatus";

// The four beats a visitor is most likely to want to jump to, in narrative
// order. They are anchors into the six-section argument, not a site map.
const LINKS = [
  { href: "#how", label: t("How it works") },
  { href: "#loop", label: t("The loop") },
  { href: "#proof", label: t("Try it") },
  { href: "#pricing", label: t("Pricing") },
];

// Small round avatar: the user's picture when present, otherwise the first
// initial of their name. Marketing-restrained (no menu, no dropdown).
function Avatar({ status }: { status: AuthStatus }) {
  if (status.avatarUrl) {
    return (
      <img
        src={status.avatarUrl}
        alt=""
        width={28}
        height={28}
        loading="lazy"
        referrerPolicy="no-referrer"
        className="h-7 w-7 rounded-full object-cover ring-1 ring-border"
      />
    );
  }
  const initial = status.name.trim().charAt(0).toUpperCase() || "?";
  return (
    <span
      aria-hidden
      className="flex h-7 w-7 items-center justify-center rounded-full bg-secondary text-xs font-medium text-foreground ring-1 ring-border"
    >
      {initial}
    </span>
  );
}

export function Nav() {
  const [open, setOpen] = useState(false);
  const [dark, setDark] = useState(isDark());
  const { status } = useAuthStatus();
  const signedIn = status !== null;

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
          {signedIn ? (
            <>
              {status.name && (
                <span className="flex items-center gap-2 pl-1">
                  <Avatar status={status} />
                  <span
                    translate="no"
                    className="max-w-[9rem] truncate text-sm text-muted-foreground"
                  >
                    {status.name}
                  </span>
                </span>
              )}
              <a
                href={APP_URL}
                className="rounded-lg bg-primary px-3.5 py-1.5 text-sm font-medium text-primary-foreground transition hover:opacity-90"
              >
                Go to app
              </a>
            </>
          ) : (
            <>
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
            </>
          )}
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
            {signedIn && status.name && (
              <div className="flex items-center gap-2 py-1">
                <Avatar status={status} />
                <span translate="no" className="truncate text-sm text-muted-foreground">
                  {status.name}
                </span>
              </div>
            )}
            <div className="mt-2 flex items-center gap-3">
              {signedIn ? (
                <a
                  href={APP_URL}
                  className="rounded-lg bg-primary px-3.5 py-1.5 text-sm font-medium text-primary-foreground"
                >
                  Go to app
                </a>
              ) : (
                <a
                  href={SIGNUP_URL}
                  className="rounded-lg bg-primary px-3.5 py-1.5 text-sm font-medium text-primary-foreground"
                >
                  Get started
                </a>
              )}
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
