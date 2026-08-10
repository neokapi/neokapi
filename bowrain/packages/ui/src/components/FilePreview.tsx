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
import { useLocales } from "../hooks/useLocales";
import { DocumentPreview } from "./editor/DocumentPreview";
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
  /** The project's target locales, in the order the project declares them. */
  targetLocales?: string[];
  /** Dismiss — Escape, the close button, or a click outside. */
  onClose: () => void;
  /** Open the item in the translation editor. */
  onOpenTranslate?: (itemName: string) => void;
  /** Open the item in review. */
  onOpenReview?: (itemName: string) => void;
  /** Open the item's pre-processing surface. */
  onOpenPreProcess?: (itemName: string) => void;
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
  targetLocales = [],
  onClose,
  onOpenTranslate,
  onOpenReview,
  onOpenPreProcess,
}: FilePreviewProps) {
  const { getDisplayName } = useLocales();
  const [side, setSide] = useState<"source" | "target">("source");
  const [picked, setPicked] = useState<string | undefined>();

  // The chosen locale, held only while it stays one the project targets — a
  // project switch must not leave the reading pinned to a locale it dropped.
  const locale = picked && targetLocales.includes(picked) ? picked : (targetLocales[0] ?? "");
  const hasTargets = targetLocales.length > 0;
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
            <span className="min-w-0 break-all font-mono" translate="no">
              {itemName}
            </span>
            {format && <Badge variant="secondary">{format}</Badge>}
          </SheetTitle>
          <SheetDescription>Read the document, then open it in an editor.</SheetDescription>
        </SheetHeader>

        {hasTargets && (
          <div className="flex flex-wrap items-center gap-2 px-4">
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
          {itemName && (
            <DocumentPreview
              key={itemName}
              projectId={projectId}
              itemName={itemName}
              targetLocale={locale}
              previewContentMode={side}
              onBlockSelect={() => {}}
            />
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
