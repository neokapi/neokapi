// RelationsPanel — the local relations widget (Apache-2.0). This is the new
// "graph": the current concept as a hub plus its DIRECT relations only, grouped
// into reading-ordered lanes. A lane with many neighbours collapses to one
// "N related →" affordance that expands on click; selecting any neighbour
// re-centres the view on it. When the source can edit relations, a relation can
// be added (pick a type + a concept) or removed inline. Deliberately light — no
// global canvas, no physics, no minimap — just a tidy hub-and-lanes layout with
// small nodes.
import { useMemo, useState } from "react";
import {
  Badge,
  Button,
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  cn,
} from "@neokapi/ui-primitives";
import {
  ArrowLeft,
  ArrowRight,
  Check,
  ChevronDown,
  ChevronRight,
  Plus,
  Share2,
  X,
} from "lucide-react";
import type { ConceptDataSource } from "./adapter";
import type { ConceptSectionProps } from "./ConceptView";
import type { ConceptSummary, RelationType } from "./types";
import { RELATION_TYPES } from "./types";
import { primaryName, relationLabel } from "./concept-meta";
import { ConceptSection, EmptyHint, ErrorHint, RelationChip } from "./atoms";
import type { RelationItem } from "./grouping";
import { buildRelationView, neighbourIds } from "./relations-group";
import type { RelationView } from "./relations-group";
import { useResource } from "./useResource";
import { useDebounced } from "./useDebounced";

export interface RelationsPanelProps extends ConceptSectionProps {
  /** Neighbours per lane past which the lane collapses by default. */
  collapseThreshold?: number;
}

export function RelationsPanel({
  concept,
  source,
  capabilities,
  onNavigate,
  collapseThreshold,
}: RelationsPanelProps) {
  const rel = useResource(() => source.getRelations(concept.id), [source, concept.id]);
  const views = useMemo(
    () => buildRelationView(rel.data ?? [], concept.id, collapseThreshold),
    [rel.data, concept.id, collapseThreshold],
  );

  const ids = useMemo(() => neighbourIds(views), [views]);
  const names = useResource(() => resolveNames(source, ids), [source, ids.join(",")]);
  const labelFor = (id: string) => names.data?.[id] ?? id;

  const [expanded, setExpanded] = useState<Set<RelationType>>(new Set());
  const toggle = (type: RelationType) =>
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(type)) next.delete(type);
      else next.add(type);
      return next;
    });

  const [adding, setAdding] = useState(false);
  const [busy, setBusy] = useState(false);

  const canEdit = capabilities.editRelations && typeof source.addRelation === "function";

  const removeRelation = async (relationId: string) => {
    if (!source.removeRelation) return;
    setBusy(true);
    try {
      await source.removeRelation(relationId);
      rel.reload();
    } finally {
      setBusy(false);
    }
  };

  const empty = views.length === 0;

  return (
    <ConceptSection
      title="Relations"
      icon={<Share2 />}
      description="This concept and its direct relations."
      actions={
        canEdit ? (
          <Button variant="outline" size="sm" onClick={() => setAdding(true)} disabled={busy}>
            <Plus />
            Relate
          </Button>
        ) : undefined
      }
    >
      {rel.error ? (
        <ErrorHint title="Could not load relations" description={rel.error.message} />
      ) : empty ? (
        <EmptyHint
          icon={<Share2 />}
          title="No relations yet"
          description="This concept stands on its own."
          action={
            canEdit ? (
              <Button variant="outline" size="sm" onClick={() => setAdding(true)}>
                <Plus />
                Relate a concept
              </Button>
            ) : undefined
          }
        />
      ) : (
        <div className="flex flex-col items-stretch">
          <Hub name={primaryName(concept)} />
          <ul className="space-y-2.5">
            {views.map((view) => (
              <RelationLane
                key={view.type}
                view={view}
                expanded={expanded.has(view.type)}
                onToggle={() => toggle(view.type)}
                labelFor={labelFor}
                onNavigate={onNavigate}
                onRemove={canEdit ? removeRelation : undefined}
                busy={busy}
              />
            ))}
          </ul>
          <p className="mt-3 text-[11px] text-muted-foreground">
            Select a related concept to re-centre the view.
          </p>
        </div>
      )}

      {canEdit && (
        <AddRelationDialog
          open={adding}
          onOpenChange={setAdding}
          source={source}
          subject={concept}
          excludeIds={ids}
          onAdded={() => {
            rel.reload();
            setAdding(false);
          }}
        />
      )}
    </ConceptSection>
  );
}

