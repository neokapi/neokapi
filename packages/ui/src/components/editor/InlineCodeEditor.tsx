import { useState, useEffect, useCallback, useRef } from "react";
import { LexicalComposer } from "@lexical/react/LexicalComposer";
import { useLexicalComposerContext } from "@lexical/react/LexicalComposerContext";
import { ContentEditable } from "@lexical/react/LexicalContentEditable";
import { LexicalErrorBoundary } from "@lexical/react/LexicalErrorBoundary";
import { HistoryPlugin } from "@lexical/react/LexicalHistoryPlugin";
import { RichTextPlugin } from "@lexical/react/LexicalRichTextPlugin";
import {
  $getRoot,
  $createParagraphNode,
  $createTextNode,
  $isElementNode,
  $isNodeSelection,
  COMMAND_PRIORITY_HIGH,
  COMMAND_PRIORITY_CRITICAL,
  KEY_ENTER_COMMAND,
  KEY_ESCAPE_COMMAND,
  KEY_BACKSPACE_COMMAND,
  KEY_DELETE_COMMAND,
  CUT_COMMAND,
  $getSelection,
  $isRangeSelection,
  type LexicalEditor,
} from "lexical";
import type { SpanInfo } from "../../types/span";
import { parseCodedSegments, segmentsToCodedText, type CodedSegment } from "./codedText";
import { TagChipNode, $createTagChipNode, $isTagChipNode } from "./TagChipNode";
import { isDeletable, isCloneable } from "./tagConstraints";
import { TagPalette } from "./TagPalette";
import { TagValidationBar } from "./TagValidationBar";
import { InlinePreview } from "./InlinePreview";
import { validateTags, type TagValidationResult } from "./tagSemantics";

export interface InlineCodeEditorProps {
  initialCodedText: string;
  initialSpans: SpanInfo[];
  sourceSpans: SpanInfo[];
  onSave: (codedText: string, spans: SpanInfo[]) => void;
  onCancel: () => void;
  /** When true, hides the tag palette and preview for compact inline editing. */
  compact?: boolean;
}

/** Read the Lexical editor state and convert to coded text + spans. */
function editorStateToCodedText(root: ReturnType<typeof $getRoot>): {
  codedText: string;
  spans: SpanInfo[];
} {
  const segments: CodedSegment[] = [];
  const paragraphs = root.getChildren();

  paragraphs.forEach((paragraph, pIdx) => {
    if (pIdx > 0) {
      segments.push({ type: "text", value: "\n" });
    }
    if (!$isElementNode(paragraph)) return;
    const children = paragraph.getChildren();
    for (const child of children) {
      if ($isTagChipNode(child)) {
        segments.push({ type: "tag", spanInfo: child.getSpanInfo() });
      } else {
        const text = child.getTextContent();
        if (text) {
          segments.push({ type: "text", value: text });
        }
      }
    }
  });

  return segmentsToCodedText(segments);
}

/** Plugin that handles Enter/Escape keys and auto-focus. */
function KeyHandlerPlugin({ onSave, onCancel }: { onSave: () => void; onCancel: () => void }) {
  const [editor] = useLexicalComposerContext();

  useEffect(() => {
    const unregEnter = editor.registerCommand(
      KEY_ENTER_COMMAND,
      (event: KeyboardEvent | null) => {
        if (event && !event.shiftKey) {
          event.preventDefault();
          onSave();
          return true;
        }
        return false;
      },
      COMMAND_PRIORITY_HIGH,
    );

    const unregEscape = editor.registerCommand(
      KEY_ESCAPE_COMMAND,
      () => {
        onCancel();
        return true;
      },
      COMMAND_PRIORITY_HIGH,
    );

    return () => {
      unregEnter();
      unregEscape();
    };
  }, [editor, onSave, onCancel]);

  // Auto-focus on mount
  useEffect(() => {
    editor.focus();
  }, [editor]);

  return null;
}

