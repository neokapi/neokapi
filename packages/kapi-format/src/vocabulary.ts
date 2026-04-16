/**
 * Vocabulary registry — maps run types to rendering/constraint
 * metadata. Mirrors neokapi's core/model/vocabulary.go so every
 * format reader (HTML, Markdown, JSX/TSX, future PO/XLIFF) can
 * register its inline codes in one place.
 *
 * The registry is the single point where "this run represents
 * bold" gets mapped to "render as <b>, chip label 'B', color navy".
 * That uniformity is what lets neokapi's preview builders (and any
 * downstream tool that consumes them) render inline codes
 * consistently across formats without per-format rendering code.
 */

import type { RunConstraints } from './block.ts';

export interface VocabularyEntry {
  /** Same value as the `type` field on a Run, e.g. "jsx:element". */
  key: string;

  /** Coarse bucket for consumer-side filtering / grouping. */
  category: VocabularyCategory;

  /** HTML rendering emitted by neokapi's preview builders. */
  html: HTMLRendering;

  /** Plain-text rendering for terminals / MT APIs that don't take HTML. */
  display: TextRendering;

  /** CAT-tool chip label + color. */
  chip: ChipRendering;

  /** Default constraints applied to matching runs. */
  constraints: RunConstraints;
}

export type VocabularyCategory =
  | 'format'       // bold, italic, underline (inline formatting)
  | 'structure'    // link, image, button (block-level)
  | 'variable'     // placeholder for a runtime value
  | 'node'         // placeholder for a runtime ReactNode
  | 'code'         // inline code span, keyword, identifier
  | 'entity';      // named entity (person, product)

export interface HTMLRendering {
  /** Returned for PcOpen runs. May contain `{data}` or `{subType}` templates. */
  open: string;
  /** Returned for PcClose runs. */
  close: string;
  /** Returned for Placeholder runs (self-closing). */
  placeholder: string;
}

export interface TextRendering {
  open: string;
  close: string;
  placeholder: string;
}

export interface ChipRendering {
  /** 1-3 character label shown in CAT-tool chips. */
  label: string;
  /** CSS color tokens. Consumers can pipe these into their theme. */
  color: {
    bg: string;
    border: string;
    text: string;
  };
}

// ─── Default JSX vocabulary entries ────────────────────────────────

/**
 * The three new span types neokapi-react needs. HTML renderings
 * produce semantic markup that downstream preview renderers can
 * style with a shared CSS theme — the same approach neokapi's
 * existing HTML and Markdown vocabularies already use.
 */
export const JSX_VOCABULARY: VocabularyEntry[] = [
  {
    key: 'jsx:element',
    category: 'structure',
    html: {
      // Rendered with the actual tag name from `subType` so <a>,
      // <strong>, <span>, custom components all preview as
      // themselves. Consumers that sandbox the preview may
      // whitelist a safe subset of tags.
      open: '<{subType} data-neokapi-span="{id}">',
      close: '</{subType}>',
      placeholder: '',
    },
    display: {
      open: '<{subType}>',
      close: '</{subType}>',
      placeholder: '',
    },
    chip: {
      label: '{subType}',
      color: { bg: '#e2e8f0', border: '#94a3b8', text: '#1e293b' },
    },
    constraints: {
      deletable: false,    // element tokens mirror source structure
      cloneable: false,
      reorderable: true,
    },
  },
  {
    key: 'jsx:var',
    category: 'variable',
    html: {
      open: '',
      close: '',
      placeholder:
        '<span class="neokapi-var" data-var="{equiv}" data-type="{subType}">{equiv}</span>',
    },
    display: {
      open: '',
      close: '',
      placeholder: '{{equiv}}',
    },
    chip: {
      label: '{equiv}',
      color: { bg: '#dbeafe', border: '#3b82f6', text: '#1e40af' },
    },
    constraints: {
      deletable: false,    // variables must survive translation
      cloneable: true,     // languages that repeat the value (formal/informal)
      reorderable: true,
    },
  },
  {
    key: 'jsx:node',
    category: 'node',
    html: {
      open: '',
      close: '',
      placeholder:
        '<span class="neokapi-node" data-node="{id}" title="{data}">{equiv}</span>',
    },
    display: {
      open: '',
      close: '',
      // Rendered as a guillemet-wrapped label so LLMs/translators see
      // it as an opaque token but still know its semantic role.
      placeholder: '«{equiv}»',
    },
    chip: {
      label: '⟨⟩',
      color: { bg: '#fef3c7', border: '#f59e0b', text: '#92400e' },
    },
    constraints: {
      deletable: true,     // conditional nodes may drop in some languages
      cloneable: false,
      reorderable: true,
    },
  },
];

// ─── Template expansion ────────────────────────────────────────────

/**
 * Expand a vocabulary template by substituting `{field}` placeholders
 * with values from a context object. Used by neokapi's preview
 * builders and by this package's reference renderer; exported here
 * so tests can exercise it without pulling in the full framework.
 */
export function expandTemplate(
  template: string,
  context: Record<string, string>,
): string {
  return template.replace(/\{(\w+)\}/g, (_, key) => context[key] ?? '');
}
