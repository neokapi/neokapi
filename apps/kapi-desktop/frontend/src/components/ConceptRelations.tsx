// A concept's direct relations in the Terms detail pane, one line each, and the
// dialog that relates it to another concept.
//
// Each line names the kind of relation and the concept at its other end, with
// an arrow for its direction: out from the selected concept, or in from the
// other one. Selecting the other concept opens it in the pane. Relate, in the
// pane's toolbar, opens the dialog; the cross on a line removes that relation.

import { useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, ArrowRight, Plus, Search, X } from "lucide-react";
import { t } from "@neokapi/i18n-react/runtime";
import {
  RELATION_TYPES,
  buildRelationView,
  primaryName,
  type ConceptDataSource,
  type ConceptNaming,
  type ConceptSummary,
  type RelationType,
} from "@neokapi/concept-ui";
import {
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Input,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  SimpleTooltip,
  cn,
  useDebounced,
} from "@neokapi/ui-primitives";

/** A relation kind, read from the selected concept, in the active UI language. */
export function relationWord(type: RelationType): string {
  switch (type) {
    case "BROADER":
      return t("broader than", "concept relation");
    case "NARROWER":
      return t("narrower than", "concept relation");
    case "PART_OF":
      return t("part of", "concept relation");
    case "HAS_PART":
      return t("has part", "concept relation");
    case "RELATED":
      return t("related to", "concept relation");
    case "REPLACED_BY":
      return t("replaced by", "concept relation");
    case "USE_INSTEAD":
      return t("use instead", "concept relation");
    case "EXACT_MATCH":
      return t("exact match", "concept relation");
    case "CLOSE_MATCH":
      return t("close match", "concept relation");
    case "COMPETITOR":
      return t("competitor", "concept relation");
    default:
      return type;
  }
}

/** The query keys a concept's relations and their names are read under. */
export const relationsKey = (handle: string, id: string) => ["terms", "relations", handle, id];

export interface ConceptRelationsProps {
  handle: string;
  concept: ConceptSummary;
  source: ConceptDataSource;
  naming: ConceptNaming;
  /** Open another concept in the pane. */
  onNavigate: (id: string) => void;
  /** Removing is offered; the source can edit relations. */
  canEdit: boolean;
}

/** The selected concept's relations, one line each. Nothing is drawn when it has none. */
export function ConceptRelations({
  handle,
  concept,
  source,
  naming,
  onNavigate,
  canEdit,
}: ConceptRelationsProps) {
  const qc = useQueryClient();
  const relations = useQuery({
    queryKey: relationsKey(handle, concept.id),
    queryFn: () => Promise.resolve(source.getRelations(concept.id)),
  });
  const views = useMemo(
    () => buildRelationView(relations.data ?? [], concept.id, Number.POSITIVE_INFINITY),
    [relations.data, concept.id],
  );
  const ids = useMemo(
    () => [...new Set(views.flatMap((v) => v.items.map((item) => item.otherId)))],
    [views],
  );
  const names = useQuery({
    queryKey: ["terms", "names", handle, ids],
    enabled: ids.length > 0,
    queryFn: async () => {
      const entries = await Promise.all(
        ids.map(async (id) => {
          const other = await Promise.resolve(
            source.getConceptSummary ? source.getConceptSummary(id) : source.getConcept(id),
          );
          return [id, other ? primaryName(other, naming) : id] as const;
        }),
      );
      return Object.fromEntries(entries) as Record<string, string>;
    },
  });
  const [removing, setRemoving] = useState<string | null>(null);

  const remove = async (relationId: string) => {
    if (!source.removeRelation) return;
    setRemoving(relationId);
    try {
      await Promise.resolve(source.removeRelation(relationId));
    } finally {
      setRemoving(null);
      void qc.invalidateQueries({ queryKey: relationsKey(handle, concept.id) });
    }
  };

  if (views.length === 0) return null;

  return (
    <section data-slot="terms-relations" aria-label={t("Relations")}>
      <h3 className="mb-1.5 text-[11px] font-medium text-muted-foreground">Relations</h3>
      <ul className="space-y-1">
        {views.flatMap((view) =>
          view.items.map((item) => {
            const name = names.data?.[item.otherId] ?? item.otherId;
            const Arrow = item.outgoing ? ArrowRight : ArrowLeft;
            return (
              <li
                key={item.relation.id}
                data-slot="terms-relation"
                className="group/relation flex min-w-0 items-center gap-2 text-sm"
              >
                <SimpleTooltip
                  content={item.outgoing ? t("From this concept") : t("To this concept")}
                >
                  <span className="inline-flex text-muted-foreground">
                    <Arrow aria-hidden className="size-3.5" />
                  </span>
                </SimpleTooltip>
                <span className="shrink-0 text-muted-foreground">{relationWord(view.type)}</span>
                <button
                  type="button"
                  onClick={() => onNavigate(item.otherId)}
                  className="min-w-0 truncate rounded text-left underline-offset-2 hover:underline focus-visible:ring-2 focus-visible:ring-ring/50 focus-visible:outline-none"
                  translate="no"
                >
                  {name}
                </button>
                {item.relation.note && (
                  <span className="min-w-0 truncate text-xs text-muted-foreground" translate="no">
                    {item.relation.note}
                  </span>
                )}
                {canEdit && (
                  <Button
                    variant="ghost"
                    size="icon-xs"
                    className="ml-auto text-muted-foreground opacity-0 group-hover/relation:opacity-100 focus-visible:opacity-100"
                    disabled={removing === item.relation.id}
                    onClick={() => void remove(item.relation.id)}
                    aria-label={t("Remove relation to {name}", { name })}
                  >
                    <X />
                  </Button>
                )}
              </li>
            );
          }),
        )}
      </ul>
    </section>
  );
}

