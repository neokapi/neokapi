/**
 * ConceptCard — displays a termbase concept with reference term, target terms,
 * and action buttons. Built on shadcn Card/Badge/Button primitives.
 *
 * The reference (source) language term is shown prominently at the top.
 * Target language terms are listed below with colored locale pills and
 * status badges. Actions (Edit/Delete) appear on hover.
 */

import type { ConceptDTO, TermDTO } from "./types";
import { LocalePill } from "./LocalePill";
import { TermStatusBadge } from "./TermStatusBadge";
import { Card, CardContent } from "../ui/card";
import { Badge } from "../ui/badge";
import { Button } from "../ui/button";
import { Pencil, Trash2 } from "lucide-react";
import { cn } from "../../lib/utils";

export interface ConceptCardProps {
  concept: ConceptDTO;
  /** Reference locale — the source language term is shown prominently. */
  referenceLocale?: string;
  /** Whether the card is selected (checkbox state). */
  selected?: boolean;
  /** Toggle selection callback. */
  onToggleSelect?: () => void;
  /** Edit callback. */
  onEdit?: () => void;
  /** Delete callback (with confirmation state). */
  onDelete?: () => void;
  /** Whether the delete confirmation is showing. */
  deleteConfirm?: boolean;
  /** Confirm the pending delete. */
  onDeleteConfirm?: () => void;
  /** Cancel the pending delete. */
  onDeleteCancel?: () => void;
  className?: string;
}

export function ConceptCard({
  concept,
  referenceLocale,
  selected = false,
  onToggleSelect,
  onEdit,
  onDelete,
  deleteConfirm,
  onDeleteConfirm,
  onDeleteCancel,
  className,
}: ConceptCardProps) {
  // Separate reference term from target terms.
  const refTerm = referenceLocale
    ? concept.terms.find((t: TermDTO) => t.locale === referenceLocale)
    : concept.terms[0];
  const targetTerms = concept.terms.filter((t: TermDTO) => t !== refTerm);

  return (
    <Card
      className={cn(
        "group transition-colors",
        selected && "border-primary/40 bg-primary/5",
        className,
      )}
      data-testid={`concept-${concept.id}`}
    >
      <CardContent className="p-4">
        {/* Header: checkbox + domain + scope */}
        <div className="mb-2 flex items-center justify-between">
          <div className="flex items-center gap-2">
            {onToggleSelect && (
              <input
                type="checkbox"
                checked={selected}
                onChange={onToggleSelect}
                className="rounded"
              />
            )}
            {concept.domain && (
              <Badge
                variant="secondary"
                className="text-[10px] font-semibold uppercase tracking-wider"
              >
                {concept.domain}
              </Badge>
            )}
          </div>
          {concept.project_id ? (
            <Badge
              variant="outline"
              className="border-blue-500/30 bg-blue-500/10 text-[10px] text-blue-600 dark:text-blue-400"
            >
              Project
            </Badge>
          ) : (
            <Badge variant="secondary" className="text-[10px]">
              User
            </Badge>
          )}
        </div>

        {/* Reference term — large, prominent */}
        {refTerm && (
          <div className="mb-1.5 flex items-center gap-2">
            <span className="text-sm font-semibold text-foreground">{refTerm.text}</span>
            <LocalePill locale={refTerm.locale} />
            {refTerm.status !== "preferred" && <TermStatusBadge status={refTerm.status} />}
          </div>
        )}

        {/* Definition */}
        {concept.definition && (
          <p className="mb-2 line-clamp-2 text-[11px] italic text-muted-foreground">
            {concept.definition}
          </p>
        )}

        {/* Target terms — compact rows with left border accent */}
        {targetTerms.length > 0 && (
          <div className="mb-2 space-y-1 border-l-2 border-border/40 pl-3">
            {targetTerms.map((term: TermDTO, idx: number) => (
              <div key={idx} className="flex items-center gap-2">
                <LocalePill locale={term.locale} />
                <span className="text-[12px] text-foreground">{term.text}</span>
                {term.status !== "preferred" && <TermStatusBadge status={term.status} />}
                {term.note && (
                  <span className="text-[10px] text-muted-foreground">({term.note})</span>
                )}
              </div>
            ))}
          </div>
        )}

        {/* Actions — visible on hover */}
        {(onEdit || onDelete) && (
          <div className="flex gap-1 pt-2 opacity-0 transition-opacity group-hover:opacity-100">
            {onEdit && (
              <Button
                variant="ghost"
                size="sm"
                className="h-6 text-[10px] text-muted-foreground"
                onClick={onEdit}
              >
                <Pencil size={10} />
                Edit
              </Button>
            )}
            {onDelete &&
              (deleteConfirm ? (
                <>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="h-6 text-[10px] text-destructive"
                    onClick={onDeleteConfirm}
                  >
                    Confirm
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="h-6 text-[10px] text-muted-foreground"
                    onClick={onDeleteCancel}
                  >
                    Cancel
                  </Button>
                </>
              ) : (
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-6 text-[10px] text-destructive/70"
                  onClick={onDelete}
                >
                  <Trash2 size={10} />
                  Delete
                </Button>
              ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
