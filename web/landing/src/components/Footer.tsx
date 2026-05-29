export function Footer() {
  return (
    <footer className="border-t border-surface-700/50 px-6 py-10">
      <div className="mx-auto flex max-w-6xl flex-col items-center justify-between gap-6 sm:flex-row">
        <div className="flex items-center gap-2.5">
          <img
            src={`${import.meta.env.BASE_URL}hero-logo.png`}
            alt="Neokapi"
            className="h-6 w-6 rounded"
          />
          <span className="font-display text-sm font-semibold text-neutral-500">neokapi</span>
        </div>

        <div className="flex flex-wrap items-center justify-center gap-6 text-xs text-neutral-600">
          <a
            href="https://github.com/neokapi/neokapi"
            target="_blank"
            rel="noopener"
            className="transition hover:text-brand-400"
          >
            GitHub
          </a>
          <a
            href="https://neokapi.github.io/web/neokapi/docs/"
            target="_blank"
            rel="noopener"
            className="transition hover:text-brand-400"
          >
            Documentation
          </a>
          <a
            href="https://github.com/neokapi/neokapi/releases"
            target="_blank"
            rel="noopener"
            className="transition hover:text-brand-400"
          >
            Releases
          </a>
        </div>

        <div className="text-xs text-neutral-700">Apache 2.0</div>
      </div>
    </footer>
  );
}
