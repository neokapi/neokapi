import { useCallback, useEffect, useMemo, useState } from "react";
import {
  Badge,
  Button,
  Card,
  CardContent,
  EmptyState,
  LocalePill,
  Skeleton,
  directionAttrs,
} from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import { Check, Loader2, PauseCircle } from "lucide-react";
import { api } from "../hooks/useApi";
import { useError } from "./ErrorBanner";
import type { ProjectFilter, SourceQueueItem } from "../types/api";

const itemId = (it: SourceQueueItem) => `${it.file}:${it.key}`;

export interface SourceLaneProps {
  tabID: string;
  filter: ProjectFilter | null;
  /** Pre-loaded queue for Storybook/tests — skips api.getSourceQueue(). */
  items?: SourceQueueItem[];
  /** Override the approve handler (Storybook/tests). */
  onApprove?: (item: SourceQueueItem) => Promise<void>;
  /** Override the edit handler (Storybook/tests); resolves to the locales the
   *  next run will re-draft. */
  onSaveSource?: (item: SourceQueueItem, text: string) => Promise<string[]>;
  /** Refetch owned by the parent, so the lane toggle's count and the lane never
   *  disagree about how much source work is left. */
  onChanged?: () => Promise<void> | void;
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
  filter,
  items: propItems,
  onApprove,
  onSaveSource,
  onChanged,
}: SourceLaneProps) {
  const { showError } = useError();
  const [queue, setQueue] = useState<SourceQueueItem[] | null>(propItems ?? null);
  const [loading, setLoading] = useState(!propItems);
  const [selectedID, setSelectedID] = useState<string | null>(null);
  const [editText, setEditText] = useState("");
  const [busy, setBusy] = useState(false);
  const [awaiting, setAwaiting] = useState<string[] | null>(null);

  const refresh = useCallback(async () => {
    if (onChanged) {
      await onChanged();
      return;
    }
    if (propItems) {
      setQueue(propItems);
      setLoading(false);
      return;
    }
    setLoading(true);
    try {
      setQueue((await api.getSourceQueue(tabID, filter ?? { id: "", name: "" })) ?? []);
    } catch (err) {
      showError("Failed to load the source queue", err);
      setQueue([]);
    } finally {
      setLoading(false);
    }
  }, [tabID, filter, propItems, onChanged, showError]);

  useEffect(() => {
    if (propItems) {
      setQueue(propItems);
      setLoading(false);
      return;
    }
    void refresh();
  }, [propItems, refresh]);

  const selected = useMemo(
    () => (queue ?? []).find((it) => itemId(it) === selectedID) ?? (queue ?? [])[0],
    [queue, selectedID],
  );

  // A new selection replaces the editor's contents and drops the last result.
  useEffect(() => {
    setEditText(selected?.source ?? "");
    setAwaiting(null);
  }, [selected?.file, selected?.key, selected?.source]);

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

  const items = queue ?? [];
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
              {it.held && <PauseCircle size={12} className="shrink-0 text-amber-500" />}
              <span className="truncate font-mono text-xs" translate="no">
                {it.key}
              </span>
              <Badge variant="outline" className="ml-auto text-[10px]">
                {it.status}
              </Badge>
            </span>
            <span className="truncate text-xs text-muted-foreground">{it.source}</span>
          </button>
        ))}
      </div>

      {selected && (
        <Card>
          <CardContent className="space-y-3 p-3">
            <div className="flex flex-wrap items-center gap-1.5 text-xs text-muted-foreground">
              <span translate="no">
                {selected.file}:{selected.key}
              </span>
              {selected.sourceLocale && <LocalePill locale={selected.sourceLocale} />}
              {selected.held && (
                <Badge variant="outline" className="border-amber-500/40 text-[10px] text-amber-600">
                  {t("holding every language")}
                </Badge>
              )}
            </div>

            <div>
              <p className="mb-1 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
                {t("Source")}
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
              <Button size="xs" onClick={() => void approve()} disabled={busy}>
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
      )}
    </div>
  );
}
