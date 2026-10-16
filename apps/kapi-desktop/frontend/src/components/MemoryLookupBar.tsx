// The Find similar mode of the Content memory search: a sentence to look up,
// with the names in it marked so the store can match them as placeholders.
//
// Select a stretch of the sentence and choose what kind of name it is. Enter
// runs the lookup; the matches take the list's place, best first, each with
// its score.

import { useRef, useState } from "react";
import { Search, X } from "lucide-react";
import { t } from "@neokapi/i18n-react/runtime";
import {
  Badge,
  Button,
  ENTITY_TYPES,
  Input,
  type EntityAnnotationDTO,
} from "@neokapi/ui-primitives";

export interface MemoryLookupBarProps {
  /** Run the lookup for this sentence and the names marked in it. */
  onLookup: (text: string, entities: EntityAnnotationDTO[]) => void;
  busy?: boolean;
  /** The sentence the bar opens holding. */
  defaultText?: string;
}

export function MemoryLookupBar({ onLookup, busy, defaultText = "" }: MemoryLookupBarProps) {
  const [text, setText] = useState(defaultText);
  const [entities, setEntities] = useState<EntityAnnotationDTO[]>([]);
  const [selection, setSelection] = useState<{ start: number; end: number } | null>(null);
  const ref = useRef<HTMLInputElement>(null);

  const readSelection = () => {
    const el = ref.current;
    if (!el) return;
    const start = el.selectionStart ?? 0;
    const end = el.selectionEnd ?? 0;
    setSelection(end > start ? { start, end } : null);
  };

  const mark = (type: string) => {
    if (!selection) return;
    const entity = { text: text.slice(selection.start, selection.end), type, ...selection };
    setEntities((prev) =>
      [...prev.filter((e) => e.end <= entity.start || e.start >= entity.end), entity].sort(
        (a, b) => a.start - b.start,
      ),
    );
    setSelection(null);
  };

  return (
    <div data-slot="memory-lookup" className="space-y-1.5">
      <div className="relative">
        <Search
          aria-hidden
          className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground"
        />
        <Input
          ref={ref}
          value={text}
          onChange={(e) => {
            const next = e.target.value;
            setText(next);
            // A marked name the edit changed is no longer marked.
            setEntities((prev) =>
              prev.filter((ent) => next.slice(ent.start, ent.end) === ent.text),
            );
          }}
          onMouseUp={readSelection}
          onKeyUp={readSelection}
          onKeyDown={(e) => {
            if (e.key === "Enter" && text.trim()) {
              e.preventDefault();
              onLookup(text, entities);
            }
          }}
          placeholder={t("A sentence to find similar wording for")}
          aria-label="Find similar wording"
          data-testid="tm-lookup"
          disabled={busy}
          className="h-8 pl-8 text-sm"
        />
      </div>
      {selection && (
        <div data-slot="memory-lookup-mark" className="flex flex-wrap items-center gap-1">
          <span className="text-[11px] text-muted-foreground">
            Mark <span translate="no">{text.slice(selection.start, selection.end)}</span> as
          </span>
          {ENTITY_TYPES.map((type) => (
            <Button
              key={type.value}
              variant="outline"
              size="xs"
              className="h-5 px-1.5 text-[11px]"
              onClick={() => mark(type.value)}
            >
              {/* The entity type's label is translated where it is defined. */}
              <span translate="no">{type.label}</span>
            </Button>
          ))}
        </div>
      )}
      {entities.length > 0 && (
        <div className="flex flex-wrap items-center gap-1">
          {entities.map((entity) => (
            <Badge
              key={`${entity.start}:${entity.end}`}
              variant="outline"
              className="h-5 gap-1 pr-0.5 text-[11px]"
            >
              <span className="text-muted-foreground" translate="no">
                {ENTITY_TYPES.find((type) => type.value === entity.type)?.label ?? entity.type}
              </span>
              <span translate="no">{entity.text}</span>
              <button
                type="button"
                onClick={() => setEntities((prev) => prev.filter((e) => e !== entity))}
                aria-label={t("Unmark {name}", { name: entity.text })}
                className="rounded text-muted-foreground hover:text-foreground"
              >
                <X className="size-3" />
              </button>
            </Badge>
          ))}
        </div>
      )}
    </div>
  );
}
