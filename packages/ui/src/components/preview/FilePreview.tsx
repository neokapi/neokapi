import * as React from "react";
import { ArrowLeft, FileWarning, Loader2 } from "lucide-react";
import { t } from "@neokapi/i18n-react/runtime";
import { cn } from "../../lib/utils";
import { Badge } from "../ui/badge";
import { Button } from "../ui/button";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "../ui/sheet";
import { ViewTab, ViewTabGroup } from "../ui/view-tab";
import DocumentViewer, { type DocumentViewerProps } from "./DocumentViewer";
import type { BlockAttrs } from "./FormatPreview";
import type { PreviewHighlights } from "./highlights";
import type { ContentNode, ContentTree } from "./types";

// FilePreview is the preview sheet every surface reads one file in: a right-hand
// panel titled by the file, with the readings it offers on a strip under the
// header, the document in a scroll container, and the explicit handoffs in a
// footer. kapi desktop drives it from the native engine's ContentTree; the
// bowrain platform renders its own body inside it and adds its story readings
// and editor actions. The shell, the reading switches and the states are here so
// the two surfaces cannot drift apart.
//
// What loads the content stays with the host: this component takes a document,
// or a body, plus whatever the host knows about loading and failing.

/** One reading of the open file, shown as a tab on the strip under the header. */
export interface FilePreviewView {
  /** Identifier for this reading. */
  value: string;
  /** The tab's label. */
  label: React.ReactNode;
  /** What the body shows while this reading is active. */
  content: React.ReactNode;
  "data-testid"?: string;
}

/** The language side being read, when the host offers both. */
export interface FilePreviewSides {
  value: "source" | "target";
  onChange: (side: "source" | "target") => void;
  /** What the target side is called: a language name, or "Target" by default. */
  targetLabel?: React.ReactNode;
  "data-testid"?: string;
}

export interface FilePreviewProps {
  /** Whether the sheet is open. */
  open: boolean;
  /** Escape, the close button, or a click outside. */
  onClose: () => void;

  /** The file, in mono, as the title. */
  filename: string;
  /**
   * A second mono line under the title. A surface titling the sheet by the file
   * an item was lifted out of names the item itself here.
   */
  subtitle?: string;
  subtitleTestId?: string;
  /** The file's format id, badged beside the title. */
  format?: string;
  /** The line under the title saying what this sheet is for. */
  description: React.ReactNode;
  /** Anything the host draws inside the header, under the description. */
  headerExtra?: React.ReactNode;

  /** A Source / Target switch on the reading strip. */
  sides?: FilePreviewSides;
  /** The readings on offer. A single reading shows no tabs. */
  views?: readonly FilePreviewView[];
  /** The active reading; the first one when unset. */
  view?: string;
  onViewChange?: (value: string) => void;
  /** Names the reading choice for a screen reader. */
  viewsLabel?: string;
  viewsTestId?: string;
  /** Host controls beside the tabs: a locale selector, a filter. */
  toolbar?: React.ReactNode;

  /**
   * The document to render, when the host has one in the shared content model.
   * Used only when no view and no children supply a body.
   */
  tree?: ContentTree | null;
  /** The rest of what this host wants from the viewer for that document. */
  viewer?: Omit<
    DocumentViewerProps,
    "tree" | "filename" | "blockAttrs" | "selectedBlockId" | "highlights"
  >;
  /**
   * Spans to mark on the document, by block id: a check finding's run anchor on
   * the side it addresses, in the finding's tone. A host arriving from a list
   * of findings passes the file's findings here, with the one it came for in
   * focus and the rest dimmed.
   */
  highlights?: PreviewHighlights;
  /** A body the host renders itself, instead of `tree`. */
  children?: React.ReactNode;

  loading?: boolean;
  /** What the loading line says. Defaults to naming the file. */
  loadingLabel?: React.ReactNode;
  error?: string | null;
  /** Drawn when nothing is loading, nothing failed, and there is no body. */
  empty?: React.ReactNode;
  /** The body scrolls inside the sheet. Unset it for a body that fills instead. */
  scrollBody?: boolean;

