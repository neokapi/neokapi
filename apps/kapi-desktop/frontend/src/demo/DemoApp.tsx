/**
 * DemoApp — a self-contained, recording-only build of Kapi Desktop.
 *
 * Renders the REAL app chrome (IconSidebar) and the REAL termbase / TM browser
 * components from @neokapi/ui-primitives, wired to in-browser mock adapters with
 * a rich, coherent sample glossary + translation memory. There is no Wails
 * backend in play — this entry exists purely so the walkthrough recorder
 * (harness/) can drive the genuine UI in a browser and capture a clean,
 * deterministic screencast (ripple clicks, human-like mouse motion, zoom).
 *
 * Not shipped in the desktop app: only `demo.html` mounts this. Toggle dark
 * mode with `?theme=dark`.
 */
import { useMemo, useState } from "react";
import {
  BookOpen,
  Database,
  Plus,
  FolderOpen,
  X,
  Upload,
  Download,
  ArrowRight,
} from "lucide-react";
import {
  Button,
  PageHeader,
  ResourceCard,
  TMBrowser,
  TermbaseBrowser,
  type TMAdapter,
  type TermbaseAdapter,
  type ConceptDTO,
  type TMEntryDTO,
} from "@neokapi/ui-primitives";
import { IconSidebar } from "../components/IconSidebar";

// ── Locale list (nicer language names in the browsers) ──────────────────────
const LOCALES = [
  { code: "en-US", displayName: "English (United States)" },
  { code: "fr-FR", displayName: "French (France)" },
  { code: "de-DE", displayName: "German (Germany)" },
  { code: "ja-JP", displayName: "Japanese (Japan)" },
  { code: "es-ES", displayName: "Spanish (Spain)" },
];

const hoursAgo = (h: number) => new Date(Date.now() - h * 3600_000).toISOString();

// ── Sample termbase: a product glossary for a fictional analytics app ───────
const CONCEPTS: ConceptDTO[] = [
  {
    id: "c-dashboard",
    project_id: "",
    domain: "Product",
    definition: "The landing screen summarising a workspace's key metrics.",
    source: "terminology",
    terms: [
      { text: "dashboard", locale: "en-US", status: "preferred", part_of_speech: "noun" },
      { text: "tableau de bord", locale: "fr-FR", status: "approved" },
      { text: "Dashboard", locale: "de-DE", status: "approved" },
      { text: "ダッシュボード", locale: "ja-JP", status: "approved" },
    ],
    created_at: hoursAgo(40),
    updated_at: hoursAgo(2),
  },
  {
    id: "c-workspace",
    project_id: "",
    domain: "Product",
    definition: "A container that groups projects, members and billing.",
    source: "terminology",
    terms: [
      { text: "workspace", locale: "en-US", status: "preferred" },
      { text: "espace de travail", locale: "fr-FR", status: "approved" },
      { text: "Arbeitsbereich", locale: "de-DE", status: "approved" },
    ],
    created_at: hoursAgo(60),
    updated_at: hoursAgo(8),
  },
  {
    id: "c-seat",
    project_id: "",
    domain: "Billing",
    definition: "A paid licence assigned to one member of a workspace.",
    source: "terminology",
    terms: [
      { text: "seat", locale: "en-US", status: "preferred" },
      { text: "siège", locale: "fr-FR", status: "approved" },
      { text: "licence", locale: "fr-FR", status: "deprecated", note: "Use 'siège'." },
      { text: "Sitzplatz", locale: "de-DE", status: "approved" },
    ],
    created_at: hoursAgo(90),
    updated_at: hoursAgo(30),
  },
  {
    id: "c-webhook",
    project_id: "",
    domain: "Engineering",
    definition: "An HTTP callback fired when an event occurs.",
    source: "terminology",
    terms: [
      { text: "webhook", locale: "en-US", status: "preferred" },
      { text: "webhook", locale: "fr-FR", status: "approved", note: "Keep in English." },
      { text: "Webhook", locale: "de-DE", status: "approved" },
    ],
    created_at: hoursAgo(120),
    updated_at: hoursAgo(120),
  },
  {
    id: "c-onboarding",
    project_id: "",
    domain: "Marketing",
    definition: "The guided first-run experience for new members.",
    source: "terminology",
    terms: [
      { text: "onboarding", locale: "en-US", status: "preferred" },
      { text: "intégration", locale: "fr-FR", status: "approved" },
      { text: "Einarbeitung", locale: "de-DE", status: "proposed" },
    ],
    created_at: hoursAgo(150),
    updated_at: hoursAgo(150),
  },
  {
    id: "c-retention",
    project_id: "",
    domain: "Analytics",
    definition: "The share of users who return over a period.",
    source: "terminology",
    terms: [
      { text: "retention", locale: "en-US", status: "preferred" },
      { text: "rétention", locale: "fr-FR", status: "approved" },
      { text: "Bindung", locale: "de-DE", status: "approved" },
    ],
    created_at: hoursAgo(180),
    updated_at: hoursAgo(170),
  },
  {
    id: "c-invoice",
    project_id: "",
    domain: "Billing",
    definition: "A document itemising charges for a billing period.",
    source: "terminology",
    terms: [
      { text: "invoice", locale: "en-US", status: "preferred" },
      { text: "facture", locale: "fr-FR", status: "approved" },
      { text: "Rechnung", locale: "de-DE", status: "approved" },
    ],
    created_at: hoursAgo(210),
    updated_at: hoursAgo(200),
  },
  {
    id: "c-export",
    project_id: "",
    domain: "Product",
    definition: "Downloading data out of the app in a portable format.",
    source: "terminology",
    terms: [
      { text: "export", locale: "en-US", status: "preferred" },
      { text: "exporter", locale: "fr-FR", status: "approved", part_of_speech: "verb" },
      { text: "Export", locale: "de-DE", status: "approved" },
    ],
    created_at: hoursAgo(240),
    updated_at: hoursAgo(220),
  },
];

