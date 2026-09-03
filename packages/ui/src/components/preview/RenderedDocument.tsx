import React from "react";
import { cn } from "../../lib/utils";
import { directionAttrs } from "../../lib/text-direction";
import { headingLevel, inlineSegments, type InlineSeg } from "./projectionRender";
import { INLINE_TAG } from "./inlineStyle";
import type { RenderNode } from "./types";

// RenderedDocument paints the Go generative-projection render AST
// (ContentTree.render, core/projection) as the document a human recognizes:
// headings, paragraphs with real inline formatting (bold/italic/links/images),
// reconstructed tables, and lists. It is the preview-fidelity payoff — the same
// projected tree every cross-format writer renders, shown faithfully instead of
// flattened to plain text (preview-fidelity #1/#2). Pure presentational; overlay
// highlighting / diff / transitions stay on the structured FormatPreview path.

export interface RenderedDocumentProps {
  /** The projected render tree (ContentTree.render). */
  node: RenderNode;
  /**
   * The locale of the rendered text. Sets `dir`/`lang` on the document root so
   * an RTL document (Arabic, Hebrew, Persian, Urdu) reads right-to-left, and
   * embedded LTR fragments — URLs, code, product names — resolve in the right
   * visual order rather than scrambling.
   */
  locale?: string;
  className?: string;
}

// The canonical run type → inline element map is shared with the structured
// preview (see ./inlineStyle), so both readings agree on what a bold or code
// pair renders as.

export default function RenderedDocument({
  node,
  locale,
  className,
}: RenderedDocumentProps): React.ReactElement {
  return (
    <div
      {...directionAttrs(locale)}
      className={cn("kapi-rendered space-y-2 text-sm leading-relaxed", className)}
    >
      {renderNode(node, "n")}
    </div>
  );
}

function renderChildren(node: RenderNode, keyBase: string): React.ReactNode {
  return (node.children ?? []).map((c, i) => renderNode(c, `${keyBase}.${i}`));
}

function renderNode(node: RenderNode, key: string): React.ReactNode {
  const role = node.role ?? "";

  // Structural containers.
  if (role === "document")
    return <React.Fragment key={key}>{renderChildren(node, key)}</React.Fragment>;
  if (role === "table") return <TableNode key={key} node={node} keyBase={key} />;
  if (role === "list") {
    const items = (node.children ?? []).map((c, i) => (
      <li key={`${key}.${i}`}>
        {renderInline(c.runs)}
        {renderChildren(c, `${key}.${i}`)}
      </li>
    ));
    return node.ordered ? (
      <ol key={key} className="list-decimal pl-6">
        {items}
      </ol>
    ) : (
      <ul key={key} className="list-disc pl-6">
        {items}
      </ul>
    );
  }

  // Heading leaves.
  const hl = headingLevel(node);
  if (hl !== null) {
    const Tag = `h${Math.min(Math.max(hl, 1), 6)}` as keyof React.JSX.IntrinsicElements;
    return (
      <Tag key={key} className="font-semibold">
        {renderInline(node.runs)}
      </Tag>
    );
  }

  // Code block.
  if (role === "code") {
    return (
      <pre key={key} className="rounded bg-muted p-2 font-mono text-xs">
        <code>{node.runs?.map((r) => r.text ?? "").join("")}</code>
      </pre>
    );
  }

  // Other leaves (paragraph, caption, …): render inline runs, then any children.
  const inline = renderInline(node.runs);
  const kids = (node.children ?? []).length > 0 ? renderChildren(node, key) : null;
  if (inline === null && kids === null) return null;
  return (
    <p key={key} className="whitespace-pre-wrap">
      {inline}
      {kids}
    </p>
  );
}

function TableNode({ node, keyBase }: { node: RenderNode; keyBase: string }): React.ReactElement {
  return (
    <table className="border-collapse border border-border text-sm">
      <tbody>
        {(node.children ?? []).map((row, ri) => (
          <tr key={`${keyBase}.r${ri}`}>
            {(row.children ?? []).map((cell, ci) => {
              const isHeader = cell.header === true || cell.role === "table-header";
              const Tag = isHeader ? "th" : "td";
              return (
                <Tag
                  key={`${keyBase}.r${ri}.c${ci}`}
                  className="border border-border px-2 py-1 text-left align-top"
                  colSpan={cell.colSpan && cell.colSpan > 1 ? cell.colSpan : undefined}
                  rowSpan={cell.rowSpan && cell.rowSpan > 1 ? cell.rowSpan : undefined}
                >
                  {renderInline(cell.runs)}
                </Tag>
              );
            })}
          </tr>
        ))}
      </tbody>
    </table>
  );
}

// renderInline turns a run sequence into nested React inline elements, driving
// the shared inlineSegments decoder through a small open/close stack so
// pcOpen/pcClose pairs become real <strong>/<em>/<a>/… and placeholders become
// <img>/labelled spans. Returns null for empty content.
function renderInline(runs: RenderNode["runs"]): React.ReactNode {
  const segs = inlineSegments(runs);
  if (segs.length === 0) return null;

  interface Frame {
    type: string;
    attrs?: Record<string, string>;
    children: React.ReactNode[];
  }
  const root: Frame = { type: "", children: [] };
  const stack: Frame[] = [root];
  let k = 0;
  const top = () => stack[stack.length - 1];

  for (const seg of segs) {
    switch (seg.kind) {
      case "text":
        top().children.push(<React.Fragment key={k++}>{seg.text}</React.Fragment>);
        break;
      case "open":
        stack.push({ type: seg.type, attrs: seg.attrs, children: [] });
        break;
      case "close": {
        if (stack.length > 1) {
          const frame = stack.pop()!;
          top().children.push(inlineElement(frame.type, frame.attrs, frame.children, k++));
        }
        break;
      }
      case "placeholder":
        top().children.push(placeholderElement(seg, k++));
        break;
    }
  }
  // Close any unbalanced frames defensively.
  while (stack.length > 1) {
    const frame = stack.pop()!;
    top().children.push(inlineElement(frame.type, frame.attrs, frame.children, k++));
  }
  return root.children;
}

function inlineElement(
  type: string,
  attrs: Record<string, string> | undefined,
  children: React.ReactNode[],
  key: number,
): React.ReactNode {
  if (type === "link:hyperlink") {
    return (
      <a key={key} href={attrs?.href} title={attrs?.title} rel="noreferrer">
        {children}
      </a>
    );
  }
  const Tag = INLINE_TAG[type];
  if (Tag) return <Tag key={key}>{children}</Tag>;
  // Unknown paired type: render its content without markup.
  return <React.Fragment key={key}>{children}</React.Fragment>;
}

function placeholderElement(
  seg: Extract<InlineSeg, { kind: "placeholder" }>,
  key: number,
): React.ReactNode {
  if (seg.type === "media:image") {
    // eslint-disable-next-line @next/next/no-img-element
    return (
      <img
        key={key}
        src={seg.attrs?.src}
        alt={seg.attrs?.alt ?? seg.equiv ?? ""}
        title={seg.attrs?.title}
        className="inline-block max-h-40"
      />
    );
  }
  // A variable / control placeholder: show its human-readable equivalent.
  return (
    <span
      key={key}
      className="rounded bg-amber-500/15 px-1 text-amber-700 dark:text-amber-400"
      title={seg.type}
    >
      {seg.equiv || seg.type}
    </span>
  );
}
