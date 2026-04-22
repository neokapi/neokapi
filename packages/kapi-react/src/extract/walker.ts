/**
 * File-level walker: parses one source file with SWC, walks its JSX
 * tree, and emits a `Document` ready to be serialised as a `.klf`.
 *
 * One Block per translatable JSX element plus one Block per
 * translatable attribute. Hashes match the plugin transform's output
 * exactly so `kapi-react compile`'s runtime dict keys line up with
 * what `__t()` / `__tx()` look up at render time.
 */

import { parseSync, type JSXElement, type Module } from "@swc/core";

import type { Block, Document, Run } from "@neokapi/kapi-format";

import {
  getTagName,
  labelLikeMemberExpr,
  lineFromOffset,
  nearestTranslate,
  resolveHTMLElement,
} from "./ast.ts";
import { buildJSXPath } from "./jsx-path.ts";
import { collectTIdentifiers, walkTCalls } from "./messages.ts";
import { buildRuns } from "./runs.ts";
import { hasTranslatableText, isAllInlineContent, resolvePolicy } from "./translatable.ts";
import type { Warning, WarningCollector } from "./warnings.ts";

import { translatableAttributes } from "../plugin/defaults.ts";
import { hashKey } from "../plugin/hash.ts";
import { resolveLibraryComponentMap } from "../plugin/manifests.ts";
import { CONTEXT_SEPARATOR, type PluginOptions } from "../types.ts";

export type ExtractOptions = Pick<PluginOptions, "componentMap" | "rules" | "communityManifestDir">;

export interface WalkerOptions extends ExtractOptions {
  /**
   * Source-relative file path stored in `Document.path` and as the
   * prefix of every block id. Use a forward-slash path.
   */
  filename: string;
  /**
   * Project root directory for resolving library i18n manifests +
   * falling back to `.d.ts` parsing for component → HTML element
   * detection. Defaults to `process.cwd()` when omitted.
   */
  projectRoot?: string;
  /**
   * Optional collector the walker pushes warnings into when it
   * auto-promotes a container or extracts from an unmapped
   * React component.
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

  // Auto-resolve library manifests (+ .d.ts fallback) for every
  // non-relative import. User-supplied componentMap wins on key
  // collisions so explicit overrides still take precedence.
  const libraryMap = resolveLibraryComponentMap(
    ast,
    opts.projectRoot ?? process.cwd(),
    opts.communityManifestDir,
    opts.filename,
  );
  const effectiveMap: Record<string, string> = {
    ...libraryMap,
    ...opts.componentMap,
  };

  const fallbackComponent = basename(opts.filename);
  const collector = new BlockCollector(code, { ...opts, componentMap: effectiveMap });
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
    documentType: "jsx",
    path: opts.filename,
    blocks,
  };
}

function basename(filename: string): string {
  const slash = Math.max(filename.lastIndexOf("/"), filename.lastIndexOf("\\"));
  const stem = slash >= 0 ? filename.slice(slash + 1) : filename;
  const dot = stem.lastIndexOf(".");
  return dot >= 0 ? stem.slice(0, dot) : stem;
}

// ─── Collector ────────────────────────────────────────────────────

class BlockCollector {
  private readonly code: string;
  private readonly componentMap: Record<string, string>;
  private readonly rules: NonNullable<ExtractOptions["rules"]>;
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
    // W3C translate inheritance: nearest explicit setting on self
    // or an ancestor wins. `translate="yes"` re-enables translation
    // inside a `translate="no"` subtree. Mirrored in plugin/transform.ts.
    if (nearestTranslate(el, ancestors) === "no") return false;

    // For unmapped React components we still want to consider
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

    const willEmit =
      policy.translate && hasTranslatableText(el) && isAllInlineContent(el, this.componentMap);

    // Only flag `{obj.label}`-style splice risks when the parent
    // ISN'T going to emit a block. When it does, the expression
    // becomes a `jsx:var` placeholder inside the block — the
    // label's value flows through runtime substitution and is
    // translated as part of the enclosing block. Firing the splice
    // warning there would be a false positive.
    if (!willEmit) this.scanForLabelSplice(el);

    if (!willEmit) return false;

    // Unknown components get a warning pointing the dev at
    // componentMap for hash stability. Container-element promotion
    // (e.g. `<div>Label</div>`) doesn't warn — it's the expected
    // default for the dominant React idiom, and warnings added
    // noise without any actionable follow-up.
    if (unmappedComponent) {
      this.warn("unknown-component", tag, el);
    }

    this.emitElementBlock(el, ancestors, policy.locNote, component);
    return true;
  }

  private warn(kind: Warning["kind"], tag: string, el: JSXElement): void {
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

  /**
   * Walks `el`'s direct JSX children and flags any expression
   * container that dereferences a label-like property
   * (`{meta.label}`, `{item.title}`, …). Fires before the walker
   * recurses, so the warning surfaces whether or not the enclosing
   * element ends up as a translatable block.
   *
   * visit() runs before the walker descends into `el`, so child
   * expression spans are still in raw SWC-base coordinates;
   * subtract spanBase to align with `code`.
   */
  private scanForLabelSplice(el: JSXElement): void {
    if (!this.warnings) return;
    for (const child of el.children ?? []) {
      if (child.type !== "JSXExpressionContainer") continue;
      if (child.expression.type === "JSXEmptyExpression") continue;
      const expr = child.expression;
      const labelExpr = labelLikeMemberExpr(expr as never);
      if (!labelExpr) continue;

      const rawStart = (expr as { span?: { start: number } }).span?.start ?? 0;
      const offset = Math.max(0, rawStart - this.spanBase);
      const line = lineFromOffset(this.code, offset);
      this.warnings.add({
        kind: "dyn-label-splice",
        filename: this.filename,
        line,
        tag: labelExpr,
        snippet: this.snippet(line),
      });
    }
  }

