import { useState } from "react";
import {
  Badge,
  Button,
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
  ToggleGroup,
  ToggleGroupItem,
} from "@neokapi/ui-primitives";
import { type CollectionPreview } from "../types/api";
import { useLocales } from "../hooks/useLocales";
import { DocumentPreview } from "./editor/DocumentPreview";
import { ItemStoryPreview, type ReadingMode } from "./editor/StoryPreview";
import { embedOrigin, storybookHost } from "./editor/previewHost";
import { LocaleSelect } from "./LocaleSelect";

/**
 * What a list of items needs to host the preview: which item it is open on, how
 * to open and close it, and where the explicit actions lead. The open item is
 * passed in rather than held here so the surface owning the URL owns the state —
 * the preview deep-links, and Back closes it.
 */
export interface ItemPreviewBinding {
  /** The item the preview is open on, by name; null when it is closed. */
  itemName: string | null;
  /** A row was clicked. */
  onOpen: (itemName: string) => void;
  onClose: () => void;
  /** The project's target locales, in the order the project declares them. */
  targetLocales?: string[];
  /** The project's source language, for the content-model preview's direction. */
  sourceLocale?: string;
  onOpenTranslate?: (itemName: string) => void;
  onOpenReview?: (itemName: string) => void;
  onOpenPreProcess?: (itemName: string) => void;
}

export interface FilePreviewProps {
  projectId: string;
  /** The item to read, addressed by name. Null closes the sheet. */
  itemName: string | null;
  /** The item's format id, shown beside its name. */
  format?: string;
  /**
   * The file this item's content was lifted out of, when the item is a
   * generated catalog rather than the source itself (`source_path`, recorded by
   * the extractor against a declared root). Given, the sheet is titled by the
   * source and names the catalog underneath: a reviewer opening a bundle is
   * looking at `bowrain/apps/bowrain/frontend/src/App.tsx`, and the item's own
   * name is what everything addresses it by — both are worth a line here, where
   * there is room for both.
   */
  sourcePath?: string;
  /** The project's target locales, in the order the project declares them. */
  targetLocales?: string[];
  /** The project's source language, for the content-model preview's direction. */
  sourceLocale?: string;
  /** Dismiss — Escape, the close button, or a click outside. */
  onClose: () => void;
  /** Open the item in the translation editor. */
  onOpenTranslate?: (itemName: string) => void;
  /** Open the item in review. */
  onOpenReview?: (itemName: string) => void;
  /** Open the item's pre-processing surface. */
  onOpenPreProcess?: (itemName: string) => void;
  /**
   * Where this item's collection publishes its components. Given a kind this
   * client can resolve a view within, the item can be read inside the component
   * that ships it — with this surface's own targets, pending edits included —
   * rather than only as a document.
   *
   * A kind it cannot resolve offers no in-context reading. Guessing at a URL
   * shape would put an empty iframe in front of a reviewer with nothing to say
   * why it is empty.
   */
  preview?: CollectionPreview;
  /**
   * The collection this item belongs to. In-context reading needs it: the story
   * index is read through this project's own API, per collection, because the
   * declared host is the collection's.
   */
  collectionId?: string;
  /**
   * Whether THIS item has a story that renders it (`useInContextItems`).
   *
   * A collection declaring a host is a claim about the collection, not about
   * every item in it: a Storybook renders what someone wrote a story for. Until
   * the two were told apart, "In context" was offered on every item and answered
   * "No story renders this item's components" on the ones without — an offer
   * made and then withdrawn, which reads as the feature being broken rather than
   * as a story being unwritten.
   *
   * The list resolving this already has the index in hand, so the toggle is
   * offered only where it leads somewhere. Omitted, the toggle is offered
   * whenever the collection has a host — the older behaviour, for a caller that
   * cannot answer per item.
   */
  hasStory?: boolean;
}

/**
 * FilePreview reads one item without opening an editor: clicking a file in a
 * list shows the document here, and the editors are reached from the footer.
 *
 * The document is the editors' own rendering (`DocumentPreview`), so a format
 * whose reader supplies its own HTML shows that, and every other format is laid
 * out from the content model by the shared preview kit. Nothing is rendered
 * twice and nothing is rendered here that an editor would render differently.
 *
 * Navigation is the consumer's: this package knows no routes, so each action is
 * a callback and an action left unset is not offered.
 */