// ── Hub ──────────────────────────────────────────────────────────────────────

function Hub({ name }: { name: string }) {
  return (
    <div className="mb-3 flex flex-col items-center">
      <div className="inline-flex max-w-full items-center gap-2 rounded-lg border border-primary/30 bg-primary/5 px-3 py-1.5 shadow-sm">
        <span className="size-2 shrink-0 rounded-full bg-primary" aria-hidden />
        <span className="truncate text-sm font-semibold text-foreground">{name}</span>
        <span className="shrink-0 text-[10px] uppercase tracking-wide text-primary/70">here</span>
      </div>
      {/* short connector implying the lanes branch from the hub */}
      <span className="h-3 w-px bg-border" aria-hidden />
    </div>
  );
}

// ── Lane ─────────────────────────────────────────────────────────────────────

function RelationLane({
  view,
  expanded,
  onToggle,
  labelFor,
  onNavigate,
  onRemove,
  busy,
}: {
  view: RelationView;
  expanded: boolean;
  onToggle: () => void;
  labelFor: (id: string) => string;
  onNavigate: (id: string) => void;
  onRemove?: (relationId: string) => void;
  busy: boolean;
}) {
  const collapsed = view.collapsed && !expanded;
  return (
    <li className="flex flex-col gap-1.5 sm:flex-row sm:items-start sm:gap-3">
      <div className="flex shrink-0 items-center gap-1.5 sm:w-32 sm:pt-1">
        <RelationChip type={view.type} />
        <span className="text-[11px] tabular-nums text-muted-foreground">{view.count}</span>
      </div>
      <div className="min-w-0 flex-1">
        {collapsed ? (
          <button
            type="button"
            onClick={onToggle}
            className="inline-flex items-center gap-1.5 rounded-full border border-dashed bg-muted/30 px-3 py-1 text-xs text-foreground transition-colors hover:border-primary/50 hover:bg-primary/5"
          >
            {view.count} related
            <ChevronRight className="size-3.5 text-muted-foreground" />
          </button>
        ) : (
          <div className="flex flex-wrap items-center gap-1.5">
            {view.items.map((item) => (
              <NeighbourChip
                key={item.relation.id}
                item={item}
                label={labelFor(item.otherId)}
                onNavigate={onNavigate}
                onRemove={onRemove}
                busy={busy}
              />
            ))}
            {view.collapsed && (
              <button
                type="button"
                onClick={onToggle}
                className="inline-flex items-center gap-1 text-[11px] text-muted-foreground underline-offset-2 hover:text-foreground hover:underline"
              >
                <ChevronDown className="size-3" />
                Show less
              </button>
            )}
          </div>
        )}
      </div>
    </li>
  );
}

function NeighbourChip({
  item,
  label,
  onNavigate,
  onRemove,
  busy,
}: {
  item: RelationItem;
  label: string;
  onNavigate: (id: string) => void;
  onRemove?: (relationId: string) => void;
  busy: boolean;
}) {
  const Arrow = item.outgoing ? ArrowRight : ArrowLeft;
  return (
    <span
      className={cn(
        "group inline-flex items-center rounded-full border bg-background text-xs transition-colors hover:border-primary/50 hover:bg-primary/5",
        !item.outgoing && "border-dashed",
      )}
    >
      <button
        type="button"
        onClick={() => onNavigate(item.otherId)}
        className="inline-flex items-center gap-1 rounded-full py-1 pl-2.5 pr-2 text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        title={item.outgoing ? "Outgoing relation" : "Incoming relation"}
      >
        <Arrow className="size-3 text-muted-foreground" aria-hidden />
        <span className="max-w-[12rem] truncate">{label}</span>
      </button>
      {onRemove && (
        <button
          type="button"
          onClick={() => onRemove(item.relation.id)}
          disabled={busy}
          aria-label={`Remove relation to ${label}`}
          className="mr-1 grid size-4 place-items-center rounded-full text-muted-foreground opacity-0 transition-opacity hover:bg-destructive/15 hover:text-destructive focus-visible:opacity-100 group-hover:opacity-100 disabled:pointer-events-none"
        >
          <X className="size-3" />
        </button>
      )}
    </span>
  );
}