export interface RelateDialogProps {
  handle: string;
  concept: ConceptSummary;
  source: ConceptDataSource;
  naming: ConceptNaming;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

/** Relate the selected concept to another: the kind of relation, then the concept. */
export function RelateDialog({
  handle,
  concept,
  source,
  naming,
  open,
  onOpenChange,
}: RelateDialogProps) {
  const qc = useQueryClient();
  const [type, setType] = useState<RelationType>("RELATED");
  const [text, setText] = useState("");
  const [target, setTarget] = useState<ConceptSummary | null>(null);
  const [busy, setBusy] = useState(false);
  const query = useDebounced(text.trim());
  const found = useQuery({
    queryKey: ["terms", "relate-search", handle, query],
    enabled: open,
    queryFn: () => Promise.resolve(source.listConcepts({ text: query || undefined, limit: 8 })),
  });
  const candidates = (found.data?.concepts ?? []).filter((c) => c.id !== concept.id);
  const name = primaryName(concept, naming);

  const close = () => {
    setType("RELATED");
    setText("");
    setTarget(null);
    onOpenChange(false);
  };

  const submit = async () => {
    if (!target || !source.addRelation) return;
    setBusy(true);
    try {
      await Promise.resolve(source.addRelation(concept.id, { targetId: target.id, type }));
      void qc.invalidateQueries({ queryKey: relationsKey(handle, concept.id) });
      close();
    } finally {
      setBusy(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={(next) => (next ? onOpenChange(true) : close())}>
      <DialogContent className="sm:max-w-md" data-slot="relate-dialog">
        <DialogHeader>
          <DialogTitle>
            Relate <span translate="no">{name}</span>
          </DialogTitle>
          <DialogDescription>
            Choose the kind of relation, then the other concept.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <div className="space-y-1.5">
            <Label htmlFor="relate-type" className="text-xs text-muted-foreground">
              Relation
            </Label>
            <Select value={type} onValueChange={(v) => setType(v as RelationType)}>
              <SelectTrigger id="relate-type" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {RELATION_TYPES.map((kind) => (
                  <SelectItem key={kind} value={kind}>
                    {relationWord(kind)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="relate-concept" className="text-xs text-muted-foreground">
              Concept
            </Label>
            <div className="relative">
              <Search
                aria-hidden
                className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground"
              />
              <Input
                id="relate-concept"
                type="search"
                value={text}
                onChange={(e) => setText(e.target.value)}
                placeholder={t("Search terms")}
                className="h-8 pl-8"
              />
            </div>
            <ul role="listbox" aria-label={t("Concepts")} className="max-h-48 overflow-y-auto">
              {candidates.map((c) => (
                <li
                  key={c.id}
                  role="option"
                  aria-selected={target?.id === c.id}
                  tabIndex={0}
                  onClick={() => setTarget(c)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" || e.key === " ") {
                      e.preventDefault();
                      setTarget(c);
                    }
                  }}
                  className={cn(
                    "cursor-default truncate rounded-md px-2 py-1.5 text-sm",
                    target?.id === c.id ? "bg-primary/10 ring-1 ring-primary" : "hover:bg-muted",
                  )}
                  translate="no"
                >
                  {primaryName(c, naming)}
                </li>
              ))}
              {found.data && candidates.length === 0 && (
                <li className="px-2 py-1.5 text-xs text-muted-foreground">
                  No other concept matches.
                </li>
              )}
            </ul>
          </div>
        </div>
        <DialogFooter>
          <Button variant="ghost" size="sm" onClick={close}>
            Cancel
          </Button>
          <Button
            size="sm"
            data-slot="relate-confirm"
            onClick={() => void submit()}
            disabled={!target || busy}
          >
            <Plus size={13} />
            Add relation
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
