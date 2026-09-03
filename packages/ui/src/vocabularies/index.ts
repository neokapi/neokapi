import type { SpanInfo } from "../types/span";
import commonFormatting from "./common-formatting.json";
import richHtml from "./rich-html.json";
import richJsx from "./rich-jsx.json";
import codeTokens from "./code-tokens.json";

// --- Vocabulary Schema Types ---

export interface HTMLRendering {
  open?: string;
  close?: string;
  placeholder?: string;
}

export interface TextRendering {
  open?: string;
  close?: string;
  placeholder?: string;
}

export interface ChipRendering {
  open?: string;
  close?: string;
  placeholder?: string;
}

export interface ColorScheme {
  bg: string;
  border: string;
  text: string;
}

export interface SpanConstraints {
  deletable: boolean;
  cloneable: boolean;
  reorderable: boolean;
}

export interface SpanTypeInfo {
  category: string;
  label: string;
  html: HTMLRendering;
  display: TextRendering;
  chipLabel: ChipRendering;
  color: ColorScheme;
  equiv: string;
  constraints: SpanConstraints;
}

interface FallbackDefinition {
  html: { open: string; close: string; placeholder: string };
  display: { open: string; close: string; placeholder: string };
  chipLabel: { open: string; close: string; placeholder: string };
  color: ColorScheme;
  constraints: SpanConstraints;
}

interface VocabularySchema {
  name: string;
  version: string;
  extends: string | null;
  entity_prefix?: string;
  types: Record<string, SpanTypeInfo>;
  fallback?: FallbackDefinition;
}

/**
 * Expand a vocabulary template over one span's fields. The substitutions are
 * the ones `core/kbf/preview.go` expands, so a chip in the browser and the
 * markup the engine writes name the same thing.
 *
 * `escape` is applied to the substituted values (not the template) where the
 * result is markup rather than text.
 */
function expand(
  template: string | undefined,
  span: SpanInfo,
  escape: (s: string) => string = (s) => s,
): string {
  if (!template) return "";
  const values: Record<string, string> = {
    id: span.id,
    subType: span.sub_type ?? "",
    data: span.data,
    equiv: span.equiv_text ?? "",
  };
  return template.replace(/\{(\w+)\}/g, (whole, key: string) =>
    key in values ? escape(values[key]) : whole,
  );
}

