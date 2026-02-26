import type { BlockTermMatch } from "../../types/api";

/** Highlights matched terminology in source text with underline styling. */
export function HighlightedSource({ text, termMatches }: { text: string; termMatches: BlockTermMatch[] }) {
  if (termMatches.length === 0) return <>{text}</>;

  const sorted = [...termMatches]
    .filter(m => m.start >= 0 && m.end > m.start && m.end <= text.length)
    .sort((a, b) => a.start - b.start);

  const parts: React.ReactNode[] = [];
  let lastEnd = 0;

  for (const m of sorted) {
    if (m.start < lastEnd) continue;
    if (m.start > lastEnd) {
      parts.push(<span key={`t-${lastEnd}`}>{text.slice(lastEnd, m.start)}</span>);
    }
    parts.push(
      <span
        key={`h-${m.start}`}
        className="underline decoration-dotted decoration-orange-600 underline-offset-2 cursor-help"
        title={`${m.source_term} \u2192 ${m.target_terms?.join(", ") || "?"} (${m.status})`}
      >
        {text.slice(m.start, m.end)}
      </span>,
    );
    lastEnd = m.end;
  }
  if (lastEnd < text.length) {
    parts.push(<span key={`t-${lastEnd}`}>{text.slice(lastEnd)}</span>);
  }
  return <>{parts}</>;
}
