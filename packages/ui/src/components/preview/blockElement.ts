// blockElement — the identity and selection contract every preview puts on the
// element carrying one block.
//
// Two things depend on it from outside the kit. A host scrolls a unit into view
// by querying `[data-block-id="…"]` inside its own scroll container (the review
// surface and the desktop's file preview both do), and a host decorates a block
// through `blockAttrs` with whatever it encodes there: review status, a batch
// mark, a test hook. Both hold whichever preview drew the element, so the
// document reading and the keyed table state the contract once, here, rather
// than twice in agreement until one of them changes.

import React from "react";

/**
 * Host-supplied decoration for one block's element: a class name plus any
 * `data-*` markers. The kit knows nothing about what a host encodes there; it
 * only puts the attributes on the element that carries the block, beside the
 * `data-block-id` it always emits.
 */
export type BlockAttrs = { className?: string } & {
  [attr: `data-${string}`]: string | undefined;
};

/** What one block's element needs to carry its identity and selection state. */
export interface BlockElementSpec {
  /** The block id, emitted as `data-block-id`. */
  id: string;
  /** The host's decoration for this block. */
  attrs?: BlockAttrs;
  /** True while this block is the selected one. */
  selected: boolean;
  /**
   * Called with the block id when the element is activated. Present, the
   * element becomes a focusable, button-roled target, so the document itself is
   * the way to select a block and a host needs no list beside it.
   */
  onSelect?: (id: string) => void;
  /** Class names the drawing component owns. */
  className?: string;
  /** Class applied while the element is selectable. */
  selectableClass?: string;
  /** Class applied while the element is the selected one. */
  selectedClass?: string;
}

/** The props for one block's element. */
export type BlockElementProps = Record<string, unknown> & { className?: string };

/**
 * Build one block element's props: its identity, the host's decoration, and
 * button semantics when the host listens for selection.
 *
 * A click that ends a text selection is not a selection of the block: the
 * reader was highlighting a phrase (to mark a term, say), and stealing that
 * gesture would close the reading over it.
 */
export function useBlockElementProps(spec: BlockElementSpec): BlockElementProps {
  const { id, attrs, selected, onSelect, className, selectableClass, selectedClass } = spec;
  const { className: hostClass, ...dataAttrs } = attrs ?? {};

  const activate = React.useCallback(() => {
    if (!onSelect) return;
    if (typeof window !== "undefined" && window.getSelection()?.isCollapsed === false) return;
    onSelect(id);
  }, [onSelect, id]);

  const onKeyDown = React.useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key !== "Enter" && e.key !== " ") return;
      e.preventDefault();
      activate();
    },
    [activate],
  );

  const classes = [
    className,
    onSelect ? selectableClass : undefined,
    selected ? selectedClass : undefined,
    hostClass,
  ].filter(Boolean);

  const base: BlockElementProps = {
    ...dataAttrs,
    "data-block-id": id,
    ...(classes.length > 0 ? { className: classes.join(" ") } : {}),
  };
  if (!onSelect) return base;
  return {
    ...base,
    role: "button",
    tabIndex: 0,
    "aria-current": selected ? "true" : undefined,
    onClick: activate,
    onKeyDown,
  };
}