/** Plugin that prevents deletion of non-deletable tag chips. */
function TagConstraintPlugin() {
  const [editor] = useLexicalComposerContext();

  useEffect(() => {
    function selectionContainsNonDeletable(): boolean {
      const selection = $getSelection();
      if ($isNodeSelection(selection)) {
        for (const node of selection.getNodes()) {
          if ($isTagChipNode(node) && !isDeletable(node.getSpanInfo())) return true;
        }
      }
      if ($isRangeSelection(selection)) {
        for (const node of selection.getNodes()) {
          if ($isTagChipNode(node) && !isDeletable(node.getSpanInfo())) return true;
        }
      }
      return false;
    }

    const handler = () => selectionContainsNonDeletable();

    const unregBackspace = editor.registerCommand(
      KEY_BACKSPACE_COMMAND,
      handler,
      COMMAND_PRIORITY_CRITICAL,
    );
    const unregDelete = editor.registerCommand(
      KEY_DELETE_COMMAND,
      handler,
      COMMAND_PRIORITY_CRITICAL,
    );
    const unregCut = editor.registerCommand(CUT_COMMAND, handler, COMMAND_PRIORITY_CRITICAL);

    return () => {
      unregBackspace();
      unregDelete();
      unregCut();
    };
  }, [editor]);

  return null;
}

/** Plugin that handles Ctrl+1..9 keyboard shortcuts for tag insertion. */
function TagShortcutPlugin({
  sourceSpans,
  usedSpans,
}: {
  sourceSpans: SpanInfo[];
  usedSpans?: SpanInfo[];
}) {
  const [editor] = useLexicalComposerContext();

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (!e.ctrlKey && !e.metaKey) return;
      const num = parseInt(e.key, 10);
      if (isNaN(num) || num < 1 || num > sourceSpans.length) return;

      e.preventDefault();
      const span = sourceSpans[num - 1];

      // Block insertion of non-cloneable tags already present in target.
      if (!isCloneable(span) && usedSpans) {
        const key = `${span.type}:${span.span_type}`;
        const usedCount = usedSpans.filter((s) => `${s.type}:${s.span_type}` === key).length;
        const sourceCount = sourceSpans.filter((s) => `${s.type}:${s.span_type}` === key).length;
        if (usedCount >= sourceCount) return;
      }

      editor.update(() => {
        const selection = $getSelection();
        if ($isRangeSelection(selection)) {
          const node = $createTagChipNode(span);
          selection.insertNodes([node]);
        }
      });
    };

    const root = editor.getRootElement();
    if (root) {
      root.addEventListener("keydown", handleKeyDown);
      return () => root.removeEventListener("keydown", handleKeyDown);
    }
  }, [editor, sourceSpans, usedSpans]);

  return null;
}

/** Captures the editor instance into a ref for external use. */
function EditorRefCapture({
  editorRef,
}: {
  editorRef: React.MutableRefObject<LexicalEditor | null>;
}) {
  const [editor] = useLexicalComposerContext();
  useEffect(() => {
    editorRef.current = editor;
  }, [editor, editorRef]);
  return null;
}

/** Plugin that observes editor state changes and extracts current spans/coded text. */
function EditorObserverPlugin({
  sourceSpans,
  onUpdate,
}: {
  sourceSpans: SpanInfo[];
  onUpdate: (codedText: string, spans: SpanInfo[], validation: TagValidationResult) => void;
}) {
  const [editor] = useLexicalComposerContext();

  useEffect(() => {
    return editor.registerUpdateListener(({ editorState }) => {
      editorState.read(() => {
        const root = $getRoot();
        const { codedText, spans } = editorStateToCodedText(root);
        const validation = validateTags(sourceSpans, spans);
        onUpdate(codedText, spans, validation);
      });
    });
  }, [editor, sourceSpans, onUpdate]);

  return null;
}

