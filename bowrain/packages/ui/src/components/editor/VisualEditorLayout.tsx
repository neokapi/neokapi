import { Button, Tabs, TabsList, TabsTrigger, cn } from "@neokapi/ui-primitives";
import { useState, useCallback, useEffect, useRef } from "react";
import type {
  ProjectInfo,
  BlockInfo,
  TMMatchInfo,
  BlockTermMatch,
  BlockNote,
  QAIssue,
  BlockHistoryEntry,
  FileQAResult,
  AddConceptRequest,
} from "../../types/api";
import type { VisualEditorMode, PreviewContentMode } from "./visual-editor-types";
import { DocumentPreview } from "./DocumentPreview";
import { VisualEditorCard } from "./VisualEditorCard";
import type { UnifiedSaveResult } from "../UnifiedTargetEditor";
import { TermSidebar } from "./TermSidebar";
import { ProblemsPanel } from "./ProblemsPanel";
import { useVisualEditorKeyboard } from "./useVisualEditorKeyboard";
import { AlertTriangle } from "../icons";

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface VisualEditorLayoutProps {
  project: ProjectInfo;
  fileName: string;
  blocks: BlockInfo[];
  selectedIndex: number;
  editingIndex: number | null;
  targetLocale: string;
  onNavigate: (index: number) => void;
  onStartEditing: () => void;
  onSave: (result: UnifiedSaveResult) => void | Promise<void>;
  onCancelEditing: () => void;
  onApprove: () => void;
  onReject: () => void;
  tmMatches: TMMatchInfo[];
  termMatches: BlockTermMatch[];
  onApplyTM: (index: number) => void;
  onInsertTerm: (text: string) => void;
  presenceSlot?: React.ReactNode;
  // QA
  qaIssues?: QAIssue[];
  fileQAResults?: FileQAResult[];
  qaLoading?: boolean;
  onRunFileQA?: () => void;
  // Block history
  history?: BlockHistoryEntry[];
  onRevertHistory?: (entry: BlockHistoryEntry) => void;
  // Block notes (enrich mode)
  notes?: BlockNote[];
  onAddNote?: (text: string) => void;
  onDeleteNote?: (noteId: string) => void;
  // Term creation (enrich mode)
  onTermCreate?: (req: AddConceptRequest) => Promise<void>;
}

// ---------------------------------------------------------------------------
// LocalStorage helpers for reference locales
// ---------------------------------------------------------------------------

const REFLOCALE_KEY_PREFIX = "visual-editor-reflocales-";

function loadReferenceLocales(projectId: string): string[] {
  try {
    const raw = localStorage.getItem(REFLOCALE_KEY_PREFIX + projectId);
    if (raw) return JSON.parse(raw);
  } catch {
    // ignore
  }
  return [];
}