// ── Sample translation memory: real UI strings, with inline code + an entity ─
const TM_ENTRIES: TMEntryDTO[] = [
  {
    id: "tm-1",
    project_id: "",
    hint_src_lang: "en-US",
    variants: {
      "en-US": { locale: "en-US", text: "Welcome back", runs: [{ text: "Welcome back" }] },
      "fr-FR": { locale: "fr-FR", text: "Bon retour", runs: [{ text: "Bon retour" }] },
      "de-DE": {
        locale: "de-DE",
        text: "Willkommen zurück",
        runs: [{ text: "Willkommen zurück" }],
      },
    },
    created_at: hoursAgo(3),
    updated_at: hoursAgo(3),
  },
  {
    id: "tm-2",
    project_id: "",
    hint_src_lang: "en-US",
    variants: {
      "en-US": {
        locale: "en-US",
        text: "Click here to continue",
        runs: [
          { text: "Click " },
          { pcOpen: { id: "1", type: "fmt:bold", data: "<b>", equiv: "b" } },
          { text: "here" },
          { pcClose: { id: "1", type: "fmt:bold", data: "</b>", equiv: "b" } },
          { text: " to continue" },
        ],
      },
      "fr-FR": {
        locale: "fr-FR",
        text: "Cliquez ici pour continuer",
        runs: [
          { text: "Cliquez " },
          { pcOpen: { id: "1", type: "fmt:bold", data: "<b>", equiv: "b" } },
          { text: "ici" },
          { pcClose: { id: "1", type: "fmt:bold", data: "</b>", equiv: "b" } },
          { text: " pour continuer" },
        ],
      },
    },
    created_at: hoursAgo(5),
    updated_at: hoursAgo(5),
  },
  {
    id: "tm-3",
    project_id: "",
    hint_src_lang: "en-US",
    variants: {
      "en-US": {
        locale: "en-US",
        text: "Invite teammates to your workspace",
        runs: [{ text: "Invite teammates to your workspace" }],
      },
      "fr-FR": {
        locale: "fr-FR",
        text: "Invitez des collègues dans votre espace de travail",
        runs: [{ text: "Invitez des collègues dans votre espace de travail" }],
      },
      "de-DE": {
        locale: "de-DE",
        text: "Laden Sie Teammitglieder in Ihren Arbeitsbereich ein",
        runs: [{ text: "Laden Sie Teammitglieder in Ihren Arbeitsbereich ein" }],
      },
    },
    created_at: hoursAgo(9),
    updated_at: hoursAgo(9),
  },
  {
    id: "tm-4",
    project_id: "",
    hint_src_lang: "en-US",
    variants: {
      "en-US": {
        locale: "en-US",
        text: "Your invoice is ready",
        runs: [{ text: "Your invoice is ready" }],
      },
      "fr-FR": {
        locale: "fr-FR",
        text: "Votre facture est prête",
        runs: [{ text: "Votre facture est prête" }],
      },
    },
    created_at: hoursAgo(20),
    updated_at: hoursAgo(20),
  },
  {
    id: "tm-5",
    project_id: "",
    hint_src_lang: "en-US",
    variants: {
      "en-US": {
        locale: "en-US",
        text: "Hi Bob, your report is ready",
        runs: [
          { text: "Hi " },
          { ph: { id: "e1", type: "entity:person", data: "Bob", equiv: "Bob" } },
          { text: ", your report is ready" },
        ],
      },
      "fr-FR": {
        locale: "fr-FR",
        text: "Bonjour Bob, votre rapport est prêt",
        runs: [
          { text: "Bonjour " },
          { ph: { id: "e1", type: "entity:person", data: "Bob", equiv: "Bob" } },
          { text: ", votre rapport est prêt" },
        ],
      },
    },
    created_at: hoursAgo(28),
    updated_at: hoursAgo(28),
  },
  {
    id: "tm-6",
    project_id: "",
    hint_src_lang: "en-US",
    variants: {
      "en-US": { locale: "en-US", text: "Settings saved", runs: [{ text: "Settings saved" }] },
      "fr-FR": {
        locale: "fr-FR",
        text: "Paramètres enregistrés",
        runs: [{ text: "Paramètres enregistrés" }],
      },
      "de-DE": {
        locale: "de-DE",
        text: "Einstellungen gespeichert",
        runs: [{ text: "Einstellungen gespeichert" }],
      },
    },
    created_at: hoursAgo(36),
    updated_at: hoursAgo(36),
  },
  {
    id: "tm-7",
    project_id: "",
    hint_src_lang: "en-US",
    variants: {
      "en-US": {
        locale: "en-US",
        text: "Export your data",
        runs: [{ text: "Export your data" }],
      },
      "fr-FR": {
        locale: "fr-FR",
        text: "Exportez vos données",
        runs: [{ text: "Exportez vos données" }],
      },
    },
    created_at: hoursAgo(48),
    updated_at: hoursAgo(48),
  },
];