/**
 * Rich text editor for translatable content with visual inline code chips.
 *
 * Renders inline formatting (bold, italic, links, placeholders) as visual
 * tag chips — non-technical users see styled indicators, not XML. Uses
 * Lexical under the hood for editing with undo/redo, constraint enforcement,
 * and keyboard shortcuts (Ctrl+1..9 for tag insertion).
 *
 * Enter saves, Escape cancels.
 */
export function InlineCodeEditor({
  initialCodedText,
  initialSpans,
  sourceSpans,
  onSave,
  onCancel,
  compact,
}: InlineCodeEditorProps) {
  const editorRef = useRef<LexicalEditor | null>(null);
  const [validation, setValidation] = useState<TagValidationResult | null>(null);
  const [currentSpans, setCurrentSpans] = useState<SpanInfo[]>(initialSpans);
  const [currentCodedText, setCurrentCodedText] = useState(initialCodedText);

  const handleSave = useCallback(() => {
    if (!editorRef.current) return;
    editorRef.current.update(() => {
      const root = $getRoot();
      const { codedText, spans } = editorStateToCodedText(root);
      onSave(codedText, spans);
    });
  }, [onSave]);

  const handleInsertTag = useCallback((spanInfo: SpanInfo) => {
    if (!editorRef.current) return;
    editorRef.current.update(() => {
      const selection = $getSelection();
      if ($isRangeSelection(selection)) {
        const node = $createTagChipNode(spanInfo);
        selection.insertNodes([node]);
      }
    });
  }, []);

  const handleEditorUpdate = useCallback(
    (codedText: string, spans: SpanInfo[], val: TagValidationResult) => {
      setCurrentCodedText(codedText);
      setCurrentSpans(spans);
      setValidation(val);
    },
    [],
  );

  const initialConfig = {
    namespace: "InlineCodeEditor",
    onError: (error: Error) => console.error("Lexical error:", error),
    nodes: [TagChipNode],
    editorState: () => {
      const root = $getRoot();
      const paragraph = $createParagraphNode();

      const segments = parseCodedSegments(initialCodedText, initialSpans);
      for (const seg of segments) {
        if (seg.type === "text") {
          paragraph.append($createTextNode(seg.value));
        } else {
          paragraph.append($createTagChipNode(seg.spanInfo));
        }
      }

      root.append(paragraph);
    },
  };

  return (
    <div style={containerStyle}>
      <LexicalComposer initialConfig={initialConfig}>
        <EditorRefCapture editorRef={editorRef} />
        <RichTextPlugin
          contentEditable={<ContentEditable style={editableStyle} />}
          ErrorBoundary={LexicalErrorBoundary}
        />
        <HistoryPlugin />
        <KeyHandlerPlugin onSave={handleSave} onCancel={onCancel} />
        <TagConstraintPlugin />
        <TagShortcutPlugin sourceSpans={sourceSpans} usedSpans={currentSpans} />
        <EditorObserverPlugin sourceSpans={sourceSpans} onUpdate={handleEditorUpdate} />
      </LexicalComposer>
      {!compact && (
        <TagPalette sourceSpans={sourceSpans} onInsert={handleInsertTag} usedSpans={currentSpans} />
      )}
      <TagValidationBar validation={validation} />
      {!compact && <InlinePreview codedText={currentCodedText} spans={currentSpans} />}
    </div>
  );
}

const containerStyle: React.CSSProperties = {
  display: "flex",
  flexDirection: "column",
  width: "100%",
};

const editableStyle: React.CSSProperties = {
  width: "100%",
  minHeight: 44,
  padding: "6px 8px",
  backgroundColor: "var(--bg-tertiary)",
  border: "1px solid var(--accent)",
  borderRadius: 4,
  color: "var(--text-primary)",
  fontSize: 14,
  lineHeight: 1.5,
  outline: "none",
  fontFamily: "inherit",
  boxSizing: "border-box",
};
