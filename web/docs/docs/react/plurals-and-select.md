---
sidebar_position: 5
title: Plurals and Select
description: Count-aware and choice-based text in kapi-react — author plural forms and select expressions as React components (Plural, Zero, One, Other) without writing raw ICU strings.
keywords: [plurals, select, Plural component, ICU, i18n, kapi-react, count-aware, React i18n]
---

# Plurals and select

Count-aware and choice-based text, authored in React with per-form children. No raw ICU strings in your source.

## Plurals — the authoring form

```tsx
import { Plural, Zero, One, Two, Few, Many, Other } from "@neokapi/react/runtime";

<p>
  <Plural count={n}>
    <Zero>No messages</Zero>
    <One>1 message</One>
    <Other>{n} messages</Other>
  </Plural>
</p>;
```

At render time, `<Plural>` consults `Intl.PluralRules` for the active locale and picks the matching form. For English `n = 1` resolves to `<One>`, everything else to `<Other>`. For Arabic, `<Zero>`, `<One>`, `<Two>`, `<Few>`, `<Many>`, `<Other>` all have distinct rules.

## Select — choice-based text

```tsx
import { Select, Case, Other } from "@neokapi/react/runtime";

<p>
  <Select value={role}>
    <Case when="admin">Admin access</Case>
    <Case when="editor">Editor access</Case>
    <Case when="viewer">Viewer access</Case>
    <Other>No access</Other>
  </Select>
</p>;
```

`<Select>` picks the `<Case>` whose `when` prop equals the source value, or falls back to `<Other>`.

## Why children, not props

Traditional i18n libraries express plurals as a single ICU message:

```text
{count, plural, zero {No messages} one {1 message} other {# messages}}
```

That works, but mixes the source-language forms into an opaque string literal. kapi-react keeps each form as a JSX child so:

- **Source readability** — the English forms look like English in source, translators see English in the extract.
- **Inline elements per form** — you can write `<Other><strong>{n}</strong> items</Other>` with the strong tag preserved as a position token. ICU doesn't model that naturally.
- **Placeholders per form** — `{n}` is an expression, handled by the same placeholder machinery as any JSX text.

Under the hood the extractor emits the canonical ICU template that translators know. Source authors work in React; translators work in their CAT tool against ICU. Both views are correct.

## Mixing inline elements inside a plural form

```tsx
<p>
  <Plural count={items.length}>
    <Zero>Your cart is empty</Zero>
    <One>
      <strong>1</strong> item in your cart
    </One>
    <Other>
      <strong>{items.length}</strong> items in your cart
    </Other>
  </Plural>
</p>
```

Each form is extracted as a typed run sequence — `<strong>` becomes a position token (`{=m0}`), `{items.length}` becomes a placeholder (`{items.length}` or deduped like `{itemsLength}`). The translator can reorder the bold element freely within each form.

## Which forms to declare

Use the superset of forms your product actually ships. Languages you don't target today can still be added later without source changes: add the translation in the CAT tool, the runtime picks the right form.

| Language                  | Forms used                                   |
| ------------------------- | -------------------------------------------- |
| English                   | `one`, `other`                               |
| French                    | `one`, `other`                               |
| Russian                   | `one`, `few`, `many`, `other`                |
| Arabic                    | `zero`, `one`, `two`, `few`, `many`, `other` |
| Japanese, Korean, Chinese | `other` only                                 |

Source authors usually only need `one` + `other`, plus optional `zero` for UX polish ("No messages" vs. "0 messages"). Translators fill the rest per locale.

## `<Plural>` with a source pivot

Sometimes the count variable's name doesn't match what you want translators to reference:

```tsx
<Plural count={cart.items.length}>
  <One>1 item</One>
  <Other>{cart.items.length} items</Other>
</Plural>
```

The pivot variable becomes a placeholder named after the expression (`items.length` → `length`). If that's confusing for translators, rename via a local:

```tsx
const n = cart.items.length;
<Plural count={n}>
  <One>1 item</One>
  <Other>{n} items</Other>
</Plural>;
```

## Translator-authored plurals

Not every language needs plurals for every string. Italian uses the same form for 1 vs many in some constructs, Japanese has only one form, while German might want a count form where English didn't.

kapi-react's editor UI (`<PluralTargetEditor>`, shipped with `@neokapi/ui-primitives`) lets translators **upgrade** a flat target into a per-form target without any source change. A translator's German target for `<p>You have {n} messages</p>` can become:

```
zero:  Keine Nachrichten
one:   1 Nachricht
other: Sie haben {n} Nachrichten
```

while the English source stays flat. The target-side data model is handled by `@neokapi/kapi-format`'s `upgradeTargetToPlural` / `downgradePluralTarget` helpers. See [AD-008](/contribute/architecture/008-project-model) for the on-disk shape.

## Runtime resolution

`<Plural>` / `<Select>` are authoring components — at render time a locale switch doesn't remount them, it re-evaluates the form via `Intl.PluralRules` / the case map. `Intl.PluralRules` ships in every modern browser and Node, so no polyfill.

In inline mode the plugin resolves the pivot at build time when the dict is available, emitting just the form's JSX for the target locale. In runtime mode the full plural template stays in the bundle and resolves per render.

## Next

- [Pipeline](./pipeline) — how plural blocks round-trip through extract/translate/compile.
- [Runtime vs. inline modes](./modes) — when to pick which build mode.