// ── Mock adapters (in-browser, no backend) ──────────────────────────────────
function termbaseAdapter(seed: ConceptDTO[]): TermbaseAdapter {
  let data = [...seed];
  return {
    async search(query) {
      const q = query.trim().toLowerCase();
      const filtered = q
        ? data.filter(
            (c) =>
              c.domain.toLowerCase().includes(q) ||
              c.definition.toLowerCase().includes(q) ||
              c.terms.some((t) => t.text.toLowerCase().includes(q)),
          )
        : data;
      return { concepts: filtered, total_count: filtered.length };
    },
    async getConcept(id) {
      return data.find((c) => c.id === id) ?? null;
    },
    async addConcept() {},
    async updateConcept() {},
    async deleteConcept(id) {
      data = data.filter((c) => c.id !== id);
    },
    async deleteConcepts(ids) {
      const s = new Set(ids);
      data = data.filter((c) => !s.has(c.id));
    },
  };
}

function tmAdapter(seed: TMEntryDTO[]): TMAdapter {
  let data = [...seed];
  const hit = (e: TMEntryDTO, q: string) =>
    Object.values(e.variants).some((v) => v.text.toLowerCase().includes(q));
  return {
    async search(query) {
      const q = query.trim().toLowerCase();
      const filtered = q ? data.filter((e) => hit(e, q)) : data;
      return { entries: filtered, total_count: filtered.length };
    },
    async getEntry(id) {
      return data.find((e) => e.id === id) ?? null;
    },
    async addEntry() {},
    async updateEntry() {},
    async deleteEntry(id) {
      data = data.filter((e) => e.id !== id);
    },
    async deleteEntries(ids) {
      const s = new Set(ids);
      data = data.filter((e) => !s.has(e.id));
    },
  };
}

