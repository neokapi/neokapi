import { useState, useMemo } from "react";
import type { ConceptInfo, GraphEdge, GraphNode } from "../../types/api";
import { GRAPH_LABEL_DISPLAY } from "../../types/api";
import { Badge } from "../ui/badge";
import { Button } from "../ui/button";
import { Separator } from "../ui/separator";
import { X, Clock, Pencil, Trash2, Link, ArrowRight, ArrowLeft } from "../icons";
import { cn } from "../../lib/utils";
import { useLocales } from "../../hooks/useLocales";

interface ConceptDetailPanelProps {
  concept: ConceptInfo;
  /** Graph edges connected to this concept */
  edges: GraphEdge[];
  /** Neighbor nodes (resolved from edges) */
  neighbors: GraphNode[];
  /** All concepts, for resolving neighbor names */
  allConcepts: ConceptInfo[];
  onClose: () => void;
  onEdit: (concept: ConceptInfo) => void;
  onDelete: (conceptId: string) => void;
  /** Navigate to a related concept */
  onNavigate: (conceptId: string) => void;
}

const STATUS_VARIANTS: Record<string, "default" | "secondary" | "destructive" | "outline"> = {
  preferred: "default",
  approved: "default",
  admitted: "secondary",
  proposed: "outline",
  deprecated: "destructive",
  forbidden: "destructive",
};

function formatDate(iso: string): string {
  if (!iso) return "-";
  const d = new Date(iso);
  return d.toLocaleDateString(undefined, { year: "numeric", month: "short", day: "numeric" });
}

