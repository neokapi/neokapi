import type { SpanInfo } from "../types/api";
import commonFormatting from "./common-formatting.json";
import richHtml from "./rich-html.json";
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

  loadDefaults(): void {
    this.load(commonFormatting as VocabularySchema);
    this.load(richHtml as VocabularySchema);
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
      chipLabel: { ...this.fallback.chipLabel },
      color: { ...this.fallback.color },
      equiv: "",
      constraints: { ...this.fallback.constraints },
    };
  }

  isEntityType(typeName: string): boolean {
    return typeName.startsWith(this.entityPrefix);
  }

  chipLabel(span: SpanInfo): string {
    const info = this.lookupOrFallback(span.type);
    switch (span.span_type) {
      case "opening":
        return info.chipLabel.open ?? "?>";
      case "closing":
        return info.chipLabel.close ?? "/?";
      case "placeholder":
        return info.chipLabel.placeholder ?? "?";
      default:
        return "?";
    }
  }

  chipColor(span: SpanInfo): ColorScheme {
    return this.lookupOrFallback(span.type).color;
  }

  htmlTag(span: SpanInfo): string | null {
    const info = this.lookupOrFallback(span.type);
    switch (span.span_type) {
      case "opening":
        return info.html.open ?? null;
      case "closing":
        return info.html.close ?? null;
      case "placeholder":
        return info.html.placeholder ?? null;
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
