/**
 * File-level walker: parses one source file with SWC, walks its JSX
 * tree, and emits a `Document` ready to be dropped into a `.klz`.
 *
 * One Block per translatable JSX element plus one Block per
 * translatable attribute. Hashes match the plugin transform's output
 * exactly so `kapi-react compile`'s runtime dict keys line up with
 * what `__t()` / `__tx()` look up at render time.
 */

import { parseSync, type JSXElement, type Module } from '@swc/core';

import type { Block, Document, Run } from '@neokapi/kapi-format';

import { getStringAttr, getTagName, lineFromOffset, resolveHTMLElement } from './ast.ts';
import { buildJSXPath } from './jsx-path.ts';
import { collectTIdentifiers, walkTCalls } from './messages.ts';
import { buildRuns } from './runs.ts';
import { hasTranslatableText, isAllInlineContent, resolvePolicy } from './translatable.ts';
import type { Warning, WarningCollector } from './warnings.ts';

import { translatableAttributes } from '../plugin/defaults.ts';
import { hashKey } from '../plugin/hash.ts';
import { CONTEXT_SEPARATOR, type PluginOptions } from '../types.ts';

export type ExtractOptions = Pick<PluginOptions, 'componentMap' | 'rules'>;

export interface WalkerOptions extends ExtractOptions {
  /**
   * Source-relative file path stored in `Document.path` and as the
   * prefix of every block id. Use a forward-slash path.
   */
  filename: string;
  /**
   * Optional collector the walker pushes warnings into when it
   * auto-promotes a container or extracts from an unmapped
   * PascalCase component.
   */
  warnings?: WarningCollector;
}

/**
 * Parse `code` and return the Document carrying its translatable
 * blocks. Returns null when the source isn't a parseable JSX/TSX file
 * or has no translatable content.
 */
export function extractDocument(code: string, opts: WalkerOptions): Document | null {
  const ast = tryParse(code, opts.filename);
  if (!ast) return null;

  const fallbackComponent = basename(opts.filename);
  const collector = new BlockCollector(code, opts);
  collector.setSpanBase(findBaseOffset(ast));
  walkJsx(ast, (el, ancestors, component) =>
    collector.visit(el, ancestors, component || fallbackComponent),
  );

  const tNames = collectTIdentifiers(ast);
  for (const call of walkTCalls(ast, tNames, (start, end) =>
    code.slice(start - findBaseOffset(ast), end - findBaseOffset(ast)),
  )) {
    collector.visitTCall(call.text, call.context, call.node, fallbackComponent);
  }

  const blocks = collector.blocks();
  if (blocks.length === 0) return null;

  return {
    id: opts.filename,
    documentType: 'jsx',
    path: opts.filename,
    blocks,
  };
}

function basename(filename: string): string {
  const slash = Math.max(filename.lastIndexOf('/'), filename.lastIndexOf('\\'));
  const stem = slash >= 0 ? filename.slice(slash + 1) : filename;
  const dot = stem.lastIndexOf('.');
  return dot >= 0 ? stem.slice(0, dot) : stem;
}

// ─── Collector ────────────────────────────────────────────────────

class BlockCollector {
  private readonly code: string;
  private readonly componentMap: Record<string, string>;
  private readonly rules: NonNullable<ExtractOptions['rules']>;
  private readonly filename: string;
  private readonly warnings: WarningCollector | undefined;
  private readonly out: Block[] = [];
  private readonly seenHashes = new Set<string>();
  /**
   * SWC reports spans as byte offsets anchored to a global parser
   * base (nonzero across processes). We subtract `spanBase` before
   * slicing so offsets address `code` directly.
   */
  private spanBase = 0;

  constructor(code: string, opts: WalkerOptions) {
    this.code = code;
    this.componentMap = opts.componentMap ?? {};
    this.rules = opts.rules ?? [];
    this.filename = opts.filename;
    this.warnings = opts.warnings;
  }

  setSpanBase(base: number): void {
    this.spanBase = base;
  }

  blocks(): Block[] {
    return this.out;
  }

  /**
   * Visits one JSX element. Returns true when an element-level block
   * was emitted — the walker uses that signal to skip re-descending
   * into its direct inline JSX children (they're consumed by the
   * parent's flat-text template). Expression-container children are
   * still visited so conditional JSX inside them (`{cond && <X/>}`)
   * can surface as its own block.
   */
  visit(el: JSXElement, ancestors: readonly JSXElement[], component: string): boolean {
    const tag = getTagName(el);
    if (!tag) return false;
    if (getStringAttr(el, 'translate') === 'no') return false;

    // For unmapped PascalCase components we still want to consider
    // their direct text — the user's source is the ground truth,
    // not an arbitrary componentMap-coverage requirement. We pass
    // the raw tag through as if it were an HTML element; it
    // classifies as `container`, which lets resolvePolicy's
    // promotion rule kick in if the component has direct text.
    const mapped = resolveHTMLElement(tag, this.componentMap);
    const htmlElement = mapped ?? tag;
    const unmappedComponent = mapped === null;

    const policy = resolvePolicy(htmlElement, el, this.rules, this.componentMap);

    // Attribute blocks come out regardless of whether the element's
    // children are translatable — an <input placeholder="…" /> still
    // earns an attribute block. Unmapped components also get their
    // translatable-by-convention props extracted (title, subtitle,
    // description, label, …) so <PageHeader title="Termbases" />
    // works without needing a componentMap entry. jsxPath uses the
    // raw tag for unmapped components, so hash parity holds across
    // extract + transform.
    this.emitAttributeBlocks(el, ancestors, policy.locNote, component);

    if (!policy.translate) return false;
    if (!hasTranslatableText(el)) return false;
    if (!isAllInlineContent(el, this.componentMap)) return false;

    // An unmapped PascalCase component always implies a promotion
    // (tags default to 'container'), so emit only the more specific
    // warning that points the dev at componentMap.
    if (unmappedComponent) {
      this.warn('unknown-component', tag, el);
    } else if (policy.promoted) {
      this.warn('container-promoted', tag, el);
    }

    this.emitElementBlock(el, ancestors, policy.locNote, component);
    return true;
  }

