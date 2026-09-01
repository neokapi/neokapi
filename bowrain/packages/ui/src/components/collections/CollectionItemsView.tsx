import { Button, Card, CardContent, cn } from "@neokapi/ui-primitives";
import { One, Other, Plural } from "@neokapi/i18n-react/runtime";
import type {
  CollectionTranslationStats,
  ItemTranslationStats,
  LocaleTranslationStats,
} from "../../types/api";
import { ArrowLeft } from "../icons";
import { CoordinateLine } from "../../context-hub/profiles/Coordinates";
import { FileProgressTable, type FileProgressPaging } from "../FileProgressTable";
import { FilePreview, type ItemPreviewBinding } from "../FilePreview";
import { LocaleCoverageRails } from "./LocaleCoverageRail";
import { useInContextItems } from "../../hooks/useInContextItems";

/**
 * CollectionItemsView is the overview's second level: the items of one
 * collection, under a header restating which collection they belong to and how
 * it stands. Reached from a collection card, or unscoped from the all-items
 * entry, in which case the rollup header is omitted and only the table remains.
 *
 * The item list is the server's — the endpoint filters by collection, so the
 * table's "N of M" counts that collection rather than a page of the project.
 */
export interface CollectionItemsViewProps {
  /** The scope's rollup; omitted for the unscoped all-items list. */
  collection?: CollectionTranslationStats;
  /** Heading for the scope. */
  title: string;
  itemStats: ItemTranslationStats[];
  /** The project's target locales, in the order the table columns follow. */
  localeStats: LocaleTranslationStats[];
  /** The scope's shared directory prefix (`item_base`); names show relative to it. */
  itemBase?: string;
  paging?: FileProgressPaging;
  onBack: () => void;
  /** Open review scoped to this collection. */
  onReview?: () => void;
  /** Open one item in the translation editor. Ignored when `preview` is set. */
  onOpenItem?: (item: ItemTranslationStats) => void;
  /**
   * Given, a row opens the item's preview instead of an editor, and the editors
   * are reached from the preview's own actions. The project id the preview reads
   * its blocks under comes with it.
   */
  preview?: ItemPreviewBinding & { projectId: string };
  className?: string;
}

export function CollectionItemsView({
  collection,
  title,
  itemStats,
  localeStats,
  itemBase,
  paging,
  onBack,
  onReview,
  onOpenItem,
  preview,
  className,
}: CollectionItemsViewProps) {
  const locales = localeStats.map((l) => l.locale);
  const previewItem = preview ? itemStats.find((i) => i.item_name === preview.itemName) : undefined;
  const localeDisplayNames = Object.fromEntries(
    localeStats.filter((l) => l.display_name).map((l) => [l.locale, l.display_name!]),
  );
  // Where THIS collection publishes its components. Read from the rollup the
  // view was opened with, so the offer is the same on the page a reviewer opens
  // a file from as on the project page.
  const collectionPreview =
    collection?.preview_kind && collection.preview_url
      ? { kind: collection.preview_kind, url: collection.preview_url }
      : undefined;
  // Resolved once for the whole collection: which of these rows have a
  // component published beside them, before a reviewer opens one to find out.
  const inContext = useInContextItems(
    preview?.projectId ?? "",
    collection?.collection_id,
    collectionPreview,
  );

  return (
    <div className={cn("space-y-4", className)} data-testid="collection-items-view">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex min-w-0 items-center gap-2">
          <Button variant="ghost" size="sm" onClick={onBack} data-testid="collection-items-back">
            <ArrowLeft className="h-4 w-4" />
            Overview
          </Button>
          <h1 className="min-w-0 truncate text-lg font-semibold">{title}</h1>
        </div>
        {onReview && (
          <Button variant="outline" size="sm" onClick={onReview}>
            Review
          </Button>
        )}
      </div>

      {collection && (
        <Card className="gap-3">
          <CardContent className="space-y-3 pt-0">
            <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
              {collection.channel && (
                <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-[11px] text-foreground">
                  {collection.channel}
                </code>
              )}
              <span className="tabular-nums">
                <Plural count={collection.item_count}>
                  <One>{collection.item_count.toLocaleString()} item</One>
                  <Other>{collection.item_count.toLocaleString()} items</Other>
                </Plural>
              </span>
              <span className="tabular-nums">
                <Plural count={collection.word_count}>
                  <One>{collection.word_count.toLocaleString()} word</One>
                  <Other>{collection.word_count.toLocaleString()} words</Other>
                </Plural>
              </span>
              <span className="tabular-nums">
                <Plural count={collection.block_count}>
                  <One>{collection.block_count.toLocaleString()} block</One>
                  <Other>{collection.block_count.toLocaleString()} blocks</Other>
                </Plural>
              </span>
              <CoordinateLine coordinates={collection.coordinates} className="text-[11px]" />
            </div>
            <LocaleCoverageRails locales={collection.locales} className="max-h-56" />
          </CardContent>
        </Card>
      )}

      <FileProgressTable
        itemStats={itemStats}
        locales={locales}
        localeDisplayNames={localeDisplayNames}
        itemBase={itemBase}
        paging={paging}
        onOpenItem={preview ? (item) => preview.onOpen(item.item_name) : onOpenItem}
        inContext={inContext.enabled ? inContext.has : undefined}
      />

      {preview && (
        <FilePreview
          projectId={preview.projectId}
          itemName={preview.itemName}
          format={previewItem?.format}
          sourcePath={previewItem?.source_path}
          targetLocales={preview.targetLocales ?? locales}
          sourceLocale={preview.sourceLocale}
          onClose={preview.onClose}
          onOpenTranslate={preview.onOpenTranslate}
          onOpenReview={preview.onOpenReview}
          onOpenPreProcess={preview.onOpenPreProcess}
          preview={collectionPreview}
          collectionId={collection?.collection_id}
          hasStory={inContext.has(previewItem?.source_path)}
        />
      )}
    </div>
  );
}
