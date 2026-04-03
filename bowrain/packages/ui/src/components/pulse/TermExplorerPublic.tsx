import { Search } from "lucide-react";
import { useState } from "react";

interface TermEntry {
  id: string;
  term: string;
  definition: string;
  domain?: string;
  locale: string;
  translations?: Record<string, string>;
}

interface TermExplorerPublicProps {
  terms: TermEntry[];
  className?: string;
}

export function TermExplorerPublic({ terms, className }: TermExplorerPublicProps) {
  const [search, setSearch] = useState("");

  const filtered = terms.filter(
    (t) =>
      t.term.toLowerCase().includes(search.toLowerCase()) ||
      t.definition.toLowerCase().includes(search.toLowerCase()),
  );

  return (
    <div className={className}>
      <div className="relative mb-4">
        <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
        <input
          type="text"
          placeholder="Search terminology..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="w-full rounded-lg border bg-background py-2 pl-10 pr-4 text-sm"
        />
      </div>

      {filtered.length === 0 ? (
        <div className="rounded-lg border bg-card p-8 text-center text-muted-foreground">
          {terms.length === 0 ? "No terminology published yet." : "No terms match your search."}
        </div>
      ) : (
        <div className="space-y-2">
          {filtered.map((t) => (
            <div key={t.id} className="rounded-lg border bg-card p-4">
              <div className="flex items-start justify-between">
                <div>
                  <span className="font-medium">{t.term}</span>
                  {t.domain && (
                    <span className="ml-2 rounded bg-muted px-1.5 py-0.5 text-xs">{t.domain}</span>
                  )}
                </div>
                <span className="text-xs text-muted-foreground">{t.locale}</span>
              </div>
              <p className="mt-1 text-sm text-muted-foreground">{t.definition}</p>
              {t.translations && Object.keys(t.translations).length > 0 && (
                <div className="mt-2 flex flex-wrap gap-2">
                  {Object.entries(t.translations).map(([loc, text]) => (
                    <span key={loc} className="rounded bg-muted px-2 py-0.5 text-xs">
                      <strong>{loc}:</strong> {text}
                    </span>
                  ))}
                </div>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
