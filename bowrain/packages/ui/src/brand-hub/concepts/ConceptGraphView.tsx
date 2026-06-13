// The interactive concept-graph experience (AD-021): the React Flow canvas
// (GraphPanel) framed with the controls a steward navigates by — search/focus a
// concept, scope the graph to a point in time (as-of) and a market, narrow to a
// concept's neighbourhood, a legend, and a side panel that summarises the
// selected concept and links to its story. Rendered as the graph mode of the
// Concepts section, so it owns no BrandHub frame of its own.
import { useMemo, useState } from "react";
import {
  Alert,
  AlertDescription,
  AlertTitle,
  Badge,
  Button,
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  Input,
  Popover,
  PopoverContent,
  PopoverTrigger,
  ScrollArea,
  Separator,
  Skeleton,
  Switch,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  cn,
} from "@neokapi/ui-primitives";
import { Calendar, Crosshair, RotateCcw } from "lucide-react";
import {
  Search,
  X,
  Network,
  ScrollText,
  ArrowRight,
  ArrowLeft,
  Info,
  AlertTriangle,
} from "../../components/icons";
import type { GraphViz, GraphVizNode, GraphParams, TermStatus } from "../../types/brand-graph";
import { useGraph } from "../../hooks/useGraphApi";
import { useMarkets } from "../../hooks/useMarketsApi";
import { useConcept } from "../../hooks/useConceptsApi";
import { TermStatusBadge, RelationBadge, EmptyState } from "../shell/atoms";
import { GraphPanel } from "./GraphPanel";
import {
  relationEdgeStyle,
  statusColorVar,
  relationNeighbours,
  type RelationFamily,
} from "./graph-style";
import { shouldGuardGraph } from "./graph-guard";

const ALL = "all";

export interface ConceptGraphViewProps {
  /** Open a concept's story page. */
  onOpenConcept: (conceptId: string) => void;
  className?: string;
}

