/**
 * Plural / Select authoring components.
 *
 * A developer writes children-based JSX:
 *
 *   <Plural count={items.length}>
 *     <Zero>Your cart is empty</Zero>
 *     <One>1 item in your cart</One>
 *     <Other><strong>{items.length}</strong> items in your cart</Other>
 *   </Plural>
 *
 *   <Select value={user.role}>
 *     <Case when="admin">Welcome, admin</Case>
 *     <Case when="guest">You're browsing as a guest</Case>
 *     <Other>Welcome, {user.name}!</Other>
 *   </Select>
 *
 * At build time the kapi-react plugin rewrites these into `__tx()` calls
 * whose flat ICU template is what the runtime dictionary keys on —
 * that's the fast path. At dev time (no plugin, no build step) the
 * components fall back to `Intl.PluralRules` / string match here so
 * the UI renders the correct form without requiring a build.
 */

import type { ReactElement, ReactNode } from "react";
import { Children, isValidElement } from "react";

// ─── Plural ──────────────────────────────────────────────────

/**
 * ICU plural-rule keywords. `'other'` is the required catch-all;
 * every plural form the runtime can receive is one of these.
 */
export type PluralFormKey = "zero" | "one" | "two" | "few" | "many" | "other";

export interface PluralProps {
  /** The integer that drives form selection (CLDR plural rules). */
  count: number;
  children: ReactNode;
  /** Override the active locale. Defaults to the runtime's current locale. */
  locale?: string;
}

/**
 * Dev-mode render: walks children for the ICU form that matches
 * `count` under the active locale's plural rules and renders that
 * form's body. At build time the plugin replaces the whole
 * `<Plural>` tree with a `__tx(hash, template, params)` call.
 */
export function Plural(props: PluralProps): ReactNode {
  const key = pluralKeyFor(props.count, props.locale);
  return pickForm(props.children, key, PluralFormTags);
}

Plural.displayName = "Plural";

// Per-form sub-components — each is a passthrough renderer. The
// extractor + plugin recognize them by tag name; the runtime recognizes
// them by `displayName` for the dev-mode fallback.

function makeFormComponent(tag: string) {
  const Component = ({ children }: { children?: ReactNode }) => <>{children}</>;
  Component.displayName = tag;
  return Component;
}

export const Zero = makeFormComponent("Zero");
export const One = makeFormComponent("One");
export const Two = makeFormComponent("Two");
export const Few = makeFormComponent("Few");
export const Many = makeFormComponent("Many");
export const Other = makeFormComponent("Other");

const PluralFormTags: Record<string, PluralFormKey> = {
  Zero: "zero",
  One: "one",
  Two: "two",
  Few: "few",
  Many: "many",
  Other: "other",
};

// ─── Select ──────────────────────────────────────────────────

export interface SelectProps {
  /** The discriminator value. Matched against each `<Case when="…">`. */
  value: string;
  children: ReactNode;
}

export function Select(props: SelectProps): ReactNode {
  return pickCase(props.children, props.value);
}

Select.displayName = "Select";

export interface CaseProps {
  /** The value this case matches. */
  when: string;
  children?: ReactNode;
}

export function Case({ children }: CaseProps): ReactNode {
  return <>{children}</>;
}

Case.displayName = "Case";

// ─── Form / case selection ────────────────────────────────────

type FormChild = ReactElement<{ children?: ReactNode }>;

function pickForm(
  children: ReactNode,
  targetKey: PluralFormKey,
  tagMap: Record<string, PluralFormKey>,
): ReactNode {
  let fallback: ReactNode = null;
  for (const child of Children.toArray(children)) {
    if (!isValidElement(child)) continue;
    const name = componentDisplayName(child);
    if (!name) continue;
    const formKey = tagMap[name];
    if (!formKey) continue;
    if (formKey === targetKey) return (child as FormChild).props.children ?? null;
    if (formKey === "other") fallback = (child as FormChild).props.children ?? null;
  }
  return fallback;
}

function pickCase(children: ReactNode, value: string): ReactNode {
  let fallback: ReactNode = null;
  for (const child of Children.toArray(children)) {
    if (!isValidElement(child)) continue;
    const name = componentDisplayName(child);
    if (name === "Case") {
      const props = (child as ReactElement<CaseProps>).props;
      if (props.when === value) return props.children ?? null;
    } else if (name === "Other") {
      fallback = (child as FormChild).props.children ?? null;
    }
  }
  return fallback;
}

function componentDisplayName(el: ReactElement): string | undefined {
  const type = el.type as { displayName?: string; name?: string } | string;
  if (typeof type === "string") return type;
  return type.displayName ?? type.name;
}

// ─── CLDR plural-rule helper ─────────────────────────────────

/**
 * Returns the ICU plural-rule form key (one of the six CLDR keywords)
 * for `n` under `locale`. Uses `Intl.PluralRules` when available;
 * falls back to a crude English rule otherwise.
 */
export function pluralKeyFor(n: number, locale?: string): PluralFormKey {
  const loc = locale ?? inferLocale();
  try {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const rules = new (Intl as any).PluralRules(loc);
    return rules.select(n) as PluralFormKey;
  } catch {
    // Minimal English fallback.
    if (n === 0) return "zero";
    if (n === 1) return "one";
    return "other";
  }
}

function inferLocale(): string {
  if (typeof document !== "undefined") {
    const lang = document.documentElement?.lang;
    if (lang) return lang;
  }
  if (typeof navigator !== "undefined") {
    const langs = navigator.languages;
    if (langs && langs.length > 0) return langs[0];
    if (navigator.language) return navigator.language;
  }
  return "en";
}
