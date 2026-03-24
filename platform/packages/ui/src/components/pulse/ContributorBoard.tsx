interface Contributor {
  name: string;
  avatar_url?: string;
  translations: number;
  reviews: number;
  languages: string[];
}

interface ContributorBoardProps {
  contributors: Contributor[];
  className?: string;
}

export function ContributorBoard({ contributors, className }: ContributorBoardProps) {
  if (contributors.length === 0) {
    return (
      <div className={`rounded-lg border bg-card p-8 text-center text-muted-foreground ${className ?? ""}`}>
        No contributors yet.
      </div>
    );
  }

  return (
    <div className={`space-y-2 ${className ?? ""}`}>
      {contributors.map((c, i) => (
        <div key={c.name} className="flex items-center gap-3 rounded-lg border bg-card p-3">
          <span className="flex h-6 w-6 items-center justify-center rounded-full bg-muted text-xs font-bold">
            {i + 1}
          </span>
          {c.avatar_url ? (
            <img src={c.avatar_url} alt={c.name} className="h-8 w-8 rounded-full" />
          ) : (
            <div className="flex h-8 w-8 items-center justify-center rounded-full bg-primary/10 text-sm font-medium">
              {c.name.charAt(0).toUpperCase()}
            </div>
          )}
          <div className="min-w-0 flex-1">
            <div className="text-sm font-medium">{c.name}</div>
            <div className="text-xs text-muted-foreground">
              {c.translations} translations · {c.reviews} reviews
            </div>
          </div>
          {c.languages.length > 0 && (
            <div className="flex gap-1">
              {c.languages.slice(0, 3).map((l) => (
                <span key={l} className="rounded bg-muted px-1.5 py-0.5 text-xs">{l}</span>
              ))}
            </div>
          )}
        </div>
      ))}
    </div>
  );
}