  private warn(
    kind: Warning['kind'],
    tag: string,
    el: JSXElement,
  ): void {
    if (!this.warnings) return;
    const line = lineFromOffset(this.code, el.span.start);
    this.warnings.add({
      kind,
      filename: this.filename,
      line,
      tag,
      snippet: this.snippet(line),
    });
  }

  private snippet(line: number): string {
    const lines = this.code.split('\n');
    const raw = (lines[line - 1] ?? '').trim();
    return raw.length > 80 ? `${raw.slice(0, 80)}…` : raw;
  }

  // ─── t() calls ───────────────────────────────────────────────

  /**
   * Emit a Block for a user-facing `t("text", context?, params?)`
   * call. The "t" desc channel prefix keeps these hashes from
   * colliding with identically-worded JSX blocks — translators
   * should be able to change a `t("Save")` translation without
   * also touching every `<Button>Save</Button>`.
   *
   * When context is non-null it enters the descriptor so the same
   * source text with different meanings (gettext's msgctxt) hashes
   * distinctly.
   */
  visitTCall(
    text: string,
    context: string | null,
    node: { span: { start: number; end: number } },
    component: string,
  ): void {
    if (text === '') return;

    const desc = `t${CONTEXT_SEPARATOR}${context ?? ''}`;
    const hash = hashKey(text, desc);
    if (this.seenHashes.has(hash)) return;
    this.seenHashes.add(hash);

    const line = lineFromOffset(this.code, node.span.start);
    const properties: Block['properties'] = {
      file: this.filename,
      line,
      component,
      jsxPath: 't()',
      element: 't',
    };
    if (context) properties.locNote = context;

    this.out.push({
      id: `${this.filename}:${line}:t`,
      hash,
      translatable: true,
      type: 'js:t',
      source: [{ text }] as Run[],
      placeholders: [],
      properties,
    });
  }

  // ─── Element blocks ─────────────────────────────────────────

  private emitElementBlock(
    el: JSXElement,
    ancestors: readonly JSXElement[],
    locNote: string | undefined,
    component: string,
  ): void {
    const jsxPath = buildJSXPath(ancestors, el, this.componentMap);
    const desc = locNote ? `${jsxPath}${CONTEXT_SEPARATOR}${locNote}` : jsxPath;
    const { runs, flatText, placeholders } = buildRuns(el, {
      componentMap: this.componentMap,
      sourceSlice: (start, end) => this.sliceSource(start, end),
    });
    if (flatText === '') return;

    const hash = hashKey(flatText, desc);
    if (this.seenHashes.has(hash)) return;
    this.seenHashes.add(hash);

    this.out.push({
      id: `${this.filename}:${lineFromOffset(this.code, el.span.start)}:${this.out.length}`,
      hash,
      translatable: true,
      type: 'jsx:element',
      source: runs,
      placeholders,
      properties: blockProperties(this.filename, el, this.code, jsxPath, component, locNote),
    });
  }

  // ─── Attribute blocks ───────────────────────────────────────

  private emitAttributeBlocks(
    el: JSXElement,
    ancestors: readonly JSXElement[],
    locNote: string | undefined,
    component: string,
  ): void {
    for (const attr of el.opening.attributes ?? []) {
      if (attr.type !== 'JSXAttribute') continue;
      if (attr.name.type !== 'Identifier') continue;
      const name = attr.name.value;
      if (!translatableAttributes.has(name)) continue;
      if (!attr.value || attr.value.type !== 'StringLiteral') continue;
      const text = attr.value.value;
      if (!text.trim()) continue;

      const jsxPath = buildJSXPath(ancestors, el, this.componentMap);
      const context = `${jsxPath}[${name}]`;
      const desc = locNote ? `${context}${CONTEXT_SEPARATOR}${locNote}` : context;
      const hash = hashKey(text, desc);
      if (this.seenHashes.has(hash)) continue;
      this.seenHashes.add(hash);

      this.out.push({
        id: `${this.filename}:${lineFromOffset(this.code, el.span.start)}:${name}`,
        hash,
        translatable: true,
        type: 'jsx:attribute',
        source: [{ text }] as Run[],
        placeholders: [],
        properties: blockProperties(this.filename, el, this.code, context, component, locNote),
      });
    }
  }