export function ConceptGraphView({ onOpenConcept, className }: ConceptGraphViewProps) {
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [asOf, setAsOf] = useState("");
  const [market, setMarket] = useState<string>(ALL);
  const [neighbourhoodOnly, setNeighbourhoodOnly] = useState(false);

  const params = useMemo<GraphParams>(() => {
    const p: GraphParams = {};
    if (asOf) p.as_of = `${asOf}T00:00:00Z`;
    if (market !== ALL) p.market = market;
    if (neighbourhoodOnly && selectedId) {
      p.focus = selectedId;
      p.depth = 1;
    }
    return p;
  }, [asOf, market, neighbourhoodOnly, selectedId]);

  const { data: graph, isLoading, isError, error, refetch } = useGraph(params);
  const { data: markets } = useMarkets();

  const nodeCount = graph?.nodes.length ?? 0;
  const edgeCount = graph?.edges.length ?? 0;
  const hasScopeFilter = asOf !== "" || market !== ALL;
  const hasFilters = hasScopeFilter || neighbourhoodOnly;

  // The graph is the one navigator surface that cannot paginate, so when the
  // wide-open view would draw more concepts than read legibly (the server caps
  // and flags the payload), we show a focus-or-filter guard instead of a
  // hairball. A focus or a scope filter narrows the view, so it renders normally.
  //
  // The guard gates on params.focus — what actually scopes the server query —
  // not on selectedId, which only opens the side panel. Selecting a concept (via
  // the search combobox, or after resetting the neighbourhood) leaves the graph
  // wide-open, so it must keep guarding until a neighbourhood focus or a scope
  // filter narrows it.
  const guarded =
    !!graph &&
    nodeCount > 0 &&
    shouldGuardGraph({
      truncated: graph.truncated,
      nodeCount,
      hasFocus: !!params.focus,
      hasFilter: hasScopeFilter,
    });

  // Focusing a concept from the guard scopes the canvas to its neighbourhood, so
  // the steward lands on a readable, relevant view rather than an arbitrary slice.
  const focusConcept = (id: string) => {
    setSelectedId(id);
    setNeighbourhoodOnly(true);
  };

  return (
    <div className={cn("space-y-3", className)}>
      <div className="flex flex-wrap items-center gap-2">
        <ConceptSearch graph={graph} value={selectedId} onSelect={setSelectedId} />

        <div className="relative">
          <Calendar className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
          <Input
            type="date"
            value={asOf}
            onChange={(e) => setAsOf(e.target.value)}
            aria-label="As of date"
            className="w-[10.5rem] pl-8"
          />
          {asOf && (
            <button
              type="button"
              onClick={() => setAsOf("")}
              aria-label="Clear as-of date"
              className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
            >
              <X className="size-3.5" />
            </button>
          )}
        </div>

        {markets && markets.length > 0 && (
          <Select value={market} onValueChange={setMarket}>
            <SelectTrigger className="w-36" size="sm">
              <SelectValue placeholder="Market" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ALL}>All markets</SelectItem>
              {markets.map((m) => (
                <SelectItem key={m.id} value={m.name}>
                  {m.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}

        <label
          className={cn(
            "flex items-center gap-2 rounded-md border bg-card px-2.5 py-1.5 text-xs",
            !selectedId && "opacity-60",
          )}
          title={
            selectedId
              ? "Show only the selected concept and its direct neighbours"
              : "Select a concept first to scope the graph to its neighbourhood"
          }
        >
          <Crosshair className="size-3.5 text-muted-foreground" />
          <span className="text-muted-foreground">Neighbourhood</span>
          <Switch
            checked={neighbourhoodOnly}
            disabled={!selectedId}
            onCheckedChange={setNeighbourhoodOnly}
            aria-label="Neighbourhood only"
          />
        </label>

        <div className="ml-auto flex items-center gap-2">
          {hasFilters && (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                setAsOf("");
                setMarket(ALL);
                setNeighbourhoodOnly(false);
                setSelectedId(null);
              }}
            >
              Reset
            </Button>
          )}
          <GraphLegend />
        </div>
      </div>

      <div className="relative h-[560px] overflow-hidden rounded-lg border bg-card">
        {isLoading ? (
          <div className="flex h-full items-center justify-center p-6">
            <Skeleton className="h-full w-full" />
          </div>
        ) : isError ? (
          <div className="flex h-full items-center justify-center p-6">
            <Alert variant="destructive" className="max-w-md">
              <AlertTriangle />
              <AlertTitle>Couldn't load the graph</AlertTitle>
              <AlertDescription className="space-y-3">
                <p>
                  {error instanceof Error
                    ? error.message
                    : "The brand graph could not be fetched. Check your connection and try again."}
                </p>
                <Button size="sm" variant="outline" onClick={() => void refetch()}>
                  <RotateCcw />
                  Retry
                </Button>
              </AlertDescription>
            </Alert>
          </div>
        ) : !graph || nodeCount === 0 ? (
          <EmptyState
            className="h-full border-0 bg-transparent"
            icon={<Network />}
            title={hasFilters ? "No concepts in this scope" : "No graph to show"}
            description={
              hasFilters
                ? "No concepts or relations match the current time, market, or neighbourhood. Try widening the scope."
                : "Add concepts and relations to see the brand graph take shape."
            }
          />
        ) : guarded ? (
          <GraphScaleGuard
            graph={graph}
            shown={nodeCount}
            total={graph.total}
            onFocus={focusConcept}
          />
        ) : (
          <>
            <GraphPanel
              graph={graph}
              focusId={selectedId ?? undefined}
              onSelectNode={setSelectedId}
            />
            <div className="pointer-events-none absolute left-3 top-3 rounded-md border bg-card/90 px-2 py-1 text-[11px] text-muted-foreground backdrop-blur">
              {nodeCount} concept{nodeCount === 1 ? "" : "s"} · {edgeCount} relation
              {edgeCount === 1 ? "" : "s"}
            </div>
            {selectedId && graph && (
              <SelectedConceptPanel
                graph={graph}
                conceptId={selectedId}
                onClose={() => setSelectedId(null)}
                onFocus={setSelectedId}
                onOpenConcept={onOpenConcept}
              />
            )}
          </>
        )}
      </div>
    </div>
  );
}

// ── Search / focus combobox ──────────────────────────────────────────────────

function ConceptSearch({
  graph,
  value,
  onSelect,
}: {
  graph?: GraphViz;
  value: string | null;
  onSelect: (id: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const nodes = graph?.nodes ?? [];
  const selected = nodes.find((n) => n.id === value);

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          size="sm"
          disabled={nodes.length === 0}
          className="min-w-56 justify-start gap-2 font-normal"
          aria-label="Find and focus a concept"
        >
          <Search className="size-4 text-muted-foreground" />
          <span className={cn("truncate", !selected && "text-muted-foreground")}>
            {selected ? selected.label : "Find a concept…"}
          </span>
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-72 p-0" align="start">
        <Command>
          <CommandInput placeholder="Search concepts…" />
          <CommandList>
            <CommandEmpty>No concepts found.</CommandEmpty>
            <CommandGroup>
              {nodes.map((n) => (
                <CommandItem
                  key={n.id}
                  value={`${n.label} ${n.domain ?? ""} ${n.id}`}
                  onSelect={() => {
                    onSelect(n.id);
                    setOpen(false);
                  }}
                  className="gap-2"
                >
                  <span
                    aria-hidden
                    className="size-2.5 shrink-0 rounded-full"
                    style={{ backgroundColor: statusColorVar(n.status) }}
                  />
                  <span className="truncate">{n.label}</span>
                  {n.domain && (
                    <span className="ml-auto truncate text-xs text-muted-foreground">
                      {n.domain}
                    </span>
                  )}
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}

// ── Scale guard ──────────────────────────────────────────────────────────────

// Shown in place of the canvas when the wide-open graph would be a hairball: a
// calm, helpful state (not an error) that names the scale and keeps the focus
// affordance front and centre. Picking a concept here scopes the canvas to its
// neighbourhood, so the steward moves straight into a readable view.
function GraphScaleGuard({
  graph,
  shown,
  total,
  onFocus,
}: {
  graph: GraphViz;
  shown: number;
  total: number;
  onFocus: (id: string) => void;
}) {
  return (
    <div className="flex h-full flex-col items-center justify-center gap-4 p-8 text-center">
      <div className="flex size-12 items-center justify-center rounded-full bg-muted text-muted-foreground [&_svg]:size-6">
        <Network />
      </div>
      <div className="space-y-1.5">
        <h3 className="text-sm font-medium text-foreground">Too many concepts to graph at once</h3>
        <p className="mx-auto max-w-md text-sm text-muted-foreground">
          Showing {shown.toLocaleString()} of {total.toLocaleString()} concepts. Focus on a concept,
          or narrow by market or date, to explore the graph without losing the thread.
        </p>
      </div>
      <ConceptSearch graph={graph} value={null} onSelect={onFocus} />
      <p className="text-xs text-muted-foreground">
        The graph stays readable when it is scoped to what you are working on.
      </p>
    </div>
  );
}

// ── Legend ───────────────────────────────────────────────────────────────────

const STATUS_LEGEND: { label: string; status: GraphVizNode["status"] }[] = [
  { label: "Preferred", status: "preferred" },
  { label: "Approved", status: "approved" },
  { label: "Admitted", status: "admitted" },
  { label: "Proposed / deprecated", status: "proposed" },
  { label: "Forbidden", status: "forbidden" },
];

const RELATION_LEGEND: { label: string; sample: RelationFamily }[] = [
  { label: "Hierarchy", sample: "hierarchy" },
  { label: "Replaced by", sample: "succession" },
  { label: "Use instead", sample: "guidance" },
  { label: "Equivalent", sample: "equivalence" },
  { label: "Competitor", sample: "competitor" },
  { label: "Related", sample: "related" },
];

const FAMILY_SAMPLE_TYPE: Record<RelationFamily, Parameters<typeof relationEdgeStyle>[0]> = {
  hierarchy: "BROADER",
  succession: "REPLACED_BY",
  guidance: "USE_INSTEAD",
  equivalence: "EXACT_MATCH",
  competitor: "COMPETITOR",
  related: "RELATED",
};

function GraphLegend() {
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button variant="outline" size="sm" className="gap-1.5">
          <Info className="size-3.5" />
          Legend
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-72 space-y-3">
        <div className="space-y-2">
          <p className="text-xs font-medium text-muted-foreground">Concept status</p>
          <ul className="grid grid-cols-1 gap-1.5">
            {STATUS_LEGEND.map((s) => (
              <li key={s.label} className="flex items-center gap-2 text-sm">
                <span
                  aria-hidden
                  className="size-3 rounded-full"
                  style={{ backgroundColor: statusColorVar(s.status) }}
                />
                {s.label}
              </li>
            ))}
          </ul>
        </div>
        <Separator />
        <div className="space-y-2">
          <p className="text-xs font-medium text-muted-foreground">Relations</p>
          <ul className="grid grid-cols-1 gap-1.5">
            {RELATION_LEGEND.map((r) => {
              const style = relationEdgeStyle(FAMILY_SAMPLE_TYPE[r.sample]);
              const markerId = `legend-arrow-${r.sample}`;
              return (
                <li key={r.label} className="flex items-center gap-2 text-sm">
                  <svg width="34" height="8" aria-hidden className="shrink-0">
                    <defs>
                      <marker
                        id={markerId}
                        viewBox="0 0 10 10"
                        refX="8"
                        refY="5"
                        markerWidth="5"
                        markerHeight="5"
                        orient="auto-start-reverse"
                      >
                        <path d="M 0 0 L 10 5 L 0 10 z" fill={style.color} />
                      </marker>
                    </defs>
                    <line
                      x1="1"
                      y1="4"
                      x2="33"
                      y2="4"
                      stroke={style.color}
                      strokeWidth={style.width}
                      strokeDasharray={style.dash}
                      markerEnd={style.directed ? `url(#${markerId})` : undefined}
                    />
                  </svg>
                  {r.label}
                </li>
              );
            })}
          </ul>
        </div>
      </PopoverContent>
    </Popover>
  );
}

// ── Selected-concept side panel ──────────────────────────────────────────────

function SelectedConceptPanel({
  graph,
  conceptId,
  onClose,
  onFocus,
  onOpenConcept,
}: {
  graph: GraphViz;
  conceptId: string;
  onClose: () => void;
  onFocus: (id: string) => void;
  onOpenConcept: (id: string) => void;
}) {
  const node = graph.nodes.find((n) => n.id === conceptId);
  const { data: concept, isLoading } = useConcept(conceptId);
  const neighbours = useMemo(() => relationNeighbours(graph, conceptId), [graph, conceptId]);
  const labelOf = useMemo(
    () => new Map(graph.nodes.map((n) => [n.id, n.label || n.id])),
    [graph.nodes],
  );

  const terms = concept?.terms ?? [];
  const byLocale = useMemo(() => {
    const map = new Map<string, typeof terms>();
    for (const t of terms) {
      const arr = map.get(t.locale) ?? [];
      arr.push(t);
      map.set(t.locale, arr);
    }
    return [...map.entries()];
  }, [terms]);

  return (
    <aside className="absolute inset-y-0 right-0 flex w-80 flex-col border-l bg-background shadow-lg">
      <div className="flex items-start justify-between gap-2 border-b p-4">
        <div className="min-w-0 space-y-1">
          <div className="flex items-center gap-2">
            <span
              aria-hidden
              className="size-2.5 shrink-0 rounded-full"
              style={{ backgroundColor: statusColorVar(node?.status) }}
            />
            <h3 className="truncate font-medium">{node?.label ?? conceptId}</h3>
          </div>
          {node?.domain && (
            <Badge variant="outline" className="text-[10px] font-normal">
              {node.domain}
            </Badge>
          )}
        </div>
        <Button
          variant="ghost"
          size="icon"
          className="size-7 shrink-0 text-muted-foreground hover:text-foreground"
          onClick={onClose}
          aria-label="Close panel"
        >
          <X className="size-4" />
        </Button>
      </div>

      <ScrollArea className="flex-1">
        <div className="space-y-4 p-4">
          {concept?.definition && (
            <p className="text-sm text-muted-foreground">{concept.definition}</p>
          )}

          <section className="space-y-2">
            <h4 className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
              Terms
            </h4>
            {isLoading ? (
              <Skeleton className="h-14 w-full" />
            ) : byLocale.length === 0 ? (
              <p className="text-sm text-muted-foreground">No terms on this concept.</p>
            ) : (
              <ul className="space-y-1.5">
                {byLocale.map(([locale, localeTerms]) => (
                  <li key={locale} className="flex items-center gap-2 text-sm">
                    <span className="w-14 shrink-0 font-mono text-xs text-muted-foreground">
                      {locale}
                    </span>
                    <span className="truncate text-foreground">{localeTerms[0].text}</span>
                    <TermStatusBadge
                      status={(localeTerms[0].status as TermStatus) ?? "proposed"}
                      className="ml-auto text-[10px]"
                    />
                  </li>
                ))}
              </ul>
            )}
          </section>

          <section className="space-y-2">
            <h4 className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
              Relations
            </h4>
            {neighbours.length === 0 ? (
              <p className="text-sm text-muted-foreground">No relations.</p>
            ) : (
              <ul className="space-y-1.5">
                {neighbours.map((n) => (
                  <li key={n.edgeId} className="flex items-center gap-2">
                    <RelationBadge type={n.type} className="shrink-0 text-[10px]" />
                    <button
                      type="button"
                      onClick={() => onFocus(n.otherId)}
                      className="flex min-w-0 flex-1 items-center gap-1 truncate text-left text-sm text-foreground hover:underline"
                    >
                      {n.outgoing ? (
                        <ArrowRight className="size-3 shrink-0 text-muted-foreground" />
                      ) : (
                        <ArrowLeft className="size-3 shrink-0 text-muted-foreground" />
                      )}
                      <span className="truncate">{labelOf.get(n.otherId) ?? n.otherId}</span>
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </section>
        </div>
      </ScrollArea>

      <div className="border-t p-3">
        <Button className="w-full gap-2" onClick={() => onOpenConcept(conceptId)}>
          <ScrollText className="size-4" />
          Open story
        </Button>
      </div>
    </aside>
  );
}