  /**
   * Open the document at one unit: the block with this key is marked, scrolled
   * into view, and named above the body. The key is the one a queue addresses a
   * unit by (`convergence.BlockKey`).
   */
  focusKey?: string | null;
  /**
   * Review state per unit key, drawn on each block as `data-review-state` and a
   * marker class, so the reader sees where the decisions stand across the file.
   */
  unitStates?: Record<string, string>;
  /**
   * What is said about the focused unit beside its key: a finding's severity and
   * message, say. Drawn in the focus row, before the review state when a host
   * supplies both.
   */
  focusNote?: React.ReactNode;
  /** Label for the button that returns the reader where they came from. */
  backLabel?: string;
  /** Where that button goes. Closes the sheet when unset. */
  onBack?: () => void;

  /** The explicit handoffs, in a footer. */
  actions?: React.ReactNode;
  className?: string;
  "data-testid"?: string;
}

// unitKeyOf is the engine's own key for a block (convergence.BlockKey): the name
// a reader gave it, else its id. A queue addresses a unit by that key, so this
// is what maps a queue row onto a node in the rendered document.
function unitKeyOf(node: ContentNode): string {
  return node.name || node.id;
}

/** The tree's translatable blocks, indexed by the key a queue addresses them by. */
function blockNodesByUnitKey(tree: ContentTree | null | undefined): Map<string, ContentNode> {
  const out = new Map<string, ContentNode>();
  if (!tree) return out;
  const walk = (n: ContentNode) => {
    if (n.kind === "block") {
      const key = unitKeyOf(n);
      if (key && !out.has(key)) out.set(key, n);
    }
    n.children?.forEach(walk);
  };
  tree.root.forEach(walk);
  return out;
}