  private snippet(line: number): string {
    const lines = this.code.split("\n");
    const raw = (lines[line - 1] ?? "").trim();
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
    if (text === "") return;

    const desc = `t${CONTEXT_SEPARATOR}${context ?? ""}`;
    const hash = hashKey(text, desc);
    if (this.seenHashes.has(hash)) return;
    this.seenHashes.add(hash);

    const line = lineFromOffset(this.code, node.span.start);
    const properties: Block["properties"] = {
      file: this.filename,
      line,
      component,
      jsxPath: "t()",
      element: "t",
    };
    if (context) properties.locNote = context;

    this.out.push({
      id: `${this.filename}:${line}:t`,
      hash,
      translatable: true,
      type: "js:t",
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
    if (flatText === "") return;

    const hash = hashKey(flatText, desc);
    if (this.seenHashes.has(hash)) return;
    this.seenHashes.add(hash);

    this.out.push({
      id: `${this.filename}:${lineFromOffset(this.code, el.span.start)}:${this.out.length}`,
      hash,
      translatable: true,
      type: "jsx:element",
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
      if (attr.type !== "JSXAttribute") continue;
      if (attr.name.type !== "Identifier") continue;
      const name = attr.name.value;
      if (!translatableAttributes.has(name)) continue;
      if (!attr.value) continue;

      // Plain string literal: single block (the dominant case).
      if (attr.value.type === "StringLiteral") {
        this.emitOneAttributeBlock(el, ancestors, locNote, component, name, attr.value.value, null);
        continue;
      }

      // `title={cond ? "A" : "B"}` — extract both branches so neither
      // silently bypasses translation. Branches disambiguate via a
      // branch index (::0 / ::1) appended to the attribute context;
      // transform rewrites each branch to its own __t() call using
      // the same suffix scheme.
      if (
        attr.value.type === "JSXExpressionContainer" &&
        attr.value.expression.type === "ConditionalExpression"
      ) {
        const cond = attr.value.expression;
        const cLit = cond.consequent.type === "StringLiteral";
        const aLit = cond.alternate.type === "StringLiteral";
        if (cLit && aLit) {
          // Both string literals — extract one block per branch.
          this.emitOneAttributeBlock(
            el,
            ancestors,
            locNote,
            component,
            name,
            (cond.consequent as { value: string }).value,
            0,
          );
          this.emitOneAttributeBlock(
            el,
            ancestors,
            locNote,
            component,
            name,
            (cond.alternate as { value: string }).value,
            1,
          );
          continue;
        }
        // Only warn when exactly one branch is a string literal —
        // the other half is unextractable, so the attr's translation
        // state is half-broken. When both are non-literals (e.g.
        // `cond ? t("A") : t("B")`) the t()-call walker handles them
        // separately; no warning needed.
        if (cLit !== aLit) {
          this.warn("ternary-attr-complex", `${name}`, el);
        }
      }
    }
  }

  private emitOneAttributeBlock(
    el: JSXElement,
    ancestors: readonly JSXElement[],
    locNote: string | undefined,
    component: string,
    name: string,
    text: string,
    branchIndex: number | null,
  ): void {
    if (!text.trim()) return;

    const jsxPath = buildJSXPath(ancestors, el, this.componentMap);
    const context =
      branchIndex === null ? `${jsxPath}[${name}]` : `${jsxPath}[${name}::${branchIndex}]`;
    const desc = locNote ? `${context}${CONTEXT_SEPARATOR}${locNote}` : context;
    const hash = hashKey(text, desc);
    if (this.seenHashes.has(hash)) return;
    this.seenHashes.add(hash);

    const idSuffix = branchIndex === null ? name : `${name}:${branchIndex}`;
    this.out.push({
      id: `${this.filename}:${lineFromOffset(this.code, el.span.start)}:${idSuffix}`,
      hash,
      translatable: true,
      type: "jsx:attribute",
      source: [{ text }] as Run[],
      placeholders: [],
      properties: blockProperties(this.filename, el, this.code, context, component, locNote),
    });
  }

  // ─── Source helpers ─────────────────────────────────────────

  private sliceSource(start: number, end: number): string {
    const a = start - this.spanBase;
    const b = end - this.spanBase;
    if (a < 0 || b <= a) return "";
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
): Block["properties"] {
  const properties: Block["properties"] = {
    file: filename,
    line: lineFromOffset(code, el.span.start),
    component,
    jsxPath,
    element: getTagName(el) ?? "",
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
    if (!node || typeof node !== "object") return;

    if (node.span && typeof node.span.start === "number" && base > 0) {
      node.span.start -= base;
      node.span.end -= base;
    }

    const pushedComponent = enterComponentScope(node, components);

    if (node.type === "JSXElement") {
      const el = node as JSXElement;
      const currentComponent = components[components.length - 1] ?? "";
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
          if (child.type === "JSXExpressionContainer") descend(child);
        }
      } else {
        for (const child of el.children ?? []) descend(child);
      }
      if (el.opening) descend(el.opening);
      if (el.closing) descend(el.closing);
      ancestors.pop();
    } else {
      for (const key of Object.keys(node)) {
        if (key === "type") continue;
        const val = (node as Record<string, unknown>)[key];
        if (Array.isArray(val)) for (const item of val) descend(item);
        else if (val && typeof val === "object" && "type" in val) descend(val);
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
    case "FunctionDeclaration":
      return pascal(node.identifier?.value);
    case "ExportDefaultDeclaration": {
      // Anonymous default export — we leave the name blank; the
      // walker will fall back to the file's basename.
      return null;
    }
    case "VariableDeclarator": {
      const nameValue = node.id?.value;
      const init = node.init;
      if (!init) return null;
      if (init.type !== "ArrowFunctionExpression" && init.type !== "FunctionExpression") {
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
  if (first >= "A" && first <= "Z") return name;
  return null;
}

/**
 * SWC reports `BytePos` offsets anchored to a process-global base that
 * grows with every `parseSync` call. For the first parse of a fresh
 * process the base is 1 (position 1 → byte 0); for subsequent parses
 * the base can be in the thousands, so we can't hardcode it.
 *
 * `module.span.start` points at the first non-whitespace byte, so it's
 * off by any leading whitespace — but leading whitespace doesn't
 * affect the line count used in warnings (a newline in whitespace
 * gets counted either way since lineFromOffset counts over the
 * input `code`, not the SWC view of it), and it's robust across
 * test-vs-standalone invocations. Accurate source slicing via
 * `sourceSlice` stays correct because each span is normalised by
 * the same offset.
 */
function findBaseOffset(module: Module): number {
  return module.span?.start ?? 1;
}

function tryParse(code: string, filename: string): Module | null {
  try {
    return parseSync(code, {
      syntax: filename.endsWith(".tsx") ? "typescript" : "ecmascript",
      tsx: true,
      jsx: true,
    });
  } catch {
    return null;
  }
}
