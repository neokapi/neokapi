// Injected at build time by vite.config.ts `define`.
declare const __BUILD_STAMP__: string;

export function Footer() {
  return (
    <footer className="border-t border-neutral-800/50 px-6 py-8">
      <div className="mx-auto flex max-w-6xl flex-col items-center justify-between gap-4 sm:flex-row">
        <div className="flex items-center gap-2 text-sm text-neutral-500">
          <img
            src={`${import.meta.env.BASE_URL}favicon.svg`}
            alt="Bowrain logo"
            className="h-5 w-5"
          />
          bowrain
        </div>
        <div className="flex gap-6 text-xs text-neutral-600">
          <a
            href="https://github.com/neokapi"
            target="_blank"
            rel="noopener"
            className="transition hover:text-neutral-400"
          >
            GitHub
          </a>
          <a
            href="https://neokapi.github.io/web/bowrain/docs/"
            className="transition hover:text-neutral-400"
          >
            Docs
          </a>
          <a href="mailto:hello@bowrain.com" className="transition hover:text-neutral-400">
            Contact
          </a>
        </div>
        <div className="text-xs text-neutral-700">AGPL-3.0 · open core on kapi (Apache-2.0)</div>
      </div>
      <div className="mx-auto mt-6 max-w-6xl text-center text-[11px] tabular-nums text-neutral-700/80">
        built {__BUILD_STAMP__}
      </div>
    </footer>
  );
}
