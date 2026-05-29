export function Footer() {
  return (
    <footer className="border-t border-neutral-800/50 px-6 py-8">
      <div className="mx-auto flex max-w-6xl flex-col items-center justify-between gap-4 sm:flex-row">
        <div className="flex items-center gap-2 text-sm text-neutral-500">
          <div className="flex h-5 w-5 items-center justify-center rounded bg-brand-500 text-xs font-bold text-white">
            B
          </div>
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
        <div className="text-xs text-neutral-700">Apache 2.0</div>
      </div>
    </footer>
  );
}