// ── Add-relation dialog ──────────────────────────────────────────────────────

function AddRelationDialog({
  open,
  onOpenChange,
  source,
  subject,
  excludeIds,
  onAdded,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  source: ConceptDataSource;
  subject: ConceptSummary;
  excludeIds: string[];
  onAdded: () => void;
}) {
  const [type, setType] = useState<RelationType>("RELATED");
  const [target, setTarget] = useState<ConceptSummary | null>(null);
  const [busy, setBusy] = useState(false);

  // Reset transient state each time the dialog opens.
  const reset = () => {
    setType("RELATED");
    setTarget(null);
  };

  const submit = async () => {
    if (!target || !source.addRelation) return;
    setBusy(true);
    try {
      await source.addRelation(subject.id, { targetId: target.id, type });
      onAdded();
    } finally {
      setBusy(false);
      reset();
    }
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) reset();
        onOpenChange(next);
      }}
    >
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Relate “{primaryName(subject)}”</DialogTitle>
          <DialogDescription>
            Choose how this concept relates to another, then pick the concept.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-3">
          <div className="space-y-1.5">
            <Label
              htmlFor="add-relation-type"
              className="text-xs font-medium text-muted-foreground"
            >
              Relation
            </Label>
            <Select value={type} onValueChange={(v) => setType(v as RelationType)}>
              <SelectTrigger id="add-relation-type" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {RELATION_TYPES.map((t) => (
                  <SelectItem key={t} value={t}>
                    {relationLabel(t)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-1.5">
            <Label
              htmlFor="add-relation-concept"
              className="text-xs font-medium text-muted-foreground"
            >
              Concept
            </Label>
            <ConceptPicker
              inputId="add-relation-concept"
              source={source}
              excludeIds={[subject.id, ...excludeIds]}
              selected={target}
              onSelect={setTarget}
            />
          </div>
        </div>

        <DialogFooter>
          <DialogClose asChild>
            <Button variant="ghost" size="sm">
              Cancel
            </Button>
          </DialogClose>
          <Button size="sm" onClick={submit} disabled={!target || busy}>
            Add relation
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function ConceptPicker({
  source,
  excludeIds,
  selected,
  onSelect,
  inputId,
}: {
  source: ConceptDataSource;
  excludeIds: string[];
  selected: ConceptSummary | null;
  onSelect: (concept: ConceptSummary) => void;
  /** id wired to the dialog's "Concept" label for screen-reader association. */
  inputId?: string;
}) {
  const [query, setQuery] = useState("");
  const debounced = useDebounced(query.trim());
  const exclude = useMemo(() => new Set(excludeIds), [excludeIds]);

  const result = useResource(
    () => source.listConcepts({ text: debounced || undefined, limit: 20 }),
    [source, debounced],
  );
  const options = (result.data?.concepts ?? []).filter((c) => !exclude.has(c.id));

  return (
    <Command shouldFilter={false} className="rounded-lg border">
      <CommandInput
        id={inputId}
        aria-label="Concept"
        value={query}
        onValueChange={setQuery}
        placeholder="Search concepts…"
      />
      <CommandList className="max-h-56">
        <CommandEmpty>No concepts found.</CommandEmpty>
        <CommandGroup>
          {options.map((c) => {
            const isSelected = selected?.id === c.id;
            return (
              <CommandItem key={c.id} value={c.id} onSelect={() => onSelect(c)} className="gap-2">
                <Check className={cn("size-4", isSelected ? "opacity-100" : "opacity-0")} />
                <span className="truncate">{primaryName(c)}</span>
                {c.domain && (
                  <Badge variant="outline" className="ml-auto shrink-0 text-[10px] font-normal">
                    {c.domain}
                  </Badge>
                )}
              </CommandItem>
            );
          })}
        </CommandGroup>
      </CommandList>
    </Command>
  );
}

// ── Helpers ──────────────────────────────────────────────────────────────────

async function resolveNames(
  source: ConceptDataSource,
  ids: string[],
): Promise<Record<string, string>> {
  const entries = await Promise.all(
    ids.map(async (id) => {
      const summary = source.getConceptSummary
        ? await source.getConceptSummary(id)
        : await source.getConcept(id);
      return [id, summary ? primaryName(summary) : id] as const;
    }),
  );
  return Object.fromEntries(entries);
}