  // ─── Source helpers ─────────────────────────────────────────

  private sliceSource(start: number, end: number): string {
    const a = start - this.spanBase;
    const b = end - this.spanBase;
    if (a < 0 || b <= a) return '';
    return this.code.slice(a, b);
  }
}

// ─── Helpers ──────────────────────────────────────────────────────

function blockProperties(
  filename: string,
  el: JSXElement,
  code: string,
  jsxPath: string,
  component: string,
  locNote: string | undefined,
): Block['properties'] {
  const properties: Block['properties'] = {
    file: filename,
    line: lineFromOffset(code, el.span.start),
    component,
    jsxPath,
    element: getTagName(el) ?? '',
  };
  if (locNote) properties.locNote = locNote;
  return properties;
}

// ─── AST traversal plumbing ───────────────────────────────────────

/**
 * Walk every JSX element in the module. Each visit receives:
 *
 *   - the element itself
 *   - its ancestor stack (immutable snapshot)
 *   - the name of the innermost enclosing React component, if any
 *
 * The component name is derived from the nearest ancestor function
 * or variable declarator that contains the JSX; it's the value a
 * consumer would refer to when writing "a TagChip block". Spans
 * are normalized to be zero-based against the input string so
 * downstream slicing works on the raw code.
 */
function walkJsx(
  module: Module,
  visit: (el: JSXElement, ancestors: readonly JSXElement[], component: string) => boolean,
): void {
  const base = findBaseOffset(module);
  const ancestors: JSXElement[] = [];
  const components: string[] = [];

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  function descend(node: any): void {
    if (!node || typeof node !== 'object') return;

    if (node.span && typeof node.span.start === 'number' && base > 0) {
      node.span.start -= base;
      node.span.end -= base;
    }

    const pushedComponent = enterComponentScope(node, components);

    if (node.type === 'JSXElement') {
      const el = node as JSXElement;
      const currentComponent = components[components.length - 1] ?? '';
      const emitted = visit(el, [...ancestors], currentComponent);

      // If a block was emitted for el, its inline JSX children were
      // consumed by the parent's flat text template (a single `ph`
      // per inline child). Re-visiting them would emit duplicate
      // blocks the plugin transform will never look up at runtime.
      // Expression-container children still need visits so
      // `{cond && <X/>}`-style conditional JSX can surface inner
      // translatable text as its own block.
      ancestors.push(el);
      if (emitted) {
        for (const child of el.children ?? []) {
          if (child.type === 'JSXExpressionContainer') descend(child);
        }
      } else {
        for (const child of el.children ?? []) descend(child);
      }
      if (el.opening) descend(el.opening);
      if (el.closing) descend(el.closing);
      ancestors.pop();
    } else {
      for (const key of Object.keys(node)) {
        if (key === 'type') continue;
        const val = (node as Record<string, unknown>)[key];
        if (Array.isArray(val)) for (const item of val) descend(item);
        else if (val && typeof val === 'object' && 'type' in val) descend(val);
      }
    }

    if (pushedComponent) components.pop();
  }

  descend(module);
}

/**
 * Pushes a component name onto the stack if `node` is a function-like
 * declaration whose name is PascalCase (the React convention).
 * Returns true if a name was pushed so the caller knows to pop.
 */
function enterComponentScope(
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  node: any,
  components: string[],
): boolean {
  const name = componentNameFromNode(node);
  if (!name) return false;
  components.push(name);
  return true;
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
function componentNameFromNode(node: any): string | null {
  switch (node.type) {
    case 'FunctionDeclaration':
      return pascal(node.identifier?.value);
    case 'ExportDefaultDeclaration': {
      // Anonymous default export — we leave the name blank; the
      // walker will fall back to the file's basename.
      return null;
    }
    case 'VariableDeclarator': {
      const nameValue = node.id?.value;
      const init = node.init;
      if (!init) return null;
      if (init.type !== 'ArrowFunctionExpression' && init.type !== 'FunctionExpression') {
        return null;
      }
      return pascal(nameValue);
    }
    default:
      return null;
  }
}

function pascal(name: string | undefined): string | null {
  if (!name) return null;
  const first = name[0];
  if (first >= 'A' && first <= 'Z') return name;
  return null;
}

/**
 * SWC reports `BytePos` offsets on the 1-based scheme Rust's source-map
 * crate uses. Position `1` corresponds to byte 0 of the input, so a
 * slice of the JS input requires subtracting 1 from every span
 * bound. The value is a constant regardless of leading whitespace
 * in the module — `module.span.start` is NOT the right base because
 * it skips any leading whitespace.
 */
function findBaseOffset(_module: Module): number {
  return 1;
}

function tryParse(code: string, filename: string): Module | null {
  try {
    return parseSync(code, {
      syntax: filename.endsWith('.tsx') ? 'typescript' : 'ecmascript',
      tsx: true,
      jsx: true,
    });
  } catch {
    return null;
  }
}
