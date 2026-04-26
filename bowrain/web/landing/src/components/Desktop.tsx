import { Apple, Monitor, Download } from 'lucide-react'

export function Desktop() {
  return (
    <section id="desktop" className="mx-auto max-w-6xl px-6 py-24">
      <div className="mx-auto max-w-3xl text-center">
        <div className="mb-4 inline-flex items-center gap-2 rounded-full border border-neutral-800 px-3 py-1 text-xs text-neutral-400 font-mono">
          DESKTOP APP
        </div>
        <h2 className="text-2xl font-bold tracking-tight text-white sm:text-3xl">
          Bowrain Desktop
        </h2>
        <p className="mt-3 text-neutral-400">
          The same translation editor, running natively on your machine.
          Works offline — changes sync when you reconnect.
        </p>
      </div>

      {/* App screenshot mockup */}
      <div className="mt-12 overflow-hidden rounded-xl border border-neutral-800 bg-neutral-900/50 shadow-2xl shadow-brand-500/5">
        {/* Native window chrome */}
        <div className="flex items-center gap-2 border-b border-neutral-800 bg-neutral-900/80 px-4 py-2.5">
          <div className="h-2.5 w-2.5 rounded-full bg-red-500/60" />
          <div className="h-2.5 w-2.5 rounded-full bg-yellow-500/60" />
          <div className="h-2.5 w-2.5 rounded-full bg-green-500/60" />
          <span className="ml-2 text-xs text-neutral-500">Bowrain — acme-website / en → de</span>
          <div className="ml-auto flex gap-3 text-xs text-neutral-600">
            <span>47 strings</span>
            <span className="text-suggestion">42 done</span>
            <span className="text-violation">3 review</span>
            <span>2 new</span>
          </div>
        </div>

        {/* Editor mockup */}
        <div className="grid grid-cols-[200px_1fr_1fr] min-h-[380px] text-xs">
          {/* Sidebar */}
          <div className="border-r border-neutral-800 bg-neutral-950/50 p-3">
            <div className="mb-3 text-[10px] uppercase tracking-wider text-neutral-600">Files</div>
            {[
              { name: 'nav.json', done: '8/8', color: 'text-suggestion' },
              { name: 'hero.json', done: '6/6', color: 'text-suggestion' },
              { name: 'settings.json', done: '12/14', color: 'text-violation' },
              { name: 'onboarding.json', done: '9/11', color: 'text-neutral-400' },
              { name: 'errors.json', done: '7/8', color: 'text-neutral-400' },
            ].map(f => (
              <div
                key={f.name}
                className={`flex items-center justify-between rounded px-2 py-1.5 ${
                  f.name === 'settings.json' ? 'bg-neutral-800/60 text-white' : 'text-neutral-500'
                }`}
              >
                <span className="truncate">{f.name}</span>
                <span className={`font-mono ${f.color}`}>{f.done}</span>
              </div>
            ))}
            <div className="mt-6 mb-3 text-[10px] uppercase tracking-wider text-neutral-600">Glossary</div>
            <div className="space-y-1 text-neutral-600">
              <div className="rounded bg-neutral-800/30 px-2 py-1">workspace → Arbeitsbereich</div>
              <div className="rounded bg-neutral-800/30 px-2 py-1">dashboard → Übersicht</div>
              <div className="rounded bg-neutral-800/30 px-2 py-1">deploy → bereitstellen</div>
            </div>
          </div>

          {/* Source column */}
          <div className="border-r border-neutral-800 p-3">
            <div className="mb-3 flex items-center justify-between">
              <span className="text-[10px] uppercase tracking-wider text-neutral-600">English (source)</span>
              <span className="rounded bg-neutral-800/50 px-1.5 py-0.5 text-[10px] text-neutral-600">settings.json</span>
            </div>
            <div className="space-y-1">
              {[
                { text: 'Workspace settings', status: 'done' },
                { text: 'Manage your notification preferences', status: 'done' },
                { text: 'Choose your default language', status: 'review' },
                { text: 'Enable two-factor authentication', status: 'review' },
                { text: 'Connected integrations', status: 'active' },
                { text: 'API access tokens', status: 'done' },
                { text: 'Danger zone', status: 'done' },
              ].map((s, i) => (
                <div
                  key={i}
                  className={`flex items-center gap-2 rounded px-2 py-2 ${
                    s.status === 'active' ? 'bg-brand-500/10 border border-brand-500/30' : 'bg-neutral-800/20'
                  }`}
                >
                  <span className={`h-1.5 w-1.5 shrink-0 rounded-full ${
                    s.status === 'done' ? 'bg-suggestion' : s.status === 'review' ? 'bg-violation' : 'bg-brand-400'
                  }`} />
                  <span className={s.status === 'active' ? 'text-neutral-200' : 'text-neutral-400'}>{s.text}</span>
                </div>
              ))}
            </div>
          </div>

          {/* Target column */}
          <div className="p-3">
            <div className="mb-3 flex items-center justify-between">
              <span className="text-[10px] uppercase tracking-wider text-neutral-600">German (target)</span>
              <div className="flex gap-1">
                <span className="rounded bg-suggestion/10 px-1.5 py-0.5 text-[10px] text-suggestion">TM 100%</span>
              </div>
            </div>
            <div className="space-y-1">
              {[
                { text: 'Arbeitsbereich-Einstellungen', status: 'done' },
                { text: 'Benachrichtigungseinstellungen verwalten', status: 'done' },
                { text: 'Standardsprache auswählen', status: 'review', note: 'term: "default" → review' },
                { text: 'Zwei-Faktor-Authentifizierung aktivieren', status: 'review' },
                { text: '', status: 'active', placeholder: 'Translating...' },
                { text: 'API-Zugriffstoken', status: 'done' },
                { text: 'Gefahrenzone', status: 'done' },
              ].map((s, i) => (
                <div
                  key={i}
                  className={`rounded px-2 py-2 ${
                    s.status === 'active' ? 'bg-brand-500/10 border border-brand-500/30' : 'bg-neutral-800/20'
                  }`}
                >
                  <div className="flex items-center gap-2">
                    <span className={`h-1.5 w-1.5 shrink-0 rounded-full ${
                      s.status === 'done' ? 'bg-suggestion' : s.status === 'review' ? 'bg-violation' : 'bg-brand-400'
                    }`} />
                    {s.text ? (
                      <span className={s.status === 'active' ? 'text-neutral-200' : 'text-neutral-400'}>{s.text}</span>
                    ) : (
                      <span className="text-brand-400/60 italic">{s.placeholder}</span>
                    )}
                  </div>
                  {s.note && (
                    <div className="ml-3.5 mt-1 text-[10px] text-violation">{s.note}</div>
                  )}
                </div>
              ))}
            </div>

            {/* Context panel hint */}
            <div className="mt-4 rounded-lg border border-neutral-800 bg-neutral-950/50 p-2.5">
              <div className="mb-1.5 text-[10px] uppercase tracking-wider text-neutral-600">Translation suggestions</div>
              <div className="space-y-1">
                <div className="flex items-center justify-between">
                  <span className="text-neutral-400">Verbundene Integrationen</span>
                  <span className="rounded bg-suggestion/10 px-1.5 py-0.5 text-[10px] text-suggestion">98%</span>
                </div>
                <div className="flex items-center justify-between">
                  <span className="text-neutral-500">Angeschlossene Integrationen</span>
                  <span className="rounded bg-neutral-800 px-1.5 py-0.5 text-[10px] text-neutral-500">82%</span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* Download links */}
      <div className="mt-8 flex flex-wrap items-center justify-center gap-4">
        <a
          href="#download-mac"
          className="flex items-center gap-2 rounded-lg border border-neutral-700 bg-neutral-900/50 px-5 py-2.5 text-sm text-neutral-300 transition hover:border-neutral-500 hover:text-white"
        >
          <Apple className="h-4 w-4" />
          macOS
        </a>
        <a
          href="#download-windows"
          className="flex items-center gap-2 rounded-lg border border-neutral-700 bg-neutral-900/50 px-5 py-2.5 text-sm text-neutral-300 transition hover:border-neutral-500 hover:text-white"
        >
          <Monitor className="h-4 w-4" />
          Windows
        </a>
        <a
          href="#download-linux"
          className="flex items-center gap-2 rounded-lg border border-neutral-700 bg-neutral-900/50 px-5 py-2.5 text-sm text-neutral-300 transition hover:border-neutral-500 hover:text-white"
        >
          <Download className="h-4 w-4" />
          Linux
        </a>
      </div>
      <p className="mt-3 text-center text-xs text-neutral-600">
        Works offline. Changes sync automatically when you reconnect.
      </p>
    </section>
  )
}
