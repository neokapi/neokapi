import { useEffect, useState } from "react";
import {
  Badge,
  Card,
  CardContent,
  LocaleLabel,
  NeighbourhoodCard,
  PointCard,
  localeLabel,
} from "@neokapi/ui-primitives";
import type { BlockRead, ChangeResult } from "@neokapi/contract-types";
import { t } from "@neokapi/i18n-react/runtime";
import { api } from "../../hooks/useApi";
import { editionContent, invalidatedEditions } from "../../lib/changes";
import { EditionEditPanel } from "../edit/EditionEditPanel";
import type { ChangeSender } from "../edit/useChangeSender";
import type { ReviewContext, ReviewItem } from "../../types/api";

export interface SourceUnitPaneProps {
  tabID: string;
  /** The selected source row of the review queue. */
  item: ReviewItem;
  /** The source unit as the change service read it; null while it loads. */
  read: BlockRead | null;
  /** Why the unit could not be read. */
  readError?: string | null;
  /** Sends the edit, and holds a refusal for the revision that moved. */
  sender: ChangeSender;
  /** The last edit of this unit that landed, whose stale translations the pane names. */
  saved?: ChangeResult | null;
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
 * waiting on the approval `translate_after: established` asks for.
 *
 * The wording is edited as the change service reads it, inline codes and all,
 * and saved as a set_content with the revision the pane read. Every
 * translation of it stays in place: the save names the translations it left on
 * an older source, and the next run re-drafts them against the wording the
 * project has now.
 *
 * The decision itself sits in the page's action bar with the keyboard verbs,
 * where a target row's decision sits, so one list has one set of verbs.
 */
export function SourceUnitPane({
  tabID,
  item,
  read,
  readError,
  sender,
  saved,
  loadContext,
}: SourceUnitPaneProps) {
  // The point this unit's file sits at, and the blocks around it. Source review
  // and target review render one model, so the wording is judged against the
  // voice that governs it rather than on its own.
  const [model, setModel] = useState<ReviewContext | null>(null);
  const [modelLoading, setModelLoading] = useState(false);

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

  const awaiting = saved ? invalidatedEditions(saved) : null;
  const content = read ? editionContent(read) : null;

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
            {readError ? (
              <p className="text-xs text-destructive" data-slot="source-unit-read-error">
                {readError}
              </p>
            ) : (
              <EditionEditPanel
                sender={sender}
                content={content}
                locale={item.sourceLocale}
                readOnlyReason={
                  read && !read.ops.includes("set_content")
                    ? t("This format does not write a change to this text.")
                    : undefined
                }
                saveLabel={t("Save and re-draft")}
                note={t("Source edited in review")}
                compact
                autoFocus={false}
                data-slot="source-unit"
              />
            )}
          </div>

          <p className="text-[11px] text-muted-foreground">
            {t(
              "Approving binds to this exact wording: editing it later drops the approval. Saving an edit leaves every translation in place, and the next run re-drafts the ones it wrote.",
            )}
          </p>

          {awaiting !== null && (
            <p className="text-[11px] text-muted-foreground" data-slot="source-unit-awaiting">
              {awaiting.length === 0
                ? t("Source saved. No language has a translation of this block yet.")
                : t("Source saved. {langs} will be re-drafted on the next run.", {
                    langs: awaiting.map((l) => localeLabel(l)).join(", "),
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
