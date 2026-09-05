import { useCallback, useEffect, useState } from "react";
import {
  Badge,
  Button,
  Card,
  CardContent,
  LocaleLabel,
  NeighbourhoodCard,
  PointCard,
  directionAttrs,
} from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import { api } from "../../hooks/useApi";
import { useError } from "../ErrorBanner";
import type { ReviewContext, ReviewItem } from "../../types/api";

export interface SourceUnitPaneProps {
  tabID: string;
  /** The selected source row of the review queue. */
  item: ReviewItem;
  /** Override the edit handler (Storybook/tests); resolves to the locales the
   *  next run will re-draft. */
  onSaveSource?: (item: ReviewItem, text: string) => Promise<string[]>;
  /** Refetch owned by the page, so the language selector's count and the pane
   *  agree about how much source work is left. */
  onChanged?: () => Promise<void> | void;
  /** Override the review-model loader (Storybook/tests); defaults to the
   *  GetSourceUnitContext binding. */
  loadContext?: (item: ReviewItem) => Promise<ReviewContext | null>;
}

/**
 * The detail pane for a source row: the author's half of the loop.
 *
 * A target row asks whether a translation is right for its source. A source row
 * asks the question underneath it, once rather than once per language: is the
 * source right at all. A unit here is holding every locale's translation, or
 * waiting on the signature an `approved` source gate asks for.
 *
 * Editing a source here leaves every translation of it in place. The loop
 * records the source it translated for each target it writes, so the next run
 * reads the edit as drift and re-drafts those units against the wording the
 * project has now. The pane names the languages that are waiting for it.
 *
 * The decision itself sits in the page's action bar with the keyboard verbs,
 * where a target row's decision sits, so one list has one set of verbs.
 */
export function SourceUnitPane({
  tabID,
  item,
  onSaveSource,
  onChanged,
  loadContext,
}: SourceUnitPaneProps) {
  const { showError } = useError();
  const [editText, setEditText] = useState(item.source ?? "");
  const [saving, setSaving] = useState(false);
  const [awaiting, setAwaiting] = useState<string[] | null>(null);
  // The point this unit's file sits at, and the blocks around it. Source review
  // and target review render one model, so the wording is judged against the
  // voice that governs it rather than on its own.
  const [model, setModel] = useState<ReviewContext | null>(null);
  const [modelLoading, setModelLoading] = useState(false);

  // A new selection replaces the editor's contents and drops the last result.
  useEffect(() => {
    setEditText(item.source ?? "");
    setAwaiting(null);
  }, [item.file, item.key, item.source]);

  // The model costs more than the row it was picked from, so the wording shows
  // straight away and the point fills in behind it.
  useEffect(() => {
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
        // leaves the pane usable rather than taking it down.
        if (!cancelled) setModel(null);
      })
      .finally(() => {
        if (!cancelled) setModelLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [tabID, item.file, item.key, loadContext]); // eslint-disable-line react-hooks/exhaustive-deps

  const save = useCallback(async () => {
    if (editText.trim() === "" || editText === item.source) return;
    setSaving(true);
    try {
      const locales = onSaveSource
        ? await onSaveSource(item, editText)
        : await api.updateSourceText(tabID, item.file, item.key, editText);
      setAwaiting(locales ?? []);
      await onChanged?.();
    } catch (err) {
      showError("Could not save the source", err);
    } finally {
      setSaving(false);
    }
  }, [item, editText, tabID, onSaveSource, onChanged, showError]);

  return (
    <div className="space-y-3" data-slot="source-unit-pane">
      <PointCard point={model?.point} loading={modelLoading} />

      <Card>
        <CardContent className="space-y-3 p-3">
          {item.held && (
            <Badge
              variant="outline"
              className="border-warning/40 text-[11px] text-warning"
              data-slot="source-unit-held"
            >
              {t("holding every language")}
            </Badge>
          )}

          <div>
            <p className="mb-1 text-[11px] font-medium text-muted-foreground">
              <LocaleLabel
                locale={item.sourceLocale ?? item.language ?? item.locale}
                source
                data-slot="source-unit-language"
              />
            </p>
            <textarea
              className="min-h-24 w-full resize-y rounded-md border bg-background p-2 text-sm"
              value={editText}
              onChange={(e) => setEditText(e.target.value)}
              aria-label={t("Edit the source wording")}
              data-slot="source-unit-editor"
              translate="no"
              {...directionAttrs(item.sourceLocale)}
            />
          </div>

          <div className="flex flex-wrap items-center gap-2">
            <Button
              variant="outline"
              size="xs"
              onClick={() => void save()}
              disabled={saving || editText.trim() === "" || editText === item.source}
              data-slot="source-unit-save"
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
            <p className="text-[11px] text-muted-foreground" data-slot="source-unit-awaiting">
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
        unitKey={item.key}
        unitSource={item.source}
        sourceLocale={item.sourceLocale}
        loading={modelLoading}
      />
    </div>
  );
}
