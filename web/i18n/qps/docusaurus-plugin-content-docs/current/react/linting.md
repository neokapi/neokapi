---
sidebar_position: 10
title: neokapi-i18n Linting
description: i18n-react-lint provides ESLint and oxlint rules that catch unextractable strings, unsafe patterns, and authoring mistakes before they reach the translator — with CI enforcement via --strict.
keywords: [linting, i18n-react-lint, ESLint, oxlint, i18n lint rules, extraction, CI enforcement]
---

# ▒ Ļîñţîñĝ ▒

▒ ñéöķàþî-î18ñ'š ƃüîļđ-ţîḿé ţŕàñšƒöŕḿ çàţçĥéš à ļöţ, ƃüţ šöḿé àüţĥöŕîñĝ ḿîšţàķéš öñļý šĥöŵ üþ _àƒţéŕ_ éẋţŕàçţîöñ (à `ţ(ṽàŕîàƃļé)` ţĥàţ çàñ'ţ ƃé éẋţŕàçţéđ, à ļàƃéļ šţŕîñĝ ĥîđđéñ îñ à đàţà àŕŕàý ţĥàţ ţĥé éẋţŕàçţöŕ ñéṽéŕ ŵàļķš, à ţéŕñàŕý ţĥàţ šḿüĝĝļéš ţŵö ļîţéŕàļš þàšţ ţĥé ĴŠẊ ŵàļķéŕ). `@ñéöķàþî/î18ñ-ŕéàçţ-ļîñţ` ĝîṽéš ýöü éđîţöŕ šǫüîĝĝļîéš ƒöŕ ţĥöšé çàšéš. ▒

▒ Ţĥé šàḿé ŕüļé öƃĵéçţš ŵöŕķ üñđéŕ **ÉŠĻîñţ** àñđ **öẋļîñţ** — öẋļîñţ'š þļüĝîñ ÀÞÎ îš ÉŠĻîñţ ṽ9 çöḿþàţîƃļé, šö ýöü îñšţàļļ öñé þļüĝîñ àñđ ŵîŕé îţ îñţö ŵĥîçĥéṽéŕ ļîñţéŕ ýöü àļŕéàđý üšé. Öẋļîñţ îš ŕéçöḿḿéñđéđ ƒöŕ šþééđ (ţýþîçàļļý 100–200ḿš öñ à ƒéŵ ĥüñđŕéđ ƒîļéš). ▒

## ▒ Ţĥé ţĥŕéé ļàýéŕš ▒

