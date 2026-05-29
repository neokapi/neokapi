import { Badge, Button, Tabs, TabsList, TabsTrigger, cn } from "@neokapi/ui-primitives";
import { useState, useCallback } from "react";
import type {
  ProjectInfo,
  BlockInfo,
  SpanInfo,
  TMMatchInfo,
  BlockTermMatch,
  BlockNote,
  QAIssue,
  BlockHistoryEntry,
  AddConceptRequest,
} from "../../types/api";
import type { VisualEditorMode } from "./visual-editor-types";
import { SourceCellDisplay } from "./SourceCellDisplay";
import { FormattedSourceDisplay } from "./FormattedSourceDisplay";
import { UnifiedTargetEditor, type UnifiedSaveResult } from "../UnifiedTargetEditor";
import { HighlightedSource } from "./HighlightedSource";
import { VisualEditorToolbar } from "./VisualEditorToolbar";
import { TermCreationPopover } from "./TermCreationPopover";
import { ContextPanel } from "./ContextPanel";
import { getBlockStatus, statusConfig } from "./blockStatus";
import { InlineCodeLegend } from "@neokapi/ui-primitives";
import { FormatVocabularyBadge } from "./FormatVocabularyBadge";
import {
  Check,
  X,
  ChevronDown,
  ChevronUp,
  ChevronRight,
  AlertTriangle,
  Info,
  Code,
} from "../icons";

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface VisualEditorCardProps {
  block: BlockInfo;
  blockIndex: number;
  totalBlocks: number;
  targetLocale: string;
  editorMode: VisualEditorMode;
  onEditorModeChange: (mode: VisualEditorMode) => void;
  isEditing: boolean;
  onStartEditing: () => void;
  onSave: (result: UnifiedSaveResult) => void | Promise<void>;
  onCancel: () => void;
  onApprove: () => void;
  onReject: () => void;
  tmMatches: TMMatchInfo[];
  termMatches: BlockTermMatch[];
  onApplyTM: (index: number) => void;
  onInsertTerm: (text: string) => void;
  referenceLocales?: string[];
  project: ProjectInfo;
  // QA
  qaIssues?: QAIssue[];
  // Block history
  history?: BlockHistoryEntry[];
  onRevertHistory?: (entry: BlockHistoryEntry) => void;
  // Block notes (enrich mode)
  notes?: BlockNote[];
  onAddNote?: (text: string) => void;
  onDeleteNote?: (noteId: string) => void;
  // Term creation (enrich mode)
  onTermCreate?: (req: AddConceptRequest) => Promise<void>;
  // Navigation
  onPrev?: () => void;
  onNext?: () => void;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function VisualEditorCard({
  block,
  blockIndex,
  totalBlocks,
  targetLocale,
  editorMode,
  onEditorModeChange,
  isEditing,
  onStartEditing,
  onSave,
  onCancel,
  onApprove,
  onReject,
  tmMatches,
  termMatches,
  onApplyTM,
  onInsertTerm: _onInsertTerm,
  referenceLocales,
  project,
  qaIssues,
  history,
  onRevertHistory,
  notes,
  onAddNote,
  onDeleteNote,
  onTermCreate,
  onPrev,
  onNext,
}: VisualEditorCardProps) {
  const [tmExpanded, setTmExpanded] = useState(true);
  const [historyExpanded, setHistoryExpanded] = useState(false);
  const [qaExpanded, setQaExpanded] = useState(false);
  const [notesExpanded, setNotesExpanded] = useState(true);
  const [noteText, setNoteText] = useState("");
  const [termPopoverOpen, setTermPopoverOpen] = useState(false);
  const [selectedSourceText, setSelectedSourceText] = useState("");
  const [codeView, setCodeView] = useState(false);
  const [showLegend, setShowLegend] = useState(false);

  const status = getBlockStatus(block, targetLocale);
  const sc = statusConfig[status];

  const sourceSpans = block.source_spans || [];
  const sourceCodedText = block.source_coded || block.source;
  const targetText = block.targets[targetLocale] || "";
  const targetCodedText = block.targets_coded?.[targetLocale] || block.targets[targetLocale] || "";
  const hasTargetSpans = block.has_spans && !!block.targets_coded?.[targetLocale];

  const qaErrors = qaIssues?.filter((i) => i.severity === "error") || [];
  const qaWarnings = qaIssues?.filter((i) => i.severity === "warning") || [];

  const handleInsertTag = useCallback(
    (_span: SpanInfo) => {
      if (!isEditing) onStartEditing();
    },
    [isEditing, onStartEditing],
  );

  const handleAddNote = useCallback(() => {
    if (!noteText.trim() || !onAddNote) return;
    onAddNote(noteText.trim());
    setNoteText("");
  }, [noteText, onAddNote]);

  const handleSourceMouseUp = useCallback(() => {
    if (editorMode !== "enrich") return;
    const sel = window.getSelection();
    const text = sel?.toString().trim();
    if (text && text.length > 0) {
      setSelectedSourceText(text);
      setTermPopoverOpen(true);
    }
  }, [editorMode]);

  const handleTermSubmit = useCallback(
    async (req: AddConceptRequest) => {
      if (onTermCreate) await onTermCreate(req);
    },
    [onTermCreate],
  );

  return (
    <div className="w-full rounded-xl border bg-card p-0" data-testid="visual-editor-card">
      {/* ── Header ─────────────────────────────────────────── */}
      <div className="flex items-center justify-between px-4 pt-3 pb-2">
        <div className="flex items-center gap-2">
          <div className="flex items-center gap-0.5">
            <button
              type="button"
              disabled={!onPrev || blockIndex === 0}
              onClick={onPrev}
              className="inline-flex items-center justify-center w-5 h-5 rounded text-muted-foreground hover:text-foreground hover:bg-muted-foreground/10 transition-colors disabled:opacity-30 disabled:pointer-events-none"
              title="Previous block"
              data-testid="prev-block-btn"
            >
              <ChevronUp className="w-3.5 h-3.5" />
            </button>
            <button
              type="button"
              disabled={!onNext || blockIndex >= totalBlocks - 1}
              onClick={onNext}
              className="inline-flex items-center justify-center w-5 h-5 rounded text-muted-foreground hover:text-foreground hover:bg-muted-foreground/10 transition-colors disabled:opacity-30 disabled:pointer-events-none"
              title="Next block"
              data-testid="next-block-btn"
            >
              <ChevronDown className="w-3.5 h-3.5" />
            </button>
          </div>
          <span className="text-xs font-medium text-muted-foreground">
            Block {blockIndex + 1}/{totalBlocks}
          </span>
          <Badge variant="secondary" className={cn("text-[10px] px-1.5 py-0 h-4", sc.className)}>
            {sc.label}
          </Badge>
          {/* QA issue badges */}
          {qaErrors.length > 0 && (
            <button
              type="button"
              onClick={() => setQaExpanded((v) => !v)}
              className="inline-flex items-center gap-0.5 text-[10px] font-bold text-destructive cursor-pointer bg-destructive/10 px-1.5 py-0 h-4 rounded"
              data-testid="qa-error-badge"
            >
              <AlertTriangle className="w-2.5 h-2.5" />
              {qaErrors.length}
            </button>
          )}
          {qaWarnings.length > 0 && (
            <button
              type="button"
              onClick={() => setQaExpanded((v) => !v)}
              className="inline-flex items-center gap-0.5 text-[10px] font-bold text-warning dark:text-warning cursor-pointer bg-warning/10 px-1.5 py-0 h-4 rounded"
              data-testid="qa-warning-badge"
            >
              <Info className="w-2.5 h-2.5" />
              {qaWarnings.length}
            </button>
          )}
          {/* Vocabulary badge showing inline tag summary */}
          {sourceSpans.length > 0 && (
            <FormatVocabularyBadge spans={sourceSpans} onClick={() => setShowLegend((v) => !v)} />
          )}
        </div>
        <Tabs
          value={editorMode}
          onValueChange={(v: string) => onEditorModeChange(v as VisualEditorMode)}
        >
          <TabsList className="h-7">
            <TabsTrigger value="translate" className="text-[11px] px-2 h-6">
              Translate
            </TabsTrigger>
            <TabsTrigger value="enrich" className="text-[11px] px-2 h-6">
              Enrich
            </TabsTrigger>
            <TabsTrigger value="review" className="text-[11px] px-2 h-6">
              Review
            </TabsTrigger>
          </TabsList>
        </Tabs>
      </div>

      {/* ── QA issues (expanded) ─────────────────────────── */}
      {qaExpanded && qaIssues && qaIssues.length > 0 && (
        <div className="px-4 pb-2" data-testid="qa-issues-list">
          <div className="rounded-md border border-border bg-muted/30 p-2 space-y-1">
            {qaIssues.map((issue, i) => (
              <div key={i} className="flex items-start gap-2 text-xs">
                {issue.severity === "error" ? (
                  <AlertTriangle className="w-3 h-3 text-destructive shrink-0 mt-0.5" />
                ) : (
                  <Info className="w-3 h-3 text-warning shrink-0 mt-0.5" />
                )}
                <span
                  className={
                    issue.severity === "error"
                      ? "text-destructive"
                      : "text-warning dark:text-warning"
                  }
                >
                  <span className="font-medium">{issue.type}:</span> {issue.message}
                </span>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* ── Toolbar (Translate mode, editing) ───────────── */}
      {editorMode === "translate" && sourceSpans.length > 0 && (
        <div className="px-4 pb-1">
          <VisualEditorToolbar
            sourceSpans={sourceSpans}
            onInsertTag={handleInsertTag}
            disabled={!isEditing}
          />
        </div>
      )}

      {/* ── Source ──────────────────────────────────────── */}
      <div className="px-4 py-2">
        <div
          className={cn(
            "visual-editor-source rounded-md px-3 py-2",
            editorMode === "enrich" && "cursor-text select-text",
          )}
          onMouseUp={handleSourceMouseUp}
        >
          <div className="flex items-center justify-between mb-1">
            <div className="text-[10px] font-bold text-muted-foreground uppercase tracking-wider">
              Source
              {editorMode === "enrich" && (
                <span className="ml-2 font-normal text-[9px] normal-case">
                  (select text to create term)
                </span>
              )}
            </div>
            {block.has_spans && (
              <button
                type="button"
                onClick={() => setCodeView((v) => !v)}
                className={cn(
                  "inline-flex items-center gap-0.5 text-[10px] px-1 py-0 h-4 rounded transition-colors",
                  codeView
                    ? "text-foreground bg-muted-foreground/15"
                    : "text-muted-foreground hover:text-foreground hover:bg-muted-foreground/10",
                )}
                title={codeView ? "Switch to formatted view" : "Switch to code view"}
                data-testid="code-view-toggle"
              >
                <Code className="w-3 h-3" />
              </button>
            )}
          </div>
          <div className="text-sm leading-relaxed">
            {termMatches.length > 0 || (block.entities && block.entities.length > 0) ? (
              <HighlightedSource
                text={block.source}
                termMatches={termMatches}
                entities={block.entities}
              />
            ) : block.has_spans && block.source_coded ? (
              codeView ? (
                <SourceCellDisplay
                  codedText={sourceCodedText}
                  spans={sourceSpans}
                  entities={block.entities}
                />
              ) : (
                <FormattedSourceDisplay codedText={sourceCodedText} spans={sourceSpans} />
              )
            ) : (
              <span>{block.source}</span>
            )}
          </div>
        </div>
      </div>

      {/* ── Inline code legend (expandable) ─────────────── */}
      {showLegend && sourceSpans.length > 0 && (
        <div className="px-4 pb-2" data-testid="inline-code-legend">
          <InlineCodeLegend spans={sourceSpans} onClose={() => setShowLegend(false)} />
        </div>
      )}

      {/* ── Term creation popover (enrich mode) ─────────── */}
      {editorMode === "enrich" && onTermCreate && (
        <TermCreationPopover
          open={termPopoverOpen}
          selectedText={selectedSourceText}
          sourceLocale={project.default_source_language}
          targetLocale={targetLocale}
          onSubmit={handleTermSubmit}
          onClose={() => setTermPopoverOpen(false)}
        />
      )}

      {/* ── Reference locales (optional) ───────────────── */}
      {referenceLocales && referenceLocales.length > 0 && (
        <div className="px-4 pb-2">
          {referenceLocales.map((refLocale) => {
            const refText = block.targets[refLocale];
            if (!refText) return null;
            return (
              <div
                key={refLocale}
                className="rounded-md px-3 py-1.5 bg-muted/50 border border-border mb-1"
              >
                <span className="text-[10px] font-bold text-muted-foreground uppercase tracking-wider mr-2">
                  {refLocale}
                </span>
                <span className="text-xs text-muted-foreground">{refText}</span>
              </div>
            );
          })}
        </div>
      )}

      {/* ── Target ─────────────────────────────────────── */}
      <div className="px-4 pb-2">
        <div className="text-[10px] font-bold text-muted-foreground uppercase tracking-wider mb-1">
          Target ({targetLocale})
        </div>
        {editorMode === "translate" && isEditing ? (
          <UnifiedTargetEditor
            block={block}
            locale={targetLocale}
            onSave={onSave}
            onCancel={onCancel}
          />
        ) : (
          <div
            className={cn(
              "text-sm leading-relaxed px-3 py-2 rounded-md border border-border bg-muted/30 min-h-[44px]",
              editorMode === "translate" &&
                !isEditing &&
                "cursor-pointer hover:bg-muted/60 transition-colors",
            )}
            onClick={editorMode === "translate" && !isEditing ? onStartEditing : undefined}
            data-testid="target-display"
          >
            {hasTargetSpans ? (
              <FormattedSourceDisplay codedText={targetCodedText} spans={sourceSpans} />
            ) : targetText ? (
              <span>{targetText}</span>
            ) : (
              <span className="text-muted-foreground italic text-xs">Click to translate...</span>
            )}
          </div>
        )}
      </div>

      {/* ── Block notes (enrich mode) ─────────────────── */}
      {editorMode === "enrich" && (
        <div className="px-4 pb-2" data-testid="block-notes-section">
          <button
            type="button"
            className="flex items-center gap-1 text-[11px] font-bold text-muted-foreground uppercase tracking-wider mb-2 hover:text-foreground transition-colors"
            onClick={() => setNotesExpanded((v) => !v)}
          >
            {notesExpanded ? (
              <ChevronDown className="w-3 h-3" />
            ) : (
              <ChevronRight className="w-3 h-3" />
            )}
            Notes
            {notes && notes.length > 0 && (
              <span className="font-normal text-[10px] ml-1">({notes.length})</span>
            )}
          </button>
          {notesExpanded && (
            <>
              {notes && notes.length > 0 && (
                <div className="space-y-1 mb-2">
                  {notes.map((note) => (
                    <div
                      key={note.id}
                      className="flex items-start gap-2 p-2 bg-muted rounded-md border border-border text-xs"
                    >
                      <div className="flex-1">
                        <div className="text-muted-foreground text-[10px] mb-0.5">
                          {note.author} &middot; {new Date(note.createdAt).toLocaleDateString()}
                        </div>
                        <div>{note.text}</div>
                      </div>
                      {onDeleteNote && (
                        <button
                          type="button"
                          onClick={() => onDeleteNote(note.id)}
                          className="text-muted-foreground hover:text-destructive transition-colors p-0.5"
                          data-testid={`delete-note-${note.id}`}
                        >
                          <X className="w-3 h-3" />
                        </button>
                      )}
                    </div>
                  ))}
                </div>
              )}
              {onAddNote && (
                <div className="flex gap-1.5">
                  <input
                    type="text"
                    value={noteText}
                    onChange={(e) => setNoteText(e.target.value)}
                    onKeyDown={(e) => e.key === "Enter" && handleAddNote()}
                    placeholder="Add a note..."
                    className="flex-1 h-7 px-2 text-xs rounded-md border border-border bg-muted/30 outline-none focus:border-primary transition-colors"
                    data-testid="note-input"
                  />
                  <Button
                    size="sm"
                    className="h-7 text-[11px] px-2"
                    onClick={handleAddNote}
                    disabled={!noteText.trim()}
                    data-testid="submit-note-btn"
                  >
                    Add
                  </Button>
                </div>
              )}
            </>
          )}
        </div>
      )}

      {/* ── Action Bar ─────────────────────────────────── */}
      <div className="flex items-center justify-end gap-2 px-4 pb-3 pt-1 border-t border-border">
        {editorMode === "translate" && !isEditing && (
          <Button size="sm" onClick={onStartEditing} data-testid="edit-btn">
            Edit
          </Button>
        )}
        {editorMode === "review" && (
          <>
            <Button
              size="sm"
              variant="ghost"
              className="text-destructive hover:text-destructive"
              onClick={onReject}
              data-testid="reject-btn"
            >
              <X className="w-3.5 h-3.5 mr-1" />
              Reject
              <span className="ml-1.5 text-[10px] text-muted-foreground">(Ctrl+Shift+R)</span>
            </Button>
            <Button size="sm" onClick={onApprove} data-testid="approve-btn">
              <Check className="w-3.5 h-3.5 mr-1" />
              Approve
              <span className="ml-1.5 text-[10px] text-muted-foreground">(Ctrl+Shift+A)</span>
            </Button>
          </>
        )}
      </div>

      {/* ── TM Matches (Translate mode) ────────────────── */}
      {editorMode === "translate" && tmMatches.length > 0 && (
        <div className="border-t border-border px-4 py-2">
          <button
            type="button"
            className="flex items-center gap-1 text-[11px] font-bold text-muted-foreground uppercase tracking-wider mb-2 hover:text-foreground transition-colors"
            onClick={() => setTmExpanded((v) => !v)}
            data-testid="tm-toggle"
          >
            {tmExpanded ? (
              <ChevronDown className="w-3 h-3" />
            ) : (
              <ChevronRight className="w-3 h-3" />
            )}
            TM Matches
            <span className="font-normal text-[10px] ml-1">({tmMatches.length})</span>
          </button>
          {tmExpanded && (
            <ContextPanel
              tmMatches={tmMatches}
              termMatches={[]}
              onApplyTM={onApplyTM}
              currentProjectId={project.id}
              sections={{ tm: true, terms: false, entities: false }}
              hideSectionTitles
              className="p-0"
            />
          )}
        </div>
      )}

      {/* ── Block History ──────────────────────────────── */}
      {history && history.length > 0 && (
        <div className="border-t border-border px-4 py-2">
          <button
            type="button"
            className="flex items-center gap-1 text-[11px] font-bold text-muted-foreground uppercase tracking-wider mb-2 hover:text-foreground transition-colors"
            onClick={() => setHistoryExpanded((v) => !v)}
            data-testid="history-toggle"
          >
            {historyExpanded ? (
              <ChevronDown className="w-3 h-3" />
            ) : (
              <ChevronRight className="w-3 h-3" />
            )}
            History
            <span className="font-normal text-[10px] ml-1">({history.length})</span>
          </button>
          {historyExpanded &&
            history.map((entry, i) => (
              <div
                key={entry.seq}
                className="p-2 bg-muted rounded-md mb-1.5 border border-border"
                data-testid={`history-entry-${i}`}
              >
                <div className="flex justify-between items-center mb-1">
                  <span className="text-[10px] text-muted-foreground">
                    {entry.author || "unknown"} &middot;{" "}
                    {new Date(entry.timestamp).toLocaleString()}
                  </span>
                  <span className="text-[10px] px-1.5 py-px rounded bg-muted-foreground/10 text-muted-foreground">
                    {entry.origin}
                  </span>
                </div>
                <div className="text-xs font-medium truncate">{entry.text || "(empty)"}</div>
                {onRevertHistory && i > 0 && (
                  <Button
                    size="sm"
                    variant="ghost"
                    className="mt-1 text-[11px] h-6 px-2"
                    onClick={() => onRevertHistory(entry)}
                    data-testid={`history-revert-${i}`}
                  >
                    Revert to this version
                  </Button>
                )}
              </div>
            ))}
        </div>
      )}
    </div>
  );
}
