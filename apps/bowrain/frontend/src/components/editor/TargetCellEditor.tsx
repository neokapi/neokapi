import { useEffect, useCallback, useRef } from "react";
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
  COMMAND_PRIORITY_HIGH,
  KEY_ENTER_COMMAND,
  KEY_ESCAPE_COMMAND,
  $getSelection,
  $isRangeSelection,
  type LexicalEditor,
} from "lexical";
import type { SpanInfo } from "../../types/api";
import { parseCodedSegments, segmentsToCodedText, type CodedSegment } from "./codedText";
import { TagChipNode, $createTagChipNode, $isTagChipNode } from "./TagChipNode";
import { TagPalette } from "./TagPalette";

interface TargetCellEditorProps {
  initialCodedText: string;
  initialSpans: SpanInfo[];
  sourceSpans: SpanInfo[];
  onSave: (codedText: string, spans: SpanInfo[]) => void;
  onCancel: () => void;
}

/** Read the Lexical editor state and convert to coded text + spans. */
function editorStateToCodedText(
  root: ReturnType<typeof $getRoot>,
): { codedText: string; spans: SpanInfo[] } {
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
function KeyHandlerPlugin({
  onSave,
  onCancel,
}: {
  onSave: () => void;
  onCancel: () => void;
}) {
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

/** Plugin that handles Ctrl+1..9 keyboard shortcuts for tag insertion. */
function TagShortcutPlugin({ sourceSpans }: { sourceSpans: SpanInfo[] }) {
  const [editor] = useLexicalComposerContext();

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (!e.ctrlKey && !e.metaKey) return;
      const num = parseInt(e.key, 10);
      if (isNaN(num) || num < 1 || num > sourceSpans.length) return;

      e.preventDefault();
      const span = sourceSpans[num - 1];
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
  }, [editor, sourceSpans]);

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

export function TargetCellEditor({
  initialCodedText,
  initialSpans,
  sourceSpans,
  onSave,
  onCancel,
}: TargetCellEditorProps) {
  const editorRef = useRef<LexicalEditor | null>(null);

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

  const initialConfig = {
    namespace: "TargetEditor",
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
        <TagShortcutPlugin sourceSpans={sourceSpans} />
      </LexicalComposer>
      <TagPalette sourceSpans={sourceSpans} onInsert={handleInsertTag} />
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
