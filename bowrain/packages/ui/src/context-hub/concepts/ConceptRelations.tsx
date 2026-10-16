// A concept's direct relations in the Concepts detail pane, one line each, and
// the dialog that relates it to another concept: the platform's twin of the
// relations on Kapi Desktop's Terms page.
//
// Each line names the kind of relation and the concept at its other end, with
// an arrow for its direction: out from the selected concept, or in from the
// other one, and the window it holds in when it carries one. Selecting the
// other concept opens it in the pane. Relate, in the pane's toolbar, opens the
// dialog; the cross on a line removes that relation. A replaced-by relation is
// governed, so adding one raises the change-set prompt instead of landing.
import { useMemo, useState } from "react";
import { useQueries, useQuery } from "@tanstack/react-query";
import { ArrowLeft, ArrowRight, Plus, Search, X } from "lucide-react";
import { t } from "@neokapi/i18n-react/runtime";
import {
  RELATION_TYPES,
  buildRelationView,
  primaryName,
  type ConceptNaming,
  type ConceptSummary,
  type RelationType,
} from "@neokapi/concept-ui";
import {
  Badge,
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
  When,
  cn,
  useDebounced,
} from "@neokapi/ui-primitives";
import { useApi } from "../../context/ApiContext";
import { useWorkspace } from "../../context/WorkspaceContext";
import {
  useAddConceptRelation,
  useConceptRelations,
  useDeleteConceptRelation,
} from "../../hooks/useConceptsApi";
import { ErrorNotice } from "../../errors";
import {
  asGovernedEditError,
  conceptSummaryFromInfo,
  relationFromDTO,
  type GovernedEditError,
} from "./restConceptSource";
import { relationWord } from "./TermStatusChip";

export interface ConceptRelationsProps {
  concept: ConceptSummary;
  naming: ConceptNaming;
  /** Open another concept in the pane. */
  onNavigate: (id: string) => void;
  /** Removing is offered. */
  canEdit: boolean;
}

