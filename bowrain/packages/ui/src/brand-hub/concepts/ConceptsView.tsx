// Concepts — the entry section of the Brand hub (AD-021). A searchable, filterable
// table of concepts (the node type of the brand knowledge graph), with a graph
// view toggle and concept creation. Absorbs the former standalone Termbase.
import { useMemo, useState } from "react";
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
  Skeleton,
  Textarea,
  cn,
} from "@neokapi/ui-primitives";
import { Search, Plus, Network, Layers } from "../../components/icons";
import type { ConceptInfo, TermInfo } from "../../types/api";
import type { ListConceptsParams, TermStatus, TermSource } from "../../types/brand-graph";
import { useConcepts, useCreateConcept } from "../../hooks/useConceptsApi";
import { useMarkets } from "../../hooks/useMarketsApi";
import { BrandHub } from "../shell/BrandHub";
import { TermStatusBadge, EmptyState } from "../shell/atoms";
import { ConceptGraphView } from "./ConceptGraphView";

const TERM_STATUSES: TermStatus[] = [
  "proposed",
  "approved",
  "preferred",
  "admitted",
  "deprecated",
  "forbidden",
];

const ALL = "all";

export interface ConceptsViewProps {
  /** Open a concept's story page. */
  onOpenConcept: (conceptId: string) => void;
}