function formatDateTime(iso: string): string {
  if (!iso) return "-";
  const d = new Date(iso);
  return d.toLocaleString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

/** Edge coloring consistent with ConceptGraph */
const EDGE_COLORS: Record<string, string> = {
  BROADER: "hsl(220 70% 55%)",
  NARROWER: "hsl(220 70% 55%)",
  RELATED: "hsl(160 60% 45%)",
  PART_OF: "hsl(280 60% 55%)",
  HAS_PART: "hsl(280 60% 55%)",
  USE_INSTEAD: "hsl(30 90% 50%)",
  REPLACED_BY: "hsl(30 90% 50%)",
  EXACT_MATCH: "hsl(340 70% 55%)",
  CLOSE_MATCH: "hsl(340 50% 65%)",
  FORBIDDEN: "hsl(0 70% 50%)",
  PREFERRED: "hsl(140 60% 45%)",
  COMPETITOR: "hsl(0 60% 60%)",
};

export function ConceptDetailPanel({
  concept,
  edges,
  neighbors,
  allConcepts,
  onClose,
  onEdit,
  onDelete,
  onNavigate,
}: ConceptDetailPanelProps) {
  const { getDisplayName } = useLocales();
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);

  const preferred = concept.terms.find((t) => t.status === "preferred") ?? concept.terms[0];

  // Group terms by locale
  const termsByLocale = useMemo(() => {
    const map = new Map<string, typeof concept.terms>();
    for (const t of concept.terms) {
      const arr = map.get(t.locale) ?? [];
      arr.push(t);
      map.set(t.locale, arr);
    }
    return map;
  }, [concept.terms]);

  // Build concept name lookup for neighbors
  const conceptNames = useMemo(() => {
    const m = new Map<string, string>();
    for (const c of allConcepts) {
      const pref = c.terms.find((t) => t.status === "preferred") ?? c.terms[0];
      m.set(c.id, pref?.text ?? c.id);
    }
    for (const n of neighbors) {
      if (!m.has(n.id)) m.set(n.id, n.properties?.name ?? n.id);
    }
    return m;
  }, [allConcepts, neighbors]);

  // Categorize edges into outgoing and incoming
  const outgoingEdges = edges.filter((e) => e.source === concept.id);
  const incomingEdges = edges.filter((e) => e.target === concept.id);

  return (
    <div className="w-[380px] border-l border-border bg-card flex flex-col h-full overflow-hidden">
      {/* Header */}
      <div className="flex items-start justify-between p-4 pb-3">
        <div className="flex-1 min-w-0">
          {concept.domain && (
            <span className="text-[10px] uppercase tracking-wider text-muted-foreground font-medium">
              {concept.domain}
            </span>
          )}
          <h3
            className="text-lg font-semibold leading-tight mt-0.5 truncate"
            title={preferred?.text}
          >
            {preferred?.text ?? "Untitled"}
          </h3>
          <div className="flex items-center gap-2 mt-1">
            <Badge
              variant={concept.project_id ? "default" : "secondary"}
              className="text-[10px] px-1.5 py-0"
            >
              {concept.project_id ? "Project" : "Workspace"}
            </Badge>
            <span className="text-[11px] text-muted-foreground flex items-center gap-1">
              <Clock className="w-3 h-3" />
              {formatDate(concept.created_at)}
            </span>
          </div>
        </div>
        <Button variant="ghost" size="sm" onClick={onClose} className="h-7 w-7 p-0 -mt-1 -mr-1">
          <X className="w-4 h-4" />
        </Button>
      </div>

      <Separator />

      {/* Scrollable content */}
      <div className="flex-1 overflow-y-auto">
        {/* Definition */}
        <div className="p-4">
          <div className="text-[11px] font-medium text-muted-foreground uppercase tracking-wider mb-1.5">
            Definition
          </div>
          <p className="text-sm leading-relaxed">
            {concept.definition || (
              <span className="text-muted-foreground italic">No definition provided</span>
            )}
          </p>
        </div>

        <Separator />

        {/* Terms by locale */}
        <div className="p-4">
          <div className="text-[11px] font-medium text-muted-foreground uppercase tracking-wider mb-2">
            Terms ({concept.terms.length})
          </div>
          <div className="space-y-3">
            {[...termsByLocale.entries()].map(([locale, terms]) => (
              <div key={locale}>
                <div className="text-[11px] font-medium text-foreground mb-1">
                  {getDisplayName(locale)}
                  <span className="text-muted-foreground ml-1">({locale})</span>
                </div>
                <div className="space-y-1 ml-2">
                  {terms.map((term, i) => (
                    <div key={i} className="flex items-center gap-1.5">
                      <span
                        className={cn(
                          "text-sm",
                          term.status === "preferred" && "font-semibold",
                          term.status === "deprecated" && "line-through text-muted-foreground",
                          term.status === "forbidden" && "line-through text-destructive",
                        )}
                      >
                        {term.text}
                      </span>
                      <Badge
                        variant={STATUS_VARIANTS[term.status] ?? "secondary"}
                        className="text-[9px] px-1 py-0 h-3.5"
                      >
                        {term.status}
                      </Badge>
                      {term.part_of_speech && (
                        <span className="text-[10px] text-muted-foreground italic">
                          {term.part_of_speech}
                        </span>
                      )}
                    </div>
                  ))}
                </div>
              </div>
            ))}
          </div>
        </div>

        <Separator />

        {/* Relationships — the graph navigation section */}
        <div className="p-4">
          <div className="text-[11px] font-medium text-muted-foreground uppercase tracking-wider mb-2">
            Relationships ({outgoingEdges.length + incomingEdges.length})
          </div>

          {outgoingEdges.length === 0 && incomingEdges.length === 0 ? (
            <p className="text-[12px] text-muted-foreground italic">
              No relationships yet. Use the graph view to connect concepts.
            </p>
          ) : (
            <div className="space-y-1.5">
              {outgoingEdges.map((edge) => (
                <button
                  key={edge.id}
                  onClick={() => onNavigate(edge.target)}
                  className="w-full flex items-center gap-2 px-2.5 py-1.5 rounded-md hover:bg-accent transition-colors text-left group"
                >
                  <ArrowRight
                    className="w-3.5 h-3.5 flex-shrink-0"
                    style={{ color: EDGE_COLORS[edge.label] }}
                  />
                  <span
                    className="text-[11px] font-medium flex-shrink-0"
                    style={{ color: EDGE_COLORS[edge.label] }}
                  >
                    {GRAPH_LABEL_DISPLAY[edge.label] ?? edge.label}
                  </span>
                  <span className="text-sm truncate group-hover:text-primary transition-colors">
                    {conceptNames.get(edge.target) ?? edge.target}
                  </span>
                  <Link className="w-3 h-3 ml-auto text-muted-foreground opacity-0 group-hover:opacity-100 transition-opacity flex-shrink-0" />
                </button>
              ))}
              {incomingEdges.map((edge) => (
                <button
                  key={edge.id}
                  onClick={() => onNavigate(edge.source)}
                  className="w-full flex items-center gap-2 px-2.5 py-1.5 rounded-md hover:bg-accent transition-colors text-left group"
                >
                  <ArrowLeft
                    className="w-3.5 h-3.5 flex-shrink-0"
                    style={{ color: EDGE_COLORS[edge.label] }}
                  />
                  <span
                    className="text-[11px] font-medium flex-shrink-0"
                    style={{ color: EDGE_COLORS[edge.label] }}
                  >
                    {GRAPH_LABEL_DISPLAY[edge.label] ?? edge.label}
                  </span>
                  <span className="text-sm truncate group-hover:text-primary transition-colors">
                    {conceptNames.get(edge.source) ?? edge.source}
                  </span>
                  <Link className="w-3 h-3 ml-auto text-muted-foreground opacity-0 group-hover:opacity-100 transition-opacity flex-shrink-0" />
                </button>
              ))}
            </div>
          )}
        </div>

        <Separator />

        {/* Properties */}
        {concept.properties && Object.keys(concept.properties).length > 0 && (
          <>
            <div className="p-4">
              <div className="text-[11px] font-medium text-muted-foreground uppercase tracking-wider mb-2">
                Properties
              </div>
              <div className="space-y-1">
                {Object.entries(concept.properties).map(([k, v]) => (
                  <div key={k} className="flex items-center gap-2 text-sm">
                    <span className="text-muted-foreground">{k}:</span>
                    <span>{v}</span>
                  </div>
                ))}
              </div>
            </div>
            <Separator />
          </>
        )}

        {/* Timeline */}
        <div className="p-4">
          <div className="text-[11px] font-medium text-muted-foreground uppercase tracking-wider mb-2">
            Timeline
          </div>
          <div className="space-y-2">
            <div className="flex items-center gap-2">
              <div className="w-1.5 h-1.5 rounded-full bg-primary" />
              <div className="text-sm">
                <span className="text-muted-foreground">Created</span>
                <span className="ml-2">{formatDateTime(concept.created_at)}</span>
              </div>
            </div>
            {concept.updated_at && concept.updated_at !== concept.created_at && (
              <div className="flex items-center gap-2">
                <div className="w-1.5 h-1.5 rounded-full bg-muted-foreground" />
                <div className="text-sm">
                  <span className="text-muted-foreground">Updated</span>
                  <span className="ml-2">{formatDateTime(concept.updated_at)}</span>
                </div>
              </div>
            )}
          </div>
        </div>
      </div>

      {/* Actions footer */}
      <Separator />
      <div className="p-3 flex gap-2">
        <Button variant="outline" size="sm" className="flex-1" onClick={() => onEdit(concept)}>
          <Pencil className="w-3.5 h-3.5 mr-1.5" />
          Edit
        </Button>
        {showDeleteConfirm ? (
          <div className="flex gap-1">
            <Button
              variant="destructive"
              size="sm"
              onClick={() => {
                onDelete(concept.id);
                setShowDeleteConfirm(false);
              }}
            >
              Confirm
            </Button>
            <Button variant="outline" size="sm" onClick={() => setShowDeleteConfirm(false)}>
              Cancel
            </Button>
          </div>
        ) : (
          <Button
            variant="ghost"
            size="sm"
            className="text-destructive hover:text-destructive"
            onClick={() => setShowDeleteConfirm(true)}
          >
            <Trash2 className="w-3.5 h-3.5" />
          </Button>
        )}
      </div>
    </div>
  );
}
