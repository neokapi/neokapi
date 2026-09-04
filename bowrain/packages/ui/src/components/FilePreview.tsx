import { useState } from "react";
import { Button } from "@neokapi/ui-primitives";
import { FilePreview as PreviewSheet, type FilePreviewView } from "@neokapi/ui-primitives/preview";
import { t } from "@neokapi/i18n-react/runtime";
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
 * The sheet around it is the shared one (`@neokapi/ui-primitives/preview`), the
 * same panel kapi desktop reads a file in. What the platform adds is its own:
 * the document is the editors' own rendering (`DocumentPreview`), so a format
 * whose reader supplies its own HTML shows that, and every other format is laid
 * out from the content model by the shared preview kit. In-context reading puts
 * the item inside the component that ships it. Nothing is rendered twice and
 * nothing is rendered here that an editor would render differently.
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
  const targetLabel = targetLocales.length === 1 ? getDisplayName(locale) : undefined;

  // Which language is one question; what it is read inside is another. A
  // document is the file; in context is the component that ships it, at the
  // width it lays out to. Each reading is keyed by item, so opening a different
  // file starts a fresh read rather than showing the previous document until the
  // new one arrives.
  const views: FilePreviewView[] = [
    {
      value: "document",
      label: t("Document"),
      content: itemName ? (
        <DocumentPreview
          key={itemName}
          projectId={projectId}
          itemName={itemName}
          sourceLocale={sourceLocale}
          targetLocale={locale}
          previewContentMode={side}
          onBlockSelect={() => {}}
        />
      ) : null,
    },
  ];
  if (storybookURL) {
    views.push({
      value: "context",
      label: t("In context"),
      content: itemName ? (
        <ItemStoryPreview
          key={itemName}
          projectId={projectId}
          collectionId={collectionId ?? ""}
          itemName={itemName}
          storybookURL={storybookURL}
          locale={locale}
          source={side === "source"}
        />
      ) : null,
    });
  }

  const name = itemName;
  const actions =
    name && (onOpenTranslate || onOpenReview || onOpenPreProcess) ? (
      <>
        {onOpenTranslate && (
          <Button
            size="sm"
            onClick={() => onOpenTranslate(name)}
            data-testid="file-preview-translate"
          >
            Open in Translate
          </Button>
        )}
        {onOpenReview && (
          <Button
            size="sm"
            variant="outline"
            onClick={() => onOpenReview(name)}
            data-testid="file-preview-review"
          >
            Review
          </Button>
        )}
        {onOpenPreProcess && (
          <Button
            size="sm"
            variant="outline"
            onClick={() => onOpenPreProcess(name)}
            data-testid="file-preview-pre-process"
          >
            Pre-process
          </Button>
        )}
      </>
    ) : undefined;

  return (
    <PreviewSheet
      open={!!itemName}
      onClose={onClose}
      data-testid="file-preview"
      // The source when there is one, because that is the file a reviewer
      // recognises; the item's own name below still says what is being
      // addressed, so nothing is hidden by being demoted.
      filename={sourcePath || itemName || ""}
      subtitle={sourcePath ? (itemName ?? undefined) : undefined}
      subtitleTestId="file-preview-item-name"
      format={format}
      description="Read the document, then open it in an editor."
      sides={
        hasTargets
          ? { value: side, onChange: setSide, targetLabel, "data-testid": "file-preview-side" }
          : undefined
      }
      views={views}
      view={reading}
      onViewChange={(value) => setReading(value as ReadingMode)}
      viewsLabel={t("Reading")}
      viewsTestId="file-preview-reading"
      toolbar={
        targetLocales.length > 1 ? (
          <LocaleSelect
            value={locale}
            onChange={setPicked}
            codes={targetLocales}
            className="w-48"
            data-testid="file-preview-locale"
          />
        ) : undefined
      }
      scrollBody={false}
      actions={actions}
    />
  );
}
