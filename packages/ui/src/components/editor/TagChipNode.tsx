import type { ReactNode } from "react";
import {
  DecoratorNode,
  type DOMConversionMap,
  type DOMExportOutput,
  type EditorConfig,
  type LexicalNode,
  type NodeKey,
  type SerializedLexicalNode,
  type Spread,
} from "lexical";
import { createElement } from "react";
import type { SpanInfo } from "../../types/span";
import { TagChipComponent } from "./TagChipComponent";
import { isDeletable } from "./tagConstraints";

export type SerializedTagChipNode = Spread<{ spanInfo: SpanInfo }, SerializedLexicalNode>;

export class TagChipNode extends DecoratorNode<ReactNode> {
  __spanInfo: SpanInfo;

  static getType(): string {
    return "tag-chip";
  }

  static clone(node: TagChipNode): TagChipNode {
    return new TagChipNode(node.__spanInfo, node.__key);
  }

  constructor(spanInfo: SpanInfo, key?: NodeKey) {
    super(key);
    this.__spanInfo = spanInfo;
  }

  getSpanInfo(): SpanInfo {
    return this.__spanInfo;
  }

  createDOM(_config: EditorConfig): HTMLElement {
    const span = document.createElement("span");
    span.style.display = "inline";
    return span;
  }

  updateDOM(): boolean {
    return false;
  }

  exportDOM(): DOMExportOutput {
    const element = document.createElement("span");
    element.setAttribute("data-tag-chip", "true");
    element.textContent = this.__spanInfo.data;
    return { element };
  }

  static importDOM(): DOMConversionMap | null {
    return null;
  }

  static importJSON(serializedNode: SerializedTagChipNode): TagChipNode {
    return new TagChipNode(serializedNode.spanInfo);
  }

  exportJSON(): SerializedTagChipNode {
    return {
      ...super.exportJSON(),
      type: "tag-chip",
      spanInfo: this.__spanInfo,
    };
  }

  isInline(): boolean {
    return true;
  }

  isIsolated(): boolean {
    return true;
  }

  decorate(): ReactNode {
    const locked = !isDeletable(this.__spanInfo);
    return createElement(TagChipComponent, { spanInfo: this.__spanInfo, locked });
  }
}

export function $createTagChipNode(spanInfo: SpanInfo): TagChipNode {
  return new TagChipNode(spanInfo);
}

export function $isTagChipNode(node: LexicalNode | null | undefined): node is TagChipNode {
  return node instanceof TagChipNode;
}