export default function FilePreview({
  open,
  onClose,
  filename,
  subtitle,
  subtitleTestId,
  format,
  description,
  headerExtra,
  sides,
  views,
  view,
  onViewChange,
  viewsLabel,
  viewsTestId,
  toolbar,
  tree,
  viewer,
  children,
  loading = false,
  loadingLabel,
  error,
  empty,
  scrollBody = true,
  highlights,
  focusKey,
  unitStates,
  focusNote,
  backLabel,
  onBack,
  actions,
  className,
  "data-testid": testID,
}: FilePreviewProps) {
  const activeView =
    views && views.length > 0 ? (views.find((v) => v.value === view) ?? views[0]) : undefined;

  // The focused unit's node, and the id every block is addressed by inside the
  // rendered document.
  const nodesByKey = React.useMemo(() => blockNodesByUnitKey(tree), [tree]);
  const focusNode = focusKey ? nodesByKey.get(focusKey) : undefined;
  const focusID = focusNode?.id;

  // Review state per block id, so one lookup answers for the whole document.
  const statesByBlockID = React.useMemo(() => {
    const out = new Map<string, string>();
    if (!unitStates) return out;
    for (const [key, node] of nodesByKey) {
      const state = unitStates[key];
      if (state) out.set(node.id, state);
    }
    return out;
  }, [nodesByKey, unitStates]);

  const blockAttrs = React.useCallback(
    (id: string): BlockAttrs | undefined => {
      const state = statesByBlockID.get(id);
      const focused = id === focusID;
      if (!state && !focused) return undefined;
      return {
        className: focused
          ? "ring-2 ring-primary/60 rounded-sm"
          : "underline decoration-dotted decoration-amber-500/70 underline-offset-4",
        "data-review-state": state,
        "data-review-focus": focused ? "true" : undefined,
      };
    },
    [statesByBlockID, focusID],
  );

  // Scroll the focused block into view once the document has rendered. The body
  // is its own scroll container, so the block is centred inside it.
  const bodyRef = React.useRef<HTMLDivElement | null>(null);
  React.useEffect(() => {
    if (!focusID || loading || error || !tree) return;
    const el = bodyRef.current?.querySelector<HTMLElement>(
      `[data-block-id="${CSS.escape(focusID)}"]`,
    );
    el?.scrollIntoView({ block: "center", behavior: "auto" });
  }, [focusID, loading, error, tree]);

  const focusState = focusKey && unitStates ? unitStates[focusKey] : undefined;
  // The focus row names the unit the reader came for and offers the way back.
  // A host that opens the whole file from a list still gets the way back and
  // its note, with no unit named.
  const hasFocusRow = !!focusKey || !!backLabel || focusNote !== undefined;
  const hasStrip = !!sides || (!!views && views.length > 1) || !!toolbar;

  let body: React.ReactNode = null;
  if (activeView) body = activeView.content;
  else if (children) body = children;
  else if (tree)
    body = (
      <DocumentViewer
        {...viewer}
        tree={tree}
        filename={filename}
        selectedBlockId={focusID}
        blockAttrs={blockAttrs}
        highlights={highlights}
      />
    );

  return (
    <Sheet open={open} onOpenChange={(next) => !next && onClose()}>
      {/* Half the viewport on a wide screen, and progressively wider as it
          narrows. The data-[side] variants outrank the Sheet's own width
          defaults. */}
      <SheetContent
        side="right"
        className={cn(
          "gap-3 data-[side=right]:w-full data-[side=right]:sm:w-3/4 data-[side=right]:sm:max-w-none data-[side=right]:lg:w-1/2",
          className,
        )}
        data-testid={testID}
      >
        <SheetHeader className="pb-0">
          <SheetTitle className="flex flex-wrap items-center gap-2 text-sm">
            <span className="min-w-0 break-all font-mono" translate="no">
              {filename}
            </span>
            {format && <Badge variant="secondary">{format}</Badge>}
          </SheetTitle>
          {subtitle && (
            <p
              className="min-w-0 break-all font-mono text-xs text-muted-foreground"
              translate="no"
              data-testid={subtitleTestId}
            >
              {subtitle}
            </p>
          )}
          <SheetDescription>{description}</SheetDescription>
          {hasFocusRow && (
            <div
              className="flex flex-wrap items-center gap-2 pt-1 text-xs"
              data-slot="file-preview-focus"
            >
              {backLabel && (
                <Button
                  variant="outline"
                  size="xs"
                  onClick={onBack ?? onClose}
                  data-slot="file-preview-back"
                >
                  <ArrowLeft size={12} />
                  {backLabel}
                </Button>
              )}
              {focusKey && (
                <span className="font-mono text-[11px] text-muted-foreground" translate="no">
                  {focusKey}
                </span>
              )}
              {focusNote}
              {focusKey && unitStates && (
                <Badge variant="outline" className="text-[10px]">
                  {focusState ?? t("awaiting review")}
                </Badge>
              )}
              {focusKey && tree && !focusNode && (
                <span className="text-[11px] text-muted-foreground">
                  {t("This unit is not in the rendered document.")}
                </span>
              )}
            </div>
          )}
          {headerExtra}
        </SheetHeader>

        {hasStrip && (
          <div className="flex flex-wrap items-center gap-2 px-4">
            {sides && (
              <ViewTabGroup aria-label={t("Language side")} data-testid={sides["data-testid"]}>
                <ViewTab active={sides.value === "source"} onClick={() => sides.onChange("source")}>
                  {t("Source")}
                </ViewTab>
                <ViewTab active={sides.value === "target"} onClick={() => sides.onChange("target")}>
                  {sides.targetLabel ?? t("Target")}
                </ViewTab>
              </ViewTabGroup>
            )}
            {views && views.length > 1 && (
              <ViewTabGroup aria-label={viewsLabel} data-testid={viewsTestId}>
                {views.map((v) => (
                  <ViewTab
                    key={v.value}
                    active={v.value === activeView?.value}
                    onClick={() => onViewChange?.(v.value)}
                    data-testid={v["data-testid"]}
                  >
                    {v.label}
                  </ViewTab>
                ))}
              </ViewTabGroup>
            )}
            {toolbar}
          </div>
        )}

        <div
          ref={bodyRef}
          className={cn("min-h-0 flex-1 px-4", scrollBody && "overflow-auto pb-4")}
        >
          {loading && (
            <div className="flex items-center gap-2 py-8 text-sm text-muted-foreground">
              <Loader2 className="size-4 animate-spin" />
              {loadingLabel ?? <>Reading {filename}…</>}
            </div>
          )}
          {!loading && error && (
            <div className="flex items-center gap-2 py-8 text-sm text-destructive">
              <FileWarning className="size-4" />
              {error}
            </div>
          )}
          {!loading && !error && (body ?? empty)}
        </div>

        {actions && <SheetFooter className="flex-row flex-wrap gap-2">{actions}</SheetFooter>}
      </SheetContent>
    </Sheet>
  );
}