/** The selected concept's relations, one line each. Nothing is drawn when it has none. */
export function ConceptRelations({ concept, naming, onNavigate, canEdit }: ConceptRelationsProps) {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";
  const relations = useConceptRelations(concept.id);
  const remove = useDeleteConceptRelation(concept.id);
  const views = useMemo(
    () =>
      buildRelationView(
        (relations.data ?? []).map(relationFromDTO),
        concept.id,
        Number.POSITIVE_INFINITY,
      ),
    [relations.data, concept.id],
  );
  const ids = useMemo(
    () => [...new Set(views.flatMap((v) => v.items.map((item) => item.otherId)))],
    [views],
  );
  // Each neighbour is read under the key the pane reads a concept under, so
  // following a relation opens on a concept that is already loaded.
  const neighbours = useQueries({
    queries: ids.map((id) => ({
      queryKey: ["concept", ws, id],
      queryFn: () => api.getConcept(ws, id),
      enabled: !!ws,
      staleTime: 15_000,
    })),
  });
  const names = useMemo(() => {
    const out: Record<string, string> = {};
    neighbours.forEach((q, i) => {
      if (q.data) out[ids[i]] = primaryName(conceptSummaryFromInfo(q.data), naming);
    });
    return out;
  }, [neighbours, ids, naming]);

  if (views.length === 0) return null;

  return (
    <section data-slot="concept-relations" aria-label={t("Relations")}>
      <h3 className="mb-1.5 text-[11px] font-medium text-muted-foreground">Relations</h3>
      {remove.error ? (
        <ErrorNotice
          className="mb-2"
          variant="inline"
          error={remove.error}
          title={t("The relation was not removed")}
        />
      ) : null}
      <ul className="space-y-1">
        {views.flatMap((view) =>
          view.items.map((item) => {
            const name = names[item.otherId] ?? item.otherId;
            const Arrow = item.outgoing ? ArrowRight : ArrowLeft;
            const validity = item.relation.validity;
            return (
              <li
                key={item.relation.id}
                data-slot="concept-relation"
                data-relation={view.type}
                className="group/relation flex min-w-0 flex-wrap items-center gap-x-2 gap-y-0.5 text-sm"
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
                  data-slot="concept-relation-target"
                  onClick={() => onNavigate(item.otherId)}
                  className="min-w-0 truncate rounded text-left underline-offset-2 hover:underline focus-visible:ring-2 focus-visible:ring-ring/50 focus-visible:outline-none"
                  translate="no"
                >
                  {name}
                </button>
                {validity?.validFrom && (
                  <span className="text-[11px] text-muted-foreground">
                    from <When iso={validity.validFrom} timeStyle="none" />
                  </span>
                )}
                {validity?.validTo && (
                  <span className="text-[11px] text-muted-foreground">
                    until <When iso={validity.validTo} timeStyle="none" />
                  </span>
                )}
                {Object.entries(validity?.tags ?? {}).map(([key, value]) => (
                  <Badge
                    key={key}
                    variant="outline"
                    className="h-4 px-1.5 text-[11px]"
                    translate="no"
                  >
                    {key}: {value}
                  </Badge>
                ))}
                {item.relation.note && (
                  <span className="min-w-0 truncate text-xs text-muted-foreground" translate="no">
                    {item.relation.note}
                  </span>
                )}
                {canEdit && (
                  <Button
                    variant="ghost"
                    size="icon-xs"
                    data-slot="concept-relation-remove"
                    className="ml-auto text-muted-foreground opacity-0 group-hover/relation:opacity-100 focus-visible:opacity-100"
                    disabled={remove.isPending && remove.variables === item.relation.id}
                    onClick={() => remove.mutate(item.relation.id)}
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
  concept: ConceptSummary;
  naming: ConceptNaming;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** The server refused the relation as governed: hand the refusal to the page's prompt. */
  onGoverned: (error: GovernedEditError) => void;
}

/** Relate the selected concept to another: the kind of relation, then the concept. */
export function RelateDialog({
  concept,
  naming,
  open,
  onOpenChange,
  onGoverned,
}: RelateDialogProps) {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";
  const add = useAddConceptRelation(concept.id);
  const [type, setType] = useState<RelationType>("RELATED");
  const [text, setText] = useState("");
  const [target, setTarget] = useState<ConceptSummary | null>(null);
  const query = useDebounced(text.trim());
  const found = useQuery({
    queryKey: ["concepts", ws, "relate", query],
    enabled: open && !!ws,
    queryFn: () => api.listConcepts(ws, { q: query || undefined, limit: 8 }),
  });
  const candidates = (found.data?.concepts ?? [])
    .map(conceptSummaryFromInfo)
    .filter((c) => c.id !== concept.id);
  const name = primaryName(concept, naming);

  const close = () => {
    setType("RELATED");
    setText("");
    setTarget(null);
    add.reset();
    onOpenChange(false);
  };

  const submit = () => {
    if (!target) return;
    add.mutate(
      { target_id: target.id, relation_type: type },
      {
        onSuccess: close,
        onError: (error) => {
          const governed = asGovernedEditError(error, concept.id);
          if (governed) {
            close();
            onGoverned(governed);
          }
        },
      },
    );
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
                placeholder={t("Search concepts")}
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
          {add.error && !asGovernedEditError(add.error, concept.id) ? (
            <ErrorNotice
              variant="inline"
              error={add.error}
              title={t("The relation was not added")}
            />
          ) : null}
        </div>
        <DialogFooter>
          <Button variant="ghost" size="sm" onClick={close}>
            Cancel
          </Button>
          <Button
            size="sm"
            data-slot="relate-confirm"
            onClick={submit}
            disabled={!target || add.isPending}
          >
            <Plus size={13} />
            Add relation
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
