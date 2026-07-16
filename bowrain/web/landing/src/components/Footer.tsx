import { Logo } from "./Logo";
import { CONTACT_EMAIL, GITHUB_URL, docsUrl } from "../links";

// Injected at build time by vite.config.ts `define`.
declare const __BUILD_STAMP__: string;

export function Footer() {
  return (
    <footer className="border-t border-border/60 px-6 py-8">
      <div className="mx-auto flex max-w-6xl flex-col items-center justify-between gap-4 sm:flex-row">
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <Logo className="h-5 w-5" />
          bowrain
        </div>
        <div className="flex gap-6 text-xs text-muted-foreground/80">
          <a
            href={GITHUB_URL}
            target="_blank"
            rel="noopener"
            className="transition hover:text-foreground"
          >
            GitHub
          </a>
          <a href={docsUrl()} className="transition hover:text-foreground">
            Docs
          </a>
          <a href={`mailto:${CONTACT_EMAIL}`} className="transition hover:text-foreground">
            Contact
          </a>
        </div>
        <div className="text-xs text-muted-foreground/60">
          AGPL-3.0 · open core on kapi (Apache-2.0)
        </div>
      </div>
      <div className="mx-auto mt-6 max-w-6xl text-center text-[11px] tabular-nums text-muted-foreground/50">
        built {__BUILD_STAMP__}
      </div>
    </footer>
  );
}