/** Minimal HTML escaping for values substituted into a markup template. */
function escapeHtml(s: string): string {
  return s
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

/** Derive a short chip label from a span type name (e.g. "fmt:bold" → "bold"). */
function shortChipLabel(
  typeName: string,
  fallback: { open: string; close: string; placeholder: string },
): { open: string; close: string; placeholder: string } {
  // Extract the part after the colon (e.g. "fmt:bold" → "bold", "struct:break" → "break").
  const short = typeName.includes(":") ? typeName.split(":").pop()! : typeName;
  return {
    open: `${short}>`,
    close: `/${short}`,
    placeholder: short || fallback.placeholder,
  };
}

// --- Vocabulary Registry ---

const defaultFallback: FallbackDefinition = {
  html: {
    open: '<span data-type="{type}">',
    close: "</span>",
    placeholder: '<span data-type="{type}"/>',
  },
  display: { open: "[{type}]", close: "[/{type}]", placeholder: "[{type}/]" },
  chipLabel: { open: "?>", close: "/?", placeholder: "?" },
  color: {
    bg: "rgba(156,163,175,0.15)",
    border: "rgba(156,163,175,0.5)",
    text: "rgb(107,114,128)",
  },
  constraints: { deletable: true, cloneable: true, reorderable: true },
};

export class VocabularyRegistry {
  private types = new Map<string, SpanTypeInfo>();
  private entityPrefix = "entity:";
  private fallback = defaultFallback;

  load(vocab: VocabularySchema): void {
    if (vocab.entity_prefix) {
      this.entityPrefix = vocab.entity_prefix;
    }
    if (vocab.fallback) {
      this.fallback = vocab.fallback;
    }
    for (const [name, info] of Object.entries(vocab.types)) {
      this.types.set(name, info);
    }
  }

  /**
   * The vocabularies every surface can meet: formatting, rich HTML, rich JSX
   * and code tokens. JSX belongs here for the same reason HTML does — a KBF
   * catalog extracted from a React tree carries `jsx:element` / `jsx:var` /
   * `jsx:node` codes, and a registry without them renders every variable in a
   * block as the same fallback chip ("var"), so a reviewer cannot tell one
   * variable from the next.
   */
  loadDefaults(): void {
    this.load(commonFormatting as VocabularySchema);
    this.load(richHtml as VocabularySchema);
    this.load(richJsx as VocabularySchema);
    this.load(codeTokens as VocabularySchema);
  }

  lookup(typeName: string): SpanTypeInfo | undefined {
    return this.types.get(typeName);
  }

  lookupOrFallback(typeName: string): SpanTypeInfo {
    const info = this.types.get(typeName);
    if (info) return info;
    return {
      category: "generic",
      label: typeName,
      html: {
        open: this.fallback.html.open.replace("{type}", typeName),
        close: this.fallback.html.close.replace("{type}", typeName),
        placeholder: this.fallback.html.placeholder.replace("{type}", typeName),
      },
      display: {
        open: this.fallback.display.open.replace("{type}", typeName),
        close: this.fallback.display.close.replace("{type}", typeName),
        placeholder: this.fallback.display.placeholder.replace("{type}", typeName),
      },
      chipLabel: shortChipLabel(typeName, this.fallback.chipLabel),
      color: { ...this.fallback.color },
      equiv: "",
      constraints: { ...this.fallback.constraints },
    };
  }

  isEntityType(typeName: string): boolean {
    return typeName.startsWith(this.entityPrefix);
  }

  /**
   * The chip a span reads as. A vocabulary states the label as a template over
   * the span's own fields — `jsx:var` is `{equiv}` — so the chip names the
   * variable it stands for rather than its type: four `{equiv}` chips in a
   * block are four different variables, and a reviewer can see which is which
   * without hovering each one.
   *
   * A template that expands to nothing (a span carrying no `equiv`) falls back
   * to the type's short name, so a chip is never blank.
   */
  chipLabel(span: SpanInfo): string {
    const info = this.lookupOrFallback(span.type);
    const fallback = shortChipLabel(span.type, this.fallback.chipLabel);
    switch (span.span_type) {
      case "opening":
        return expand(info.chipLabel.open, span) || fallback.open;
      case "closing":
        return expand(info.chipLabel.close, span) || fallback.close;
      case "placeholder":
        return expand(info.chipLabel.placeholder, span) || fallback.placeholder;
      default:
        return "?";
    }
  }

  /**
   * The span's bracketed display label — the symmetric `[TAG]` / `[/TAG]` form,
   * consistent in casing across every type. A reading surface uses this for the
   * chips it keeps (links, unknown pairs) so an opening never reads as `[A]`
   * beside a closing `/a`. Unlike {@link chipLabel}, the open and close forms
   * are a matched pair by construction.
   */
  displayLabel(span: SpanInfo): string {
    const info = this.lookupOrFallback(span.type);
    const fallback = this.fallback.display;
    switch (span.span_type) {
      case "opening":
        return expand(info.display.open, span) || fallback.open.replace("{type}", span.type);
      case "closing":
        return expand(info.display.close, span) || fallback.close.replace("{type}", span.type);
      case "placeholder":
        return (
          expand(info.display.placeholder, span) ||
          fallback.placeholder.replace("{type}", span.type)
        );
      default:
        return "?";
    }
  }

  /**
   * The span's text equivalent — what it reads as where markup cannot be shown
   * (a break's newline, a variable's name). Empty when the vocabulary states
   * none.
   */
  textEquiv(span: SpanInfo): string {
    return expand(this.lookupOrFallback(span.type).equiv, span);
  }

  chipColor(span: SpanInfo): ColorScheme {
    return this.lookupOrFallback(span.type).color;
  }

  /**
   * The markup a span renders as, with the vocabulary's template expanded from
   * the span (`<{subType} data-neokapi-span="{id}">` → `<span
   * data-neokapi-span="3">`). Values are HTML-escaped on the way in, mirroring
   * `core/kbf`'s renderer, because the result is concatenated into a document.
   */
  htmlTag(span: SpanInfo): string | null {
    const info = this.lookupOrFallback(span.type);
    switch (span.span_type) {
      case "opening":
        return info.html.open ? expand(info.html.open, span, escapeHtml) : null;
      case "closing":
        return info.html.close ? expand(info.html.close, span, escapeHtml) : null;
      case "placeholder":
        return info.html.placeholder ? expand(info.html.placeholder, span, escapeHtml) : null;
      default:
        return null;
    }
  }

  categories(): string[] {
    const cats = new Set<string>();
    for (const info of this.types.values()) {
      cats.add(info.category);
    }
    return [...cats];
  }

  typesInCategory(category: string): string[] {
    const result: string[] = [];
    for (const [name, info] of this.types) {
      if (info.category === category) {
        result.push(name);
      }
    }
    return result;
  }

  allTypes(): string[] {
    return [...this.types.keys()];
  }
}

// Singleton default registry.
let defaultRegistry: VocabularyRegistry | null = null;

export function getDefaultRegistry(): VocabularyRegistry {
  if (!defaultRegistry) {
    defaultRegistry = new VocabularyRegistry();
    defaultRegistry.loadDefaults();
  }
  return defaultRegistry;
}
