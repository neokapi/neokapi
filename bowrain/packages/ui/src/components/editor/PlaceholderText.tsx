import type { ReactNode } from "react";

const PLACEHOLDER = /<x id="([^"]*)"\/>/g;

/**
 * Text in the placeholder form a read shows, each inline code drawn as a chip
 * naming its id, so a link or a variable reads as a code and not as markup.
 */
export function PlaceholderText({ text }: { text: string }) {
  if (text === "") {
    return <span className="text-muted-foreground italic">No translation</span>;
  }
  const parts: ReactNode[] = [];
  let last = 0;
  for (const match of text.matchAll(PLACEHOLDER)) {
    const at = match.index ?? 0;
    if (at > last) parts.push(text.slice(last, at));
    parts.push(
      <span
        key={`${at}-${match[1]}`}
        className="mx-0.5 inline-block rounded bg-muted px-1 font-mono text-[11px] text-muted-foreground"
      >
        {match[1]}
      </span>,
    );
    last = at + match[0].length;
  }
  if (last < text.length) parts.push(text.slice(last));
  return <>{parts}</>;
}