function saveReferenceLocales(projectId: string, locales: string[]): void {
  try {
    localStorage.setItem(REFLOCALE_KEY_PREFIX + projectId, JSON.stringify(locales));
  } catch {
    // ignore
  }
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function VisualEditorLayout({
  project,
  fileName,
  blocks,
  selectedIndex,
  editingIndex,
  targetLocale,
  onNavigate,
  onStartEditing,
  onSave,
  onCancelEditing,
  onApprove,
  onReject,
  tmMatches,
  termMatches,
  onApplyTM,
  onInsertTerm,
  presenceSlot,
  qaIssues,
  fileQAResults,
  qaLoading,
  onRunFileQA,
  history,
  onRevertHistory,
  notes,
  onAddNote,
  onDeleteNote,
  onTermCreate,
}: VisualEditorLayoutProps) {
  // ── Visual-only view state (owned here, not lifted) ────────────────────
  const [editorMode, setEditorMode] = useState<VisualEditorMode>("translate");
  const [previewContentMode, setPreviewContentMode] = useState<PreviewContentMode>("source");

  // ── Reference locales (persisted per project) ──────────────────────────
  const [referenceLocales, setReferenceLocales] = useState<string[]>(() =>
    loadReferenceLocales(project.id),
  );
  const [showRefPicker, setShowRefPicker] = useState(false);
  const [showProblemsPanel, setShowProblemsPanel] = useState(false);

  // ── Inline card positioning state ──────────────────────────────────────
  const [spacerY, setSpacerY] = useState(0);
  const [cardHeight, setCardHeight] = useState(0);
  const cardRef = useRef<HTMLDivElement>(null);
  const scrollRef = useRef<HTMLDivElement>(null);

  // Measure card height via ResizeObserver
  useEffect(() => {
    const el = cardRef.current;
    if (!el) return;
    const ro = new ResizeObserver((entries) => {
      for (const entry of entries) {
        setCardHeight(entry.contentRect.height);
      }
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  // Spacer height = card height + padding above/below
  const spacerHeight = cardHeight > 0 ? cardHeight + 64 : 400;

  useEffect(() => {
    saveReferenceLocales(project.id, referenceLocales);
  }, [project.id, referenceLocales]);

  // ── Derived state ──────────────────────────────────────────────────────
  const currentBlock = blocks[selectedIndex];
  const showTermSidebar = termMatches.length > 0 || editorMode === "enrich";

  // Available reference locales = target locales minus the active target locale
  const availableRefLocales = project.target_languages.filter((l) => l !== targetLocale);

  const toggleRefLocale = useCallback((locale: string) => {
    setReferenceLocales((prev) =>
      prev.includes(locale) ? prev.filter((l) => l !== locale) : [...prev, locale].slice(0, 2),
    );
  }, []);

  // Total file QA issue count for badge
  const fileQAIssueCount = fileQAResults
    ? fileQAResults.reduce((acc, r) => acc + r.issues.length, 0)
    : 0;

  // ── Block selection from preview ───────────────────────────────────────
  const handleBlockSelect = useCallback(
    (blockId: string) => {
      const idx = blocks.findIndex((b) => b.id === blockId);
      if (idx >= 0) onNavigate(idx);
    },
    [blocks, onNavigate],
  );

  // ── Navigate to block from problems panel ──────────────────────────────
  const handleNavigateToBlock = useCallback(
    (blockId: string) => {
      const idx = blocks.findIndex((b) => b.id === blockId);
      if (idx >= 0) onNavigate(idx);
    },
    [blocks, onNavigate],
  );

  // ── Scroll card into view on navigation ────────────────────────────────
  useEffect(() => {
    if (cardRef.current) {
      cardRef.current.scrollIntoView({ block: "nearest", behavior: "smooth" });
    }
  }, [selectedIndex]);

  // ── Spacer position callback from DocumentPreview ──────────────────────
  const handleSpacerPosition = useCallback((y: number) => {
    setSpacerY(y);
  }, []);

  // ── Keyboard hook ──────────────────────────────────────────────────────
  const handleSaveAndNext = useCallback(() => {
    // Save is handled by TargetCellEditor inside VisualEditorCard
  }, []);

  useVisualEditorKeyboard({
    blocks,
    selectedIndex,
    editingIndex,
    onNavigate,
    onStartEditing,
    onCancelEditing,
    onSaveAndNext: handleSaveAndNext,
    onApprove,
    onReject,
    enabled: true,
  });

  // ── Render ─────────────────────────────────────────────────────────────
  if (!currentBlock) {
    return (
      <div className="relative w-full h-full flex items-center justify-center text-muted-foreground text-sm">
        No blocks to display
      </div>
    );
  }

  return (
    <div
      ref={scrollRef}
      className="relative w-full h-full overflow-y-auto flex flex-col"
      data-testid="visual-editor-layout"
    >
      {/* ── Sticky toolbar ───────────────────────────────────────── */}
      <div
        className={cn(
          "sticky top-0 z-20 flex items-center gap-2 px-4 py-2 bg-background",
          showTermSidebar ? "mr-[260px]" : "",
        )}
      >
        {/* Presence avatars (left) */}
        {presenceSlot && <div className="mr-auto">{presenceSlot}</div>}

        {!presenceSlot && <div className="flex-1" />}

        {/* QA check / Problems toggle */}
        {onRunFileQA && (
          <Button
            size="sm"
            variant="ghost"
            className={cn(
              "h-7 text-[11px] px-2",
              showProblemsPanel && "bg-primary/15 text-primary",
            )}
            onClick={() => {
              if (!showProblemsPanel && fileQAResults === undefined) onRunFileQA();
              setShowProblemsPanel((v) => !v);
            }}
            data-testid="problems-toggle"
          >
            <AlertTriangle className="w-3 h-3 mr-1" />
            Problems
            {fileQAIssueCount > 0 && (
              <span className="ml-1 text-[10px] px-1 rounded-full bg-destructive/15 text-destructive font-bold">
                {fileQAIssueCount}
              </span>
            )}
          </Button>
        )}

        {/* Reference locale picker */}
        {availableRefLocales.length > 0 && (
          <div className="relative">
            <Button
              size="sm"
              variant="ghost"
              className={cn(
                "h-7 text-[11px] px-2",
                referenceLocales.length > 0 && "bg-primary/15 text-primary",
              )}
              onClick={() => setShowRefPicker((v) => !v)}
              data-testid="ref-locale-toggle"
            >
              Ref
              {referenceLocales.length > 0 && (
                <span className="ml-1 text-[10px]">({referenceLocales.length})</span>
              )}
            </Button>
            {showRefPicker && (
              <div className="absolute top-full right-0 mt-1 w-48 p-2 rounded-md border border-border bg-card shadow-lg z-20">
                <div className="text-[10px] font-bold text-muted-foreground uppercase tracking-wider mb-1.5">
                  Reference Languages (max 2)
                </div>
                {availableRefLocales.map((locale) => (
                  <button
                    key={locale}
                    type="button"
                    onClick={() => toggleRefLocale(locale)}
                    className={cn(
                      "w-full text-left px-2 py-1 rounded text-xs hover:bg-muted/60 transition-colors",
                      referenceLocales.includes(locale) && "bg-primary/10 text-primary font-medium",
                    )}
                    data-testid={`ref-locale-${locale}`}
                  >
                    {locale}
                    {referenceLocales.includes(locale) && " ✓"}
                  </button>
                ))}
              </div>
            )}
          </div>
        )}

        {/* Preview content mode toggle */}
        <Tabs
          value={previewContentMode}
          onValueChange={(v: string) => setPreviewContentMode(v as PreviewContentMode)}
        >
          <TabsList className="h-7">
            <TabsTrigger value="source" className="text-[11px] px-2 h-6">
              Source
            </TabsTrigger>
            <TabsTrigger value="target" className="text-[11px] px-2 h-6">
              Target
            </TabsTrigger>
            <TabsTrigger value="pseudo" className="text-[11px] px-2 h-6">
              Pseudo
            </TabsTrigger>
          </TabsList>
        </Tabs>
      </div>

      {/* ── Preview + inline card container ───────────────────────── */}
      <div
        className="relative flex-1"
        style={{
          marginRight: showTermSidebar ? 260 : 0,
          minHeight: cardHeight > 0 ? spacerY + cardHeight + 96 : undefined,
        }}
      >
        <DocumentPreview
          projectId={project.id}
          itemName={fileName}
          targetLocale={targetLocale}
          selectedBlockId={currentBlock.id}
          onBlockSelect={handleBlockSelect}
          blocks={blocks}
          previewContentMode={previewContentMode}
          spacerHeight={spacerHeight}
          onSpacerPosition={handleSpacerPosition}
        />

        {/* ── Inline editor card ────────────────────────────────── */}
        <div
          ref={cardRef}
          className="absolute z-10 left-0 right-0 px-4 transition-[top] duration-[250ms] ease-in-out"
          style={{ top: spacerY + 32 }}
        >
          <VisualEditorCard
            block={currentBlock}
            blockIndex={selectedIndex}
            totalBlocks={blocks.length}
            targetLocale={targetLocale}
            editorMode={editorMode}
            onEditorModeChange={setEditorMode}
            isEditing={editingIndex !== null}
            onStartEditing={onStartEditing}
            onSave={onSave}
            onCancel={onCancelEditing}
            onApprove={onApprove}
            onReject={onReject}
            tmMatches={tmMatches}
            termMatches={termMatches}
            onApplyTM={onApplyTM}
            onInsertTerm={onInsertTerm}
            referenceLocales={referenceLocales}
            project={project}
            qaIssues={qaIssues}
            history={history}
            onRevertHistory={onRevertHistory}
            notes={notes}
            onAddNote={onAddNote}
            onDeleteNote={onDeleteNote}
            onTermCreate={onTermCreate}
            onPrev={selectedIndex > 0 ? () => onNavigate(selectedIndex - 1) : undefined}
            onNext={
              selectedIndex < blocks.length - 1 ? () => onNavigate(selectedIndex + 1) : undefined
            }
          />
        </div>
      </div>

      {/* ── Term sidebar (right side, fixed) ──────────────────────── */}
      {showTermSidebar && (
        <div className="fixed top-0 right-0 h-full z-10 w-[260px]">
          <TermSidebar
            termMatches={termMatches}
            onInsertTerm={onInsertTerm}
            editorMode={editorMode}
          />
        </div>
      )}

      {/* ── Problems panel (bottom overlay, fixed) ────────────────── */}
      {showProblemsPanel && (
        <ProblemsPanel
          issues={fileQAResults || []}
          loading={qaLoading}
          onNavigateToBlock={handleNavigateToBlock}
          onClose={() => setShowProblemsPanel(false)}
        />
      )}
    </div>
  );
}