export function FilePreview({
  projectId,
  itemName,
  format,
  sourcePath,
  targetLocales = [],
  sourceLocale,
  onClose,
  onOpenTranslate,
  onOpenReview,
  onOpenPreProcess,
  preview,
  collectionId,
  hasStory = true,
}: FilePreviewProps) {
  const { getDisplayName } = useLocales();
  const [side, setSide] = useState<"source" | "target">("source");
  const [reading, setReading] = useState<ReadingMode>("document");
  const [picked, setPicked] = useState<string | undefined>();

  // The chosen locale, held only while it stays one the project targets — a
  // project switch must not leave the reading pinned to a locale it dropped.
  const locale = picked && targetLocales.includes(picked) ? picked : (targetLocales[0] ?? "");
  const hasTargets = targetLocales.length > 0;
  // Every part must be present or the reading cannot happen: a host this client
  // can resolve a view within, the collection whose API serves that host's
  // index, an embed origin to frame it through — and a story that renders THIS
  // item, because a collection's host is a claim about the collection and not
  // about every file in it. Offering the toggle without one of them puts an
  // empty frame, or an apology, in front of a reviewer who asked to read.
  const storybookURL =
    collectionId && embedOrigin() && hasStory ? storybookHost(preview) : undefined;
  // With one target locale the toggle names it outright; with several the
  // selector names it, and the toggle only says which side is being read.
  const targetLabel = targetLocales.length === 1 ? getDisplayName(locale) : "Target";

  return (
    <Sheet open={!!itemName} onOpenChange={(open) => !open && onClose()}>
      {/* Half the viewport on a wide screen, and progressively wider as it
          narrows — the same proportions the desktop app's preview uses. The
          data-[side] variants outrank the Sheet's own width defaults. */}
      <SheetContent
        side="right"
        className="gap-3 data-[side=right]:w-full data-[side=right]:sm:w-3/4 data-[side=right]:sm:max-w-none data-[side=right]:lg:w-1/2"
        data-testid="file-preview"
      >
        <SheetHeader className="pb-0">
          <SheetTitle className="flex flex-wrap items-center gap-2 text-sm">
            {/* The source when there is one, because that is the file a
                reviewer recognises; the item's own name below still says what
                is being addressed, so nothing is hidden by being demoted. */}
            <span className="min-w-0 break-all font-mono" translate="no">
              {sourcePath || itemName}
            </span>
            {format && <Badge variant="secondary">{format}</Badge>}
          </SheetTitle>
          {sourcePath && (
            <p
              className="min-w-0 break-all font-mono text-xs text-muted-foreground"
              translate="no"
              data-testid="file-preview-item-name"
            >
              {itemName}
            </p>
          )}
          <SheetDescription>Read the document, then open it in an editor.</SheetDescription>
        </SheetHeader>

        {(hasTargets || storybookURL) && (
          <div className="flex flex-wrap items-center gap-2 px-4">
            {hasTargets && (
              <ToggleGroup
                type="single"
                variant="outline"
                size="sm"
                value={side}
                onValueChange={(value) => value && setSide(value as "source" | "target")}
                data-testid="file-preview-side"
              >
                <ToggleGroupItem value="source">Source</ToggleGroupItem>
                <ToggleGroupItem value="target">{targetLabel}</ToggleGroupItem>
              </ToggleGroup>
            )}
            {/* Which language is one question; what it is read inside is
                another. A document is the file; in context is the component
                that ships it, at the width it lays out to. */}
            {storybookURL && (
              <ToggleGroup
                type="single"
                variant="outline"
                size="sm"
                value={reading}
                onValueChange={(value) => value && setReading(value as ReadingMode)}
                data-testid="file-preview-reading"
              >
                <ToggleGroupItem value="document">Document</ToggleGroupItem>
                <ToggleGroupItem value="context">In context</ToggleGroupItem>
              </ToggleGroup>
            )}
            {targetLocales.length > 1 && (
              <LocaleSelect
                value={locale}
                onChange={setPicked}
                codes={targetLocales}
                className="w-48"
                data-testid="file-preview-locale"
              />
            )}
          </div>
        )}

        {/* Keyed by item: opening a different file starts a fresh read rather
            than showing the previous document until the new one arrives. */}
        <div className="min-h-0 flex-1 px-4">
          {itemName && reading === "context" && storybookURL ? (
            <ItemStoryPreview
              key={itemName}
              projectId={projectId}
              collectionId={collectionId ?? ""}
              itemName={itemName}
              storybookURL={storybookURL}
              locale={locale}
              source={side === "source"}
            />
          ) : (
            itemName && (
              <DocumentPreview
                key={itemName}
                projectId={projectId}
                itemName={itemName}
                sourceLocale={sourceLocale}
                targetLocale={locale}
                previewContentMode={side}
                onBlockSelect={() => {}}
              />
            )
          )}
        </div>

        {itemName && (onOpenTranslate || onOpenReview || onOpenPreProcess) && (
          <SheetFooter className="flex-row flex-wrap gap-2">
            {onOpenTranslate && (
              <Button
                size="sm"
                onClick={() => onOpenTranslate(itemName)}
                data-testid="file-preview-translate"
              >
                Open in Translate
              </Button>
            )}
            {onOpenReview && (
              <Button
                size="sm"
                variant="outline"
                onClick={() => onOpenReview(itemName)}
                data-testid="file-preview-review"
              >
                Review
              </Button>
            )}
            {onOpenPreProcess && (
              <Button
                size="sm"
                variant="outline"
                onClick={() => onOpenPreProcess(itemName)}
                data-testid="file-preview-pre-process"
              >
                Pre-process
              </Button>
            )}
          </SheetFooter>
        )}
      </SheetContent>
    </Sheet>
  );
}