export function ConceptsView({ onOpenConcept }: ConceptsViewProps) {
  const [q, setQ] = useState("");
  const [status, setStatus] = useState<string>(ALL);
  const [source, setSource] = useState<string>(ALL);
  const [market, setMarket] = useState<string>(ALL);
  const [mode, setMode] = useState<"list" | "graph">("list");
  const [createOpen, setCreateOpen] = useState(false);

  const params = useMemo<ListConceptsParams>(() => {
    const p: ListConceptsParams = {};
    if (q.trim()) p.q = q.trim();
    if (status !== ALL) p.status = status as TermStatus;
    if (source !== ALL) p.source = source as TermSource;
    if (market !== ALL) p.market = market;
    return p;
  }, [q, status, source, market]);

  const { data, isLoading } = useConcepts(params);
  const { data: markets } = useMarkets();

  const concepts = data?.concepts ?? [];

  return (
    <BrandHub
      title="Concepts"
      description="The language-neutral units of your brand — each with its terms, status by locale, and place in the graph."
      width="wide"
      actions={
        <Button onClick={() => setCreateOpen(true)} size="sm">
          <Plus />
          New concept
        </Button>
      }
      toolbar={
        <div className="flex flex-wrap items-center gap-2">
          {mode === "list" && (
            <>
              <div className="relative min-w-56 flex-1">
                <Search className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
                <Input
                  value={q}
                  onChange={(e) => setQ(e.target.value)}
                  placeholder="Search concepts and terms…"
                  className="pl-8"
                />
              </div>
              <Select value={status} onValueChange={setStatus}>
                <SelectTrigger className="w-36" size="sm">
                  <SelectValue placeholder="Status" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={ALL}>All statuses</SelectItem>
                  {TERM_STATUSES.map((s) => (
                    <SelectItem key={s} value={s} className="capitalize">
                      {s}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Select value={source} onValueChange={setSource}>
                <SelectTrigger className="w-40" size="sm">
                  <SelectValue placeholder="Source" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={ALL}>All sources</SelectItem>
                  <SelectItem value="terminology">Terminology</SelectItem>
                  <SelectItem value="brand_vocabulary">Brand vocabulary</SelectItem>
                </SelectContent>
              </Select>
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
            </>
          )}
          {mode === "graph" && (
            <p className="text-sm text-muted-foreground">
              Explore how concepts connect — search, scope by time and market, and open any
              concept's story.
            </p>
          )}
          <div className="ml-auto flex items-center rounded-md border bg-muted/40 p-0.5">
            <ModeButton active={mode === "list"} onClick={() => setMode("list")} label="List">
              <Layers />
            </ModeButton>
            <ModeButton active={mode === "graph"} onClick={() => setMode("graph")} label="Graph">
              <Network />
            </ModeButton>
          </div>
        </div>
      }
    >
      {mode === "graph" ? (
        <ConceptGraphView onOpenConcept={onOpenConcept} />
      ) : isLoading ? (
        <ConceptListSkeleton />
      ) : concepts.length === 0 ? (
        <EmptyState
          icon={<Layers />}
          title="No concepts yet"
          description="Create your first concept, or import terminology, to start mapping your brand language."
          action={
            <Button onClick={() => setCreateOpen(true)} size="sm" variant="outline">
              <Plus />
              New concept
            </Button>
          }
        />
      ) : (
        <ul className="divide-y rounded-lg border">
          {concepts.map((c) => (
            <ConceptRow key={c.id} concept={c} onOpen={() => onOpenConcept(c.id)} />
          ))}
        </ul>
      )}

      {mode === "list" && data && (
        <p className="mt-3 text-xs text-muted-foreground">
          {data.total_count} concept{data.total_count === 1 ? "" : "s"}
        </p>
      )}

      <CreateConceptDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        onCreated={(id) => {
          setCreateOpen(false);
          onOpenConcept(id);
        }}
      />
    </BrandHub>
  );
}

function ModeButton({
  active,
  onClick,
  label,
  children,
}: {
  active: boolean;
  onClick: () => void;
  label: string;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={active}
      title={label}
      className={cn(
        "flex items-center gap-1.5 rounded px-2 py-1 text-xs font-medium transition-colors [&_svg]:size-3.5",
        active
          ? "bg-background text-foreground shadow-sm"
          : "text-muted-foreground hover:text-foreground",
      )}
    >
      {children}
      <span className="hidden sm:inline">{label}</span>
    </button>
  );
}

function ConceptRow({ concept, onOpen }: { concept: ConceptInfo; onOpen: () => void }) {
  // Group the terms by locale so the row reads as "what we call this, per locale".
  const byLocale = useMemo(() => groupTerms(concept.terms), [concept.terms]);
  const name = primaryName(concept);

  return (
    <li>
      <button
        type="button"
        onClick={onOpen}
        className="flex w-full items-start gap-4 px-4 py-3 text-left transition-colors hover:bg-muted/40"
      >
        <div className="min-w-0 flex-1 space-y-1">
          <div className="flex items-center gap-2">
            <span className="truncate font-medium text-foreground">{name}</span>
            {concept.domain && (
              <Badge variant="outline" className="shrink-0 text-[10px] font-normal">
                {concept.domain}
              </Badge>
            )}
          </div>
          {concept.definition && (
            <p className="line-clamp-1 text-sm text-muted-foreground">{concept.definition}</p>
          )}
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1 pt-0.5">
            {byLocale.slice(0, 4).map(({ locale, terms }) => (
              <span key={locale} className="flex items-center gap-1.5 text-xs">
                <span className="font-mono text-muted-foreground">{locale}</span>
                <span className="text-foreground">{terms[0].text}</span>
                <TermStatusBadge
                  status={(terms[0].status as TermStatus) ?? "proposed"}
                  className="text-[10px]"
                />
              </span>
            ))}
            {byLocale.length > 4 && (
              <span className="text-xs text-muted-foreground">+{byLocale.length - 4} more</span>
            )}
          </div>
        </div>
        <span className="shrink-0 pt-0.5 text-xs text-muted-foreground">
          {concept.terms.length} term{concept.terms.length === 1 ? "" : "s"}
        </span>
      </button>
    </li>
  );
}

function ConceptListSkeleton() {
  return (
    <div className="space-y-px rounded-lg border">
      {Array.from({ length: 6 }).map((_, i) => (
        <div key={i} className="flex items-center gap-4 px-4 py-3.5">
          <div className="flex-1 space-y-2">
            <Skeleton className="h-4 w-40" />
            <Skeleton className="h-3 w-72" />
          </div>
          <Skeleton className="h-4 w-12" />
        </div>
      ))}
    </div>
  );
}

interface CreateConceptDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreated: (conceptId: string) => void;
}

function CreateConceptDialog({ open, onOpenChange, onCreated }: CreateConceptDialogProps) {
  const create = useCreateConcept();
  const [domain, setDomain] = useState("");
  const [definition, setDefinition] = useState("");
  const [termText, setTermText] = useState("");
  const [locale, setLocale] = useState("en-US");
  const [termStatus, setTermStatus] = useState<TermStatus>("proposed");

  const reset = () => {
    setDomain("");
    setDefinition("");
    setTermText("");
    setLocale("en-US");
    setTermStatus("proposed");
  };

  const canSubmit = termText.trim().length > 0 && locale.trim().length > 0 && !create.isPending;

  const submit = () => {
    if (!canSubmit) return;
    create.mutate(
      {
        // Hub concepts are workspace-scoped; project_id is omitted so the server
        // stores no project affinity.
        domain: domain.trim(),
        definition: definition.trim(),
        terms: [{ text: termText.trim(), locale: locale.trim(), status: termStatus }],
      },
      {
        onSuccess: (created) => {
          reset();
          onCreated(created.id);
        },
      },
    );
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        if (!o) reset();
        onOpenChange(o);
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>New concept</DialogTitle>
          <DialogDescription>
            A concept holds one meaning across locales. Add its first term now; you can add more,
            relations, and observations from its story.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <div className="grid grid-cols-[1fr_8rem] gap-3">
            <div className="space-y-1.5">
              <Label htmlFor="bh-term">First term</Label>
              <Input
                id="bh-term"
                value={termText}
                onChange={(e) => setTermText(e.target.value)}
                placeholder="e.g. Checkout"
                autoFocus
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="bh-locale">Locale</Label>
              <Input
                id="bh-locale"
                value={locale}
                onChange={(e) => setLocale(e.target.value)}
                placeholder="en-US"
              />
            </div>
          </div>
          <div className="grid grid-cols-[1fr_10rem] gap-3">
            <div className="space-y-1.5">
              <Label htmlFor="bh-domain">Domain</Label>
              <Input
                id="bh-domain"
                value={domain}
                onChange={(e) => setDomain(e.target.value)}
                placeholder="optional, e.g. commerce"
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="bh-status">Status</Label>
              <Select value={termStatus} onValueChange={(v) => setTermStatus(v as TermStatus)}>
                <SelectTrigger id="bh-status">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {TERM_STATUSES.map((s) => (
                    <SelectItem key={s} value={s} className="capitalize">
                      {s}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="bh-def">Definition</Label>
            <Textarea
              id="bh-def"
              value={definition}
              onChange={(e) => setDefinition(e.target.value)}
              placeholder="What this concept means."
              rows={2}
            />
          </div>
          {create.isError && (
            <p className="text-sm text-destructive">
              {create.error instanceof Error ? create.error.message : "Could not create concept."}
            </p>
          )}
        </div>
        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button onClick={submit} disabled={!canSubmit}>
            {create.isPending ? "Creating…" : "Create concept"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// ── helpers ──────────────────────────────────────────────────────────────────

function groupTerms(terms: TermInfo[]): { locale: string; terms: TermInfo[] }[] {
  const map = new Map<string, TermInfo[]>();
  for (const t of terms) {
    const arr = map.get(t.locale) ?? [];
    arr.push(t);
    map.set(t.locale, arr);
  }
  return [...map.entries()].map(([locale, ts]) => ({ locale, terms: ts }));
}

/** A human label for a concept: a preferred/first English term, else any term, else the id. */
function primaryName(concept: ConceptInfo): string {
  if (concept.terms.length === 0) return concept.domain || concept.id;
  const preferred = concept.terms.find((t) => t.status === "preferred");
  const english = concept.terms.find((t) => t.locale.startsWith("en"));
  return (preferred ?? english ?? concept.terms[0]).text;
}