// ── Resource lists for the picker screens ───────────────────────────────────
const TERMBASE_RESOURCES = [
  {
    name: "product-glossary",
    path: "~/.config/kapi/termbases/product-glossary.db",
    size: 262144,
    modified: hoursAgo(2),
  },
  {
    name: "brand-terms",
    path: "~/.config/kapi/termbases/brand-terms.db",
    size: 131072,
    modified: hoursAgo(48),
  },
];

const TM_RESOURCES = [
  {
    name: "acme-app",
    path: "~/.config/kapi/tm/acme-app.db",
    size: 786432,
    modified: hoursAgo(3),
  },
  {
    name: "global-tm",
    path: "~/.config/kapi/tm/global-tm.db",
    size: 1572864,
    modified: hoursAgo(72),
  },
];

// ── Pages ───────────────────────────────────────────────────────────────────
function HomeView({ onGo }: { onGo: (v: string) => void }) {
  return (
    <div className="flex h-full flex-col items-center justify-center gap-8 p-10 text-center">
      <div>
        <h1 className="text-3xl font-semibold tracking-tight">Kapi Desktop</h1>
        <p className="mt-2 max-w-xl text-muted-foreground">
          Browse the terminology and translation memory behind your localisation — every concept,
          every approved term, every remembered translation.
        </p>
      </div>
      <div className="grid w-full max-w-2xl grid-cols-1 gap-3 md:grid-cols-2">
        <button
          onClick={() => onGo("termbases")}
          className="group flex flex-col items-start gap-2 rounded-xl border border-border bg-card p-5 text-left transition-all hover:border-primary/40 hover:shadow-md"
        >
          <BookOpen size={22} className="text-primary" />
          <div className="text-base font-semibold">Termbases</div>
          <div className="text-sm text-muted-foreground">
            Multi-locale concepts with preferred and deprecated terms.
          </div>
          <span className="mt-1 inline-flex items-center gap-1 text-sm font-medium text-primary opacity-0 transition-opacity group-hover:opacity-100">
            Open <ArrowRight size={14} />
          </span>
        </button>
        <button
          onClick={() => onGo("memories")}
          className="group flex flex-col items-start gap-2 rounded-xl border border-border bg-card p-5 text-left transition-all hover:border-primary/40 hover:shadow-md"
        >
          <Database size={22} className="text-primary" />
          <div className="text-base font-semibold">Translation Memories</div>
          <div className="text-sm text-muted-foreground">
            Searchable past translations, inline formatting preserved.
          </div>
          <span className="mt-1 inline-flex items-center gap-1 text-sm font-medium text-primary opacity-0 transition-opacity group-hover:opacity-100">
            Open <ArrowRight size={14} />
          </span>
        </button>
      </div>
    </div>
  );
}