| Layer               | When it runs                 | What catches                                                                                                                           | How loud                                                                   |
| ------------------- | ---------------------------- | -------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------- |
| **Lint rules**      | editor / `oxlint` / `eslint` | single-file authoring mistakes (unextractable `t()` calls, concat in translatable attrs, ternaries smuggling literals past extraction) | per-rule severity in your config                                           |
| **Plugin warnings** | build-time transform         | cross-cutting issues that need config context (unmapped components → componentMap, ternary attrs the extractor can't resolve)          | `console.warn` by default                                                  |
| **Enforcement**     | CI                           | both of the above, promoted to errors                                                                                                  | `--strict` on the extract CLI or `warningsAsErrors: true` in plugin config |

▒ Ķééþ ţĥé ļöüđéšţ ļàýéŕ (éñƒöŕçéḿéñţ) öƒƒ îñ đàý-ţö-đàý àüţĥöŕîñĝ, àñđ ţüŕñ îţ öñ îñ ÇÎ öñçé ţĥé çöđéƃàšé îš çļéàñ. ▒

## ▒ Îñšţàļļ ▒

```bash
vp install -D @neokapi/i18n-react-lint
```

## ▒ Öẋļîñţ ▒

▒ Àđđ ţö `.öẋļîñţŕç.ĵšöñ`: ▒

```json
{
  "jsPlugins": ["@neokapi/i18n-react-lint/oxlint"],
  "rules": {
    "neokapi-i18n/t-literal-first-arg": "error",
    "neokapi-i18n/t-no-concat": "error",
    "neokapi-i18n/no-concat-in-translatable-attr": "error",
    "neokapi-i18n/no-ternary-in-translatable-attr": "error",
    "neokapi-i18n/no-ternary-literals-in-jsx-child": "error",
    "neokapi-i18n/no-string-literal-jsx-expr": "warn",
    "neokapi-i18n/prefer-t-for-label-expr": "warn"
  },
  "overrides": [
    {
      "files": ["src/stories/**"],
      "rules": {
        "neokapi-i18n/no-ternary-literals-in-jsx-child": "off",
        "neokapi-i18n/prefer-t-for-label-expr": "off"
      }
    }
  ]
}
```

▒ Ţĥé `öṽéŕŕîđéš` ƃļöçķ đîšàƃļéš ţĥé ţŵö ĥîĝĥéŕ-ƑÞ ŕüļéš ƒöŕ Šţöŕýƃööķ ƒîẋţüŕé ƒîļéš, ŵĥéŕé đéḿö šţŕîñĝš đöñ'ţ ŵàŕŕàñţ ţĥé šàḿé ŕîĝöŕ. ▒

## ▒ ÉŠĻîñţ (ƒļàţ çöñƒîĝ) ▒

```js title="eslint.config.js"
import { recommended } from "@neokapi/i18n-react-lint/eslint";

export default [
  {
    files: ["**/*.{ts,tsx,js,jsx}"],
    languageOptions: {
      ecmaVersion: 2023,
      sourceType: "module",
      parserOptions: { ecmaFeatures: { jsx: true } },
    },
  },
  recommended,
];
```

▒ Ţĥé šĥàŕéàƃļé çöñƒîĝš àŕé `ŕéçöḿḿéñđéđ` (šàƒé đéƒàüļţš — ţĥé ƒîṽé çöŕé ŕüļéš àţ `éŕŕöŕ`, ţĥé ţŵö ļàƃéļ ŕüļéš àţ `ŵàŕñ`, `þŕéƒéŕ-ţ-ƒöŕ-ļàƃéļ-þŕöþš` öƒƒ) àñđ `ŕéçöḿḿéñđéđŠţŕîçţ` (éṽéŕýţĥîñĝ àţ `éŕŕöŕ`, îñçļüđîñĝ `þŕéƒéŕ-ţ-ƒöŕ-ļàƃéļ-þŕöþš` àñđ `þŕéƒéŕ-ţ-ƒöŕ-ļàƃéļ-éẋþŕ`). ▒

## ▒ Ţĥé Ŵ3Ç `ţŕàñšļàţé="ñö"` éšçàþé ĥàţçĥ ▒

▒ Àļļ ŕüļéš îñ ţĥîš þàçķàĝé ŕéšþéçţ `ţŕàñšļàţé="ñö"` öñ ţĥé éļéḿéñţ îţšéļƒ **öŕ àñý ĴŠẊ àñçéšţöŕ**. Ţĥé ñéöķàþî-î18ñ éẋţŕàçţöŕ àļŕéàđý ĥöñöüŕš îţ; ţĥé ļîñţ ŕüļéš ḿàţçĥ ţĥöšé šéḿàñţîçš. ▒

```tsx
// Rule fires — {meta.label} looks like user-visible copy.
<h1>{meta.label}</h1>

// Rule silent — author explicitly marked the subtree as non-translatable.
<h1 translate="no">{meta.label}</h1>

// Rule silent — an ancestor opted out, so the whole subtree is quiet.
<section translate="no">
  <div>
    <h1>{meta.label}</h1>
  </div>
</section>
```

▒ Üšé îţ ƒöŕ đàţà ţĥàţ'š ļéĝîţîḿàţéļý đýñàḿîç — ƃàçķéñđ îđéñţîƒîéŕš, ƒîļé þàţĥš, ṽéŕšîöñ šţŕîñĝš, üšéŕ-þŕöṽîđéđ ñàḿéš — ŵîţĥöüţ þöļļüţîñĝ ţĥé ļîñţ çöñƒîĝ ŵîţĥ þéŕ-ļîñé đîšàƃļéš. ▒

## ▒ Ŕüļéš ▒

### ▒ `ţ-ļîţéŕàļ-ƒîŕšţ-àŕĝ` ▒

▒ Ƒļàĝš `ţ(ṽàŕîàƃļé)` / `ţ(ĝéţĻàƃéļ())` / `ţ(çöñđ ? 'À' : 'Ƃ')`. Ţĥé éẋţŕàçţöŕ ŕéàđš ţĥé ƒîŕšţ àŕĝüḿéñţ öƒ `ţ()` šţàţîçàļļý àţ ƃüîļđ ţîḿé; àñýţĥîñĝ ţĥàţ îšñ'ţ à ļîţéŕàļ þŕöđüçéš ñöţĥîñĝ ţö ţŕàñšļàţé. ▒

```tsx
// ✓ fine
t("Sign in");
t("Sign in", "Button label"); // with context

// ✗ not extractable
t(label);
t(labels[key]);
t(ok ? "Save" : "Cancel");
```

### ▒ `ţ-ñö-çöñçàţ` ▒

▒ Ƒļàĝš `ţ('Ĥéļļö ' + ñàḿé)` àñđ ``ţ(`Ĥéļļö ${name}`)`` — ñéîţĥéŕ éẋţŕàçţš ƃéçàüšé ţĥé ƒüļļ šţŕîñĝ îšñ'ţ ṽîšîƃļé àţ ƃüîļđ ţîḿé. Üšé à þļàçéĥöļđéŕ þàţţéŕñ îñšţéàđ. ▒

```tsx
// ✗ broken
t("Welcome " + user.name);
t(`You have ${count} messages`);

// ✓ extractable, rendered via runtime substitution
t("Welcome {name}", { name: user.name });
// or use <Plural>/<Select> for pluralisation
```

### ▒ `ñö-çöñçàţ-îñ-ţŕàñšļàţàƃļé-àţţŕ` ▒

▒ Àñý àţţŕîƃüţé îñ ñéöķàþî-î18ñ'š ţŕàñšļàţàƃļé-àţţŕîƃüţé šéţ (`àļţ`, `ţîţļé`, `þļàçéĥöļđéŕ`, `àŕîà-ļàƃéļ`, `ļàƃéļ`, `đéšçŕîþţîöñ`, `ĥéļþŢéẋţ`, …) ḿüšţ ƃé à šţŕîñĝ ļîţéŕàļ öŕ à ļîţéŕàļ ŵîţĥ þļàçéĥöļđéŕš — ñöţ à ŕüñţîḿé çöñçàţ. ▒

```tsx
// ✗ alt won't extract
<img alt={'Logo ' + brand} />

// ✓ if you need dynamic parts, compute via t() and pass the result
<img alt={t('Logo for {brand}', { brand })} />
```

### ▒ `ñö-ţéŕñàŕý-îñ-ţŕàñšļàţàƃļé-àţţŕ` ▒

▒ Šîƃļîñĝ öƒ `ñö-çöñçàţ-îñ-ţŕàñšļàţàƃļé-àţţŕ`. Ƒļàĝš ţŕàñšļàţàƃļé àţţŕîƃüţéš ŵĥöšé ṽàļüé îš à ţéŕñàŕý ŵîţĥ àţ ļéàšţ öñé _ñöñ-šţŕîñĝ-ļîţéŕàļ_ ƃŕàñçĥ. Ţĥé àļļ-šţŕîñĝ-ļîţéŕàļ çàšé (`ţîţļé={cond ? "A" : "B"}`) îš éẋţŕàçţéđ ƃý ţĥé ñéöķàþî-î18ñ ŵàļķéŕ àš ţŵö ƃļöçķš — ñö ŵàŕñîñĝ. Ţĥé ḿîẋéđ çàšé îš üñéẋţŕàçţàƃļé. ▒

```tsx
// ✓ both branches are string literals — extractor handles them.
<PageHeader title={isProjectMode ? "Project Flows" : "Flows"} />

// ✓ both branches are t() calls — the t-call walker handles them.
<Input placeholder={disabled ? t("Off") : t("On")} />

// ✗ one literal, one computed — the computed branch silently bypasses translation.
<Input placeholder={disabled ? getLabel() : "Type here…"} />
```

▒ Ƒîẋ ƃý ŵŕàþþîñĝ ţĥé çöḿþüţéđ ƃŕàñçĥ ŵîţĥ `ţ()` ţöö, öŕ ƃý ļîƒţîñĝ ţĥé ļöĝîç šö ƃöţĥ ƃŕàñçĥéš ŕéšöļṽé ţö šţŕîñĝ ļîţéŕàļš. ▒

### ▒ `ñö-ţéŕñàŕý-ļîţéŕàļš-îñ-ĵšẋ-çĥîļđ` ▒

▒ Çàţçĥéš ţĥé ĴŠẊ-çĥîļđŕéñ çöüñţéŕþàŕţ öƒ ţĥé àţţŕîƃüţé ŕüļé: ▒

```tsx
// ✗ neither literal gets extracted — the extractor treats the
// whole conditional as a single opaque placeholder.
<Button>{loading ? "Saving..." : "Save"}</Button>
```

▒ Ŵĥý ţĥîš šļîþš ţĥŕöüĝĥ éṽéŕýţĥîñĝ éļšé: ñéöķàþî-î18ñ'š ŵàļķéŕ šééš öñé `ĴŠẊÉẋþŕéššîöñÇöñţàîñéŕ` àñđ éḿîţš öñé `ĵšẋ:ṽàŕ` þļàçéĥöļđéŕ ƒöŕ îţ. Îţ ñéṽéŕ ļööķš îñšîđé àţ ţĥé ƃŕàñçĥéš — `"Šàṽîñĝ..."` àñđ `"Šàṽé"` àŕé ƃöţĥ îñṽîšîƃļé ţö éẋţŕàçţîöñ. ▒

▒ Ƒîẋ ŵîţĥ `ţ()`: ▒

```tsx
// ✓ each branch extracts as its own block; the branch's value flows through
// the button's `__tx` call at render time.
<Button>{loading ? t("Saving...") : t("Save")}</Button>
```

▒ Ṽàŕîàñţš ţĥé ŕüļé ĥàñđļéš çļéàñļý: ▒

- ▒ Ƃöţĥ ƃŕàñçĥéš šţŕîñĝ ļîţéŕàļš → ƒļàĝĝéđ (éîţĥéŕ/ƃöţĥ ļöšţ). ▒
- ▒ Öñé šţŕîñĝ ļîţéŕàļ, öñé `ţ()` çàļļ → ƒļàĝĝéđ (ţĥé ļîţéŕàļ ƃŕàñçĥ îš ļöšţ). ▒
- ▒ Ƃöţĥ `ţ()` çàļļš → **ñöţ** ƒļàĝĝéđ (ĝöéš ţĥŕöüĝĥ ţĥé ţ-çàļļ þàţĥ). ▒
- ▒ Ţéḿþļàţé ļîţéŕàļš ŵîţĥ àļþĥàƃéţîç ţéẋţ (`` `Ļöàđîñĝ ${n}...` ``) → ƒļàĝĝéđ. ▒
- ▒ Ƒöŕḿàţ-öñļý ţéḿþļàţéš ŵîţĥ ñö àļþĥàƃéţîç ǫüàšî (`` `${pct}%` ``, `` `ṽ${version}` ``) → **ñöţ** ƒļàĝĝéđ (çöđé-ļéṽéļ ƒöŕḿàţţîñĝ, ñöţ ÜÎ çöþý). ▒

### ▒ `ñö-šţŕîñĝ-ļîţéŕàļ-ĵšẋ-éẋþŕ` ▒

▒ `<þ>{'Hello'}</þ>` — à ƃàŕé šţŕîñĝ ļîţéŕàļ ŵŕàþþéđ îñ àñ éẋþŕéššîöñ çöñţàîñéŕ. Ļööķš éẋţŕàçţàƃļé ƃüţ îšñ'ţ: ţĥé ţŕàñšƒöŕḿ ŵàļķš ĴŠẊ ţéẋţ ñöđéš, ñöţ éẋþŕéššîöñ çöñţàîñéŕš ţĥàţ ĥàþþéñ ţö ĥöļđ à šţŕîñĝ. Àüţö-ƒîẋéš ţö `<þ>Ĥéļļö</þ>`. ▒

### ▒ `þŕéƒéŕ-ţ-ƒöŕ-ļàƃéļ-éẋþŕ` ▒

▒ Ţĥé ŕéñđéŕ-šîđé çöḿþàñîöñ ţö `þŕéƒéŕ-ţ-ƒöŕ-ļàƃéļ-þŕöþš` ƃéļöŵ. Ƒļàĝš `{obj.label}` / `{item.title}` / `{entry.caption}` ŕéñđéŕéđ àš ĴŠẊ ţéẋţ: ▒

```tsx
// ✗ `meta.label` looks user-visible; the extractor can't see the string
// it will resolve to at runtime.
<h1>{meta.label}</h1>;

// ✓ wrap the source data so the literal is visible to extraction
const categoryMeta = {
  utility: { label: t("Utility") },
  // …
};
```

▒ Öñļý ƒîŕéš öñ à ñàŕŕöŵ šéţ öƒ þŕöþéŕţý ñàḿéš ţĥàţ _àļḿöšţ àļŵàýš_ ñàḿé üšéŕ-ṽîšîƃļé çöþý: `ļàƃéļ`, `ţîţļé`, `ĥéàđîñĝ`, `çàþţîöñ`, `šüƃţîţļé`, `ţööļţîþ`, `þļàçéĥöļđéŕ`, `šüḿḿàŕý`. Đéļîƃéŕàţéļý éẋçļüđéš `.ñàḿé`, `.đéšçŕîþţîöñ`, `.ţéẋţ`, `.ḿéššàĝé` — ţĥöšé öṽéŕŵĥéļḿîñĝļý ñàḿé ƃàçķéñđ / ŕüñţîḿé đàţà îñ ŕéàļ Ŕéàçţ àþþš àñđ ŵöüļđ çŕéàţé ţöö ḿüçĥ ñöîšé. ▒

▒ Çüšţöḿîšé ṽîà ţĥé `ķéýš` öþţîöñ: ▒

```json
{ "rules": { "neokapi-i18n/prefer-t-for-label-expr": ["warn", { "keys": ["label", "cta"] }] } }
```

▒ Šüþþŕéšš ƒàļšé þöšîţîṽéš öñ à šþéçîƒîç éļéḿéñţ ŵîţĥ `ţŕàñšļàţé="ñö"`: ▒

```tsx
// file.name is an OS path, not UI copy
<option value={f.path} translate="no">
  {f.name}
</option>
```

### ▒ `þŕéƒéŕ-ţ-ƒöŕ-ļàƃéļ-þŕöþš` ▒

▒ Ţĥé çļàššîç "ļàƃéļ ĥîđđéñ îñ à đàţà àŕŕàý" þàţţéŕñ — ţĥé _đéçļàŕàţîöñ šîđé_ öƒ ţĥé šàḿé îđéà: ▒

```tsx
// ✗ 'System' never gets extracted
const THEMES = [
  { value: "system", label: "System" },
  { value: "light", label: "Light" },
];
return THEMES.map(({ value, label }) => <button>{label}</button>);

// ✓ the literals are now visible to extraction
const THEMES = [
  { value: "system", label: t("System") },
  { value: "light", label: t("Light") },
];
```

▒ Öñļý îñ ţĥé `ŕéçöḿḿéñđéđŠţŕîçţ` þŕéšéţ ƃý đéƒàüļţ ƃéçàüšé îţ çàñ ƒîŕé öñ îñţéŕñàļ-öñļý đàţà àŕŕàýš. Šàḿé ñàŕŕöŵ ķéý ļîšţ àš `þŕéƒéŕ-ţ-ƒöŕ-ļàƃéļ-éẋþŕ`. Ţüŕñ öñ îñđîṽîđüàļļý: ▒

```json
{ "rules": { "neokapi-i18n/prefer-t-for-label-props": "error" } }
```

### ▒ Ḿöđüļé-ļéṽéļ `ţ()` ĝöţçĥà ▒

▒ Àļļ `ţ()`-ŵŕàþþîñĝ ƒîẋéš àƃöṽé àššüḿé ţĥé çàļļš ĥàþþéñ **þéŕ ŕéñđéŕ**. À ḿöđüļé-ļéṽéļ çöñšţ ƒŕééžéš éàçĥ `ţ()` çàļļ àţ ŵĥàţéṽéŕ ţĥé đîçţ šàîđ ŵĥéñ ţĥé ḿöđüļé ƒîŕšţ ļöàđéđ — ţýþîçàļļý ţĥé ƒàļļƃàçķ ļàñĝüàĝé, ƃéçàüšé ţŕàñšļàţîöñš ļöàđ _àƒţéŕ_ ţĥé îñîţîàļ îḿþöŕţ. ▒

```tsx
// ✗ Frozen at load time. "Utility" will still say "Utility" in pseudo.
const categoryMeta = {
  utility: { label: t("Utility") },
};

// ✓ Per-render: each invocation picks up the current dict.
function categoryMeta(cat: string) {
  switch (cat) {
    case "utility":
      return { label: t("Utility") };
    // …
  }
}
```

▒ Ŵŕàþ ñöñ-ţŕîṽîàļ ļööķüþ ţàƃļéš îñ à ƒüñçţîöñ ţĥàţ ŕéţüŕñš ƒŕéšĥ ṽàļüéš þéŕ ŕéñđéŕ. Šéé [Ţĥé `ţ()` éšçàþé ĥàţçĥ → Ḿöđüļé-ļéṽéļ ĝöţçĥà](./t-escape-hatch#module-level-t-gotcha) ƒöŕ ḿöŕé. ▒

## ▒ ÇÎ éñƒöŕçéḿéñţ ▒

▒ Ţŵö ŵàýš ţö ƒàîļ ţĥé ƃüîļđ öñ ŵàŕñîñĝš: ▒

▒ **Ļîñţ šţéþ:** ▒

```bash
vp lint                  # or: oxlint / eslint
```

▒ Ñöñ-žéŕö éẋîţ ŵĥéñ àñý ŕüļé àţ šéṽéŕîţý `éŕŕöŕ` ƒîŕéš. Ŵîŕé îţ àļöñĝšîđé ýöüŕ ţýþéçĥéçķ šţéþ îñ ÇÎ (öŕ ŕüñ îţ ƒŕöḿ à Ĝîţ þŕé-çöḿḿîţ ĥööķ). ▒

▒ **Éẋţŕàçţ ÇĻÎ:** ▒

```bash
vpx neokapi-i18n extract --strict
```

▒ Éẋîţš ñöñ-žéŕö îƒ ţĥé éẋţŕàçţöŕ ŕéçöŕđéđ àñý ŵàŕñîñĝ (`üñķñöŵñ-çöḿþöñéñţ`, `ţéŕñàŕý-àţţŕ-çöḿþļéẋ`, `đýñ-ļàƃéļ-šþļîçé`). Ĝööđ ƒöŕ çàţçĥîñĝ àüţĥöŕîñĝ îššüéš ţĥé ļîñţ ŕüļéš çàñ'ţ šéé ƒŕöḿ à šîñĝļé ƒîļé. ▒

▒ **Þļüĝîñ (ƃüîļđ-ţîḿé):** ▒

```ts title="vite.config.ts"
import neokapi from "@neokapi/i18n-react/vite";

export default {
  plugins: [neokapi({ warningsAsErrors: process.env.CI === "true" })],
};
```

▒ Þŕöḿöţéš ţŕàñšƒöŕḿ-šîđé ŵàŕñîñĝš (üñķñöŵñ-çöḿþöñéñţ, éţç.) ţö ţĥŕöŵñ ƃüîļđ éŕŕöŕš. Üšé `þŕöçéšš.éñṽ.ÇÎ` ţö ķééþ ļöçàļ đéṽ éŕĝöñöḿîç. ▒

## ▒ Éẋçļüđîñĝ ƒîẋţüŕé çöđé ▒

▒ Šţöŕîéš, ḿöçķš, àñđ ƒîẋţüŕéš đöñ'ţ üšüàļļý ŵàŕŕàñţ ţĥé šàḿé î18ñ ŕîĝöŕ àš šĥîþþéđ çöḿþöñéñţš. Ţŵö çöḿþļéḿéñţàŕý ŵàýš ţö éẋçļüđé ţĥéḿ: ▒

▒ **Ƒŕöḿ ļîñţ** — `.öẋļîñţŕç.ĵšöñ` `öṽéŕŕîđéš` ƃļöçķ (šéé àƃöṽé). ▒

▒ **Ƒŕöḿ éẋţŕàçţîöñ** — `--îĝñöŕé` ƒļàĝ: ▒

```json title="package.json"
{
  "scripts": {
    "extract": "vpx neokapi-i18n extract --out i18n/ --ignore 'src/stories/**' --ignore '**/*.test.tsx'"
  }
}
```

▒ Ţĥé ƒļàĝ îš ŕéþéàţàƃļé àñđ þàššéđ ţĥŕöüĝĥ ţö Ñöđé'š `ƒš/þŕöḿîšéš.ĝļöƃ` `éẋçļüđé` öþţîöñ. ▒

## ▒ Ƒöļļöŵ-üþš ▒

▒ Ţĥéšé ŕüļéš àŕé þļàññéđ ƃüţ ñöţ ýéţ šĥîþþéđ — ţĥéý ñééđ ŢýþéŠçŕîþţ ţýþé îñƒöŕḿàţîöñ öŕ çŕöšš-ƒîļé àñàļýšîš ţĥàţ à šîḿþļé ÉŠĻîñţ ŕüļé çàñ'ţ đö öñ îţš öŵñ: ▒

- ▒ `ţŕàñšļàţàƃļé-àţţŕ-éẋþéçţš-šţŕîñĝ` — çàţçĥ `<ÞàĝéĤéàđéŕ ţîţļé={x} />` ŵĥéŕé `ẋ` îš `ŔéàçţÑöđé`, ñöţ `šţŕîñĝ` ▒
- ▒ `üñḿàþþéđ-çöḿþöñéñţ-îñ-éđîţöŕ` — ḿîŕŕöŕ ţĥé þļüĝîñ'š `üñķñöŵñ-çöḿþöñéñţ` ŵàŕñîñĝ îñ ţĥé éđîţöŕ ▒
- ▒ `üñüšéđ-çöḿþöñéñţḿàþ-éñţŕý` — ƒļàĝ çöḿþöñéñţḾàþ ķéýš ţĥàţ ñö šöüŕçé ƒîļé ŕéƒéŕéñçéš ▒

▒ Ţŕàçķ þŕöĝŕéšš öñ [îššüé #381](https://github.com/neokapi/neokapi/issues/381). ▒
