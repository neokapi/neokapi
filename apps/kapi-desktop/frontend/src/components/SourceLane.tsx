import { useCallback, useEffect, useMemo, useState } from "react";
import {
  Badge,
  Button,
  Card,
  CardContent,
  EmptyState,
  LocaleLabel,
  Skeleton,
  StatusBadge,
  directionAttrs,
} from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import { Check, Loader2, PauseCircle } from "lucide-react";
import { api } from "../hooks/useApi";
import { useError } from "./ErrorBanner";
import { NeighbourhoodCard } from "./review/NeighbourhoodCard";
import { PointRail } from "./review/PointRail";
import type { ReviewContext, ReviewItem } from "../types/api";

const itemId = (it: ReviewItem) => `${it.file}:${it.key}`;

export interface SourceLaneProps {
  tabID: string;
  /** The source rows of the unified review queue, owned by the page. */
  items: ReviewItem[];
  /** The queue is still on its way. */
  loading?: boolean;
  /** Override the approve handler (Storybook/tests). */
  onApprove?: (item: ReviewItem) => Promise<void>;
  /** Override the edit handler (Storybook/tests); resolves to the locales the
   *  next run will re-draft. */
  onSaveSource?: (item: ReviewItem, text: string) => Promise<string[]>;
  /** Refetch owned by the parent, so the language selector's count and the lane
   *  never disagree about how much source work is left. */
  onChanged?: () => Promise<void> | void;
  /** Override the review-model loader (Storybook/tests); defaults to the
   *  GetSourceUnitContext binding. */
  loadContext?: (item: ReviewItem) => Promise<ReviewContext | null>;
}

/**
 * The Review page's source lane: the author's half of the loop.
 *
 * The target lane asks whether a translation is right for its source. This asks
 * the question underneath it, once rather than once per language: is the source
 * right at all. A unit here is holding every locale's translation, or waiting on
 * the signature an `approved` source gate asks for.
 *
 * Editing a source here leaves every translation of it in place. The loop
 * records the source it translated for each target it writes, so the next run
 * reads the edit as drift and re-drafts those units against the wording the
 * project has now. The lane names the languages that are waiting for it.
 */