function TermbasesView() {
  const [open, setOpen] = useState<string | null>(null);
  const adapter = useMemo(() => termbaseAdapter(CONCEPTS), []);
  if (open) {
    return (
      <div className="p-6">
        <PageHeader
          title={open}
          subtitle={`~/.config/kapi/termbases/${open}.db`}
          backButton={
            <Button variant="ghost" size="icon-xs" onClick={() => setOpen(null)} title="Close">
              <X size={16} />
            </Button>
          }
          actions={
            <Button variant="outline" size="sm">
              <Upload size={12} /> Import CSV
            </Button>
          }
        />
        <TermbaseBrowser adapter={adapter} locales={LOCALES} />
      </div>
    );
  }
  return (
    <div className="p-6">
      <PageHeader
        title="Termbases"
        actions={
          <div className="flex gap-2">
            <Button variant="outline" size="sm">
              <FolderOpen size={12} /> Open File...
            </Button>
            <Button size="sm">
              <Plus size={12} /> New Termbase
            </Button>
          </div>
        }
      />
      <div className="grid grid-cols-1 gap-2 md:grid-cols-2">
        {TERMBASE_RESOURCES.map((r) => (
          <ResourceCard
            key={r.path}
            name={r.name}
            path={r.path}
            size={r.size}
            modified={r.modified}
            entryCount={r.name === "product-glossary" ? CONCEPTS.length : 24}
            icon={<BookOpen size={18} />}
            onClick={() => setOpen(r.name)}
          />
        ))}
      </div>
    </div>
  );
}

function MemoriesView() {
  const [open, setOpen] = useState<string | null>(null);
  const adapter = useMemo(() => tmAdapter(TM_ENTRIES), []);
  if (open) {
    return (
      <div className="p-6">
        <PageHeader
          title={open}
          subtitle={`~/.config/kapi/tm/${open}.db`}
          backButton={
            <Button variant="ghost" size="icon-xs" onClick={() => setOpen(null)} title="Close">
              <X size={16} />
            </Button>
          }
          actions={
            <div className="flex gap-2">
              <Button variant="outline" size="sm">
                <Upload size={12} /> Import TMX
              </Button>
              <Button variant="outline" size="sm">
                <Download size={12} /> Export TMX
              </Button>
            </div>
          }
        />
        <TMBrowser
          adapter={adapter}
          locales={LOCALES}
          sourceLocale="en-US"
          targetLocales={["fr-FR", "de-DE"]}
        />
      </div>
    );
  }
  return (
    <div className="p-6">
      <PageHeader
        title="Translation Memories"
        actions={
          <div className="flex gap-2">
            <Button variant="outline" size="sm">
              <FolderOpen size={12} /> Open File...
            </Button>
            <Button size="sm">
              <Plus size={12} /> Create TM
            </Button>
          </div>
        }
      />
      <div className="grid grid-cols-1 gap-2 md:grid-cols-2">
        {TM_RESOURCES.map((r) => (
          <ResourceCard
            key={r.path}
            name={r.name}
            path={r.path}
            size={r.size}
            modified={r.modified}
            entryCount={r.name === "acme-app" ? TM_ENTRIES.length : 312}
            icon={<Database size={18} />}
            onClick={() => setOpen(r.name)}
          />
        ))}
      </div>
    </div>
  );
}

// ── App shell (mirrors App.tsx chrome, adhoc mode) ──────────────────────────
export default function DemoApp() {
  const [view, setView] = useState("home");

  return (
    <div className="flex h-screen flex-col bg-background text-foreground">
      <div className="flex min-h-0 flex-1">
        {/* Icon sidebar */}
        <div className="flex shrink-0 flex-col bg-sidebar">
          <div className="h-12 shrink-0" />
          <div className="flex-1 border-r border-border">
            <IconSidebar mode="adhoc" active={view} onChange={setView} />
          </div>
        </div>

        {/* Right: top bar + content */}
        <div className="flex flex-1 flex-col overflow-hidden">
          <div className="flex h-12 shrink-0 items-center border-b border-border bg-sidebar px-4">
            <span className="text-sm font-medium text-muted-foreground">Kapi Desktop</span>
          </div>
          <main className="flex-1 overflow-auto">
            {view === "termbases" ? (
              <TermbasesView />
            ) : view === "memories" ? (
              <MemoriesView />
            ) : (
              <HomeView onGo={setView} />
            )}
          </main>
        </div>
      </div>
    </div>
  );
}