export function SourceLane({
  tabID,
  items,
  loading = false,
  onApprove,
  onSaveSource,
  onChanged,
  loadContext,
}: SourceLaneProps) {
  const { showError } = useError();
  const [selectedID, setSelectedID] = useState<string | null>(null);
  const [editText, setEditText] = useState("");
  const [busy, setBusy] = useState(false);
  const [awaiting, setAwaiting] = useState<string[] | null>(null);
  // The point this unit's file sits at, and the blocks around it. Source review
  // and target review render one model, so the wording is judged against the
  // voice that governs it rather than on its own.
  const [model, setModel] = useState<ReviewContext | null>(null);
  const [modelLoading, setModelLoading] = useState(false);

  const refresh = useCallback(async () => {
    await onChanged?.();
  }, [onChanged]);

  const selected = useMemo(
    () => items.find((it) => itemId(it) === selectedID) ?? items[0],
    [items, selectedID],
  );

  // A new selection replaces the editor's contents and drops the last result.
  useEffect(() => {
    setEditText(selected?.source ?? "");
    setAwaiting(null);
  }, [selected?.file, selected?.key, selected?.source]);

  // The model costs more than the row it was picked from, so the wording shows
  // straight away and the point fills in behind it.
  useEffect(() => {
    if (!selected) {
      setModel(null);
      return;
    }
    const item = selected;
    let cancelled = false;
    setModel(null);
    setModelLoading(true);
    const load =
      loadContext ?? ((it: ReviewItem) => api.getSourceUnitContext(tabID, it.file, it.key));
    load(item)
      .then((ctx) => {
        if (!cancelled) setModel(ctx ?? null);
      })
      .catch(() => {
        // The point is context around the decision, so failing to read it
        // leaves the lane usable rather than taking it down.
        if (!cancelled) setModel(null);
      })
      .finally(() => {
        if (!cancelled) setModelLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [tabID, selected?.file, selected?.key, loadContext]); // eslint-disable-line react-hooks/exhaustive-deps

  const approve = useCallback(async () => {
    if (!selected) return;
    setBusy(true);
    try {
      if (onApprove) await onApprove(selected);
      else await api.approveSourceUnit(tabID, selected.file, selected.key);
      await refresh();
    } catch (err) {
      showError("Could not approve the source", err);
    } finally {
      setBusy(false);
    }
  }, [selected, tabID, onApprove, refresh, showError]);

  const save = useCallback(async () => {
    if (!selected || editText.trim() === "" || editText === selected.source) return;
    setBusy(true);
    try {
      const locales = onSaveSource
        ? await onSaveSource(selected, editText)
        : await api.updateSourceText(tabID, selected.file, selected.key, editText);
      setAwaiting(locales ?? []);
      await refresh();
    } catch (err) {
      showError("Could not save the source", err);
    } finally {
      setBusy(false);
    }
  }, [selected, editText, tabID, onSaveSource, refresh, showError]);

  if (loading) {
    return (
      <div className="space-y-2" data-slot="source-lane-loading">
        <Skeleton className="h-8 w-full" />
        <Skeleton className="h-8 w-full" />
      </div>
    );
  }

  if (items.length === 0) {
    return (
      <EmptyState
        data-slot="source-lane-empty"
        title={t("The source is settled")}
        description={t(
          "Every source unit clears this project's source gate, so nothing is holding the translations.",
        )}
      />
    );
  }

  return (
    <div className="grid flex-1 grid-cols-[minmax(240px,1fr)_2fr] gap-4" data-slot="source-lane">
      <div className="overflow-auto rounded-lg border">
        {items.map((it) => (
          <button
            key={itemId(it)}
            type="button"
            onClick={() => setSelectedID(itemId(it))}
            className={`flex w-full flex-col gap-0.5 border-b px-3 py-2 text-left last:border-b-0 hover:bg-accent ${
              selected && itemId(selected) === itemId(it) ? "bg-accent" : ""
            }`}
          >
            <span className="flex items-center gap-1.5">
              {it.held && (
                <PauseCircle
                  size={12}
                  className="shrink-0 text-warning"
                  aria-label={t("holding every language")}
                />
              )}
              <span className="truncate font-mono text-xs" translate="no">
                {it.key}
              </span>
              {it.status && (
                <StatusBadge ladder="source" status={it.status} compact className="ml-auto" />
              )}
            </span>
            <span className="truncate text-xs text-muted-foreground">{it.source}</span>
          </button>
        ))}
      </div>

      {selected && (
        <div className="min-w-0 space-y-3 overflow-auto">
          <PointRail point={model?.point} loading={modelLoading} />

          <Card>
            <CardContent className="space-y-3 p-3">
              <div className="flex flex-wrap items-center gap-1.5 text-xs text-muted-foreground">
                <span translate="no">
                  {selected.file}:{selected.key}
                </span>
                {selected.status && <StatusBadge ladder="source" status={selected.status} />}
                {selected.held && (
                  <Badge variant="outline" className="border-warning/40 text-[10px] text-warning">
                    {t("holding every language")}
                  </Badge>
                )}
              </div>

              <div>
                <p className="mb-1 text-[11px] font-medium text-muted-foreground">
                  <LocaleLabel
                    locale={selected.sourceLocale ?? selected.language ?? selected.locale}
                    source
                    data-slot="source-lane-language"
                  />
                </p>
                <textarea
                  className="min-h-24 w-full resize-y rounded-md border bg-background p-2 text-sm"
                  value={editText}
                  onChange={(e) => setEditText(e.target.value)}
                  data-slot="source-lane-editor"
                  {...directionAttrs(selected.sourceLocale)}
                />
              </div>

              <div className="flex flex-wrap items-center gap-2">
                <Button variant="success" size="xs" onClick={() => void approve()} disabled={busy}>
                  {busy ? <Loader2 size={12} className="animate-spin" /> : <Check size={12} />}
                  {t("Approve source")}
                </Button>
                <Button
                  variant="outline"
                  size="xs"
                  onClick={() => void save()}
                  disabled={busy || editText.trim() === "" || editText === selected.source}
                  data-slot="source-lane-save"
                >
                  {t("Save and re-draft")}
                </Button>
              </div>

              <p className="text-[11px] text-muted-foreground">
                {t(
                  "Approving binds to this exact wording: editing it later drops the approval. Saving an edit leaves every translation in place, and the next run re-drafts the ones it wrote.",
                )}
              </p>

              {awaiting !== null && (
                <p className="text-[11px] text-muted-foreground" data-slot="source-lane-awaiting">
                  {awaiting.length === 0
                    ? t("Source saved. No language has a translation of this unit yet.")
                    : t("Source saved. {langs} will be re-drafted on the next run.", {
                        langs: awaiting.join(", "),
                      })}
                </p>
              )}
            </CardContent>
          </Card>

          <NeighbourhoodCard
            neighbourhood={model?.neighbourhood}
            unitKey={selected.key}
            unitSource={selected.source}
            sourceLocale={selected.sourceLocale}
            loading={modelLoading}
          />
        </div>
      )}
    </div>
  );
}
