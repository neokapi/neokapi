# Mart: the shared sample dataset

Mart is the canonical sample used across neokapi and bowrain demos, docs,
videos, and WASM embeds. It exists so that every walkthrough draws on one
coherent fiction with realistic content, instead of inventing its own throwaway
three-string fixture. When a reader moves from a kapi page to a framework
explainer to a bowrain tour, they see the same product, the same strings, and
the same people.

## The fiction

KapiMart is a fictional storefront and order platform for small retailers. The
sample models a slice of its product surface: navigation, a product and cart and
checkout flow, an account area, error and empty states, and a short piece of
marketing copy. The content is written the way a real small-shop product would
write it, and that realism is the point. Note that the restrained prose guideline
in `docs/internals/brand-communication.md` governs documentation _about_ this
sample (including this README), not the in-fiction product copy itself.

## Two instances, one design

Two brand instances share the same source strings, structure, terms, and content
memory design. Only the product name differs per surface, which keeps the product
boundary clean in each context:

| Instance | Use it for | Product name in copy |
| --- | --- | --- |
| KapiMart | kapi and framework examples: CLI recipes, Claude videos, framework WASM embeds, the neokapi landing page | `KapiMart` |
| BowMart | bowrain examples, adding the multi-user cast and the collaboration / correction-loop history | `BowMart` |

The files in this directory carry the `KapiMart` name as the default instance.
For BowMart, substitute the product name at record time (a single
find-and-replace of `KapiMart`), since the strings, keys, locales, terms, and
content memory are identical by design. The terms list both names as
do-not-translate so a check passes whichever instance is in use.

## File layout

```
samples/mart/
├── README.md            # this file — the source of truth
├── src/
│   ├── en.json          # source (nested, i18next-style keys)
│   ├── about.md         # source prose document (marketing/about copy)
│   ├── fr.json          # French — complete
│   ├── de.json          # German — partial (~70%)
│   └── ja.json          # Japanese — partial (~30%)
├── context/
│   ├── terms.json       # the terms as a bundle
│   └── memory/
│       └── mart-fr.memory.json   # approved en→fr pairs, exact and near matches
└── brand/
    ├── forbidden-terms.txt   # one term per line, for a brand check
    └── README.md             # forbidden→preferred substitutions, naming rules
```

Two source formats exercise the engine's round-trip: `src/en.json` for
structured key-value content, and `src/about.md` for prose. The locale JSON
files share the exact same key set as the source.

`context/` is what a project reads in with `kapi store import ./context`.
`terms.json` carries the vocabulary as a bundle: the product and payment names
as concepts with the same term in every locale, the shop vocabulary with its
French, German and Japanese renderings, and the brand words as forbidden terms
beside the word to use instead. `memory/mart-fr.memory.json` is the content
memory in its own form: five approved English to French pairs, three that
match a catalog string exactly and two that are near matches, so a recycle
step has both kinds to show. The learning labs under `/learn` seed a project
from both; the brand files stay for the brand-check example.

## Locales and completion

| Locale | Completion | Why |
| --- | --- | --- |
| `en` | source | — |
| `fr` | complete | shows a finished locale and 100% content memory/leverage |
| `de` | ~70% | shows pending work and partial progress |
| `ja` | ~30% | shows an early-stage locale with most strings pending |

**Missing-translation convention:** every locale file carries the full key set;
an untranslated string is an **empty string** (`""`), never an absent key. This
keeps the key sets identical across locales, so a diff shows only value changes
and a "pending" count is simply the number of empty values.

The sample includes ICU messages so plural and select handling can be shown:

- `cart.summary` is an ICU **plural** (`{count, plural, ...}`).
- `checkout.pay_with` is an ICU **select** on the payment provider.

Plural categories follow CLDR per locale: French and German use `one`/`other`;
Japanese uses `other` only. Partial locales leave the ICU strings empty where
that key has not been translated yet.

## Terms and brand

`context/terms.json` gives each concept its French, German and Japanese terms
and a definition. The product names and the third-party brands PayPal and
Apple Pay carry the same term in every locale, which is how a do-not-translate
term is expressed. It also encodes the preference for "AI assistant" over
"Copilot".

`brand/forbidden-terms.txt` lists marketing words a brand check should flag;
`brand/README.md` gives the preferred substitutions and the naming rules.

## Cast (for BowMart)

BowMart adds three named people so multi-user collaboration, review handoff, and
the correction-learning loop can be shown with the same cast every time:

| Person | Role | Scope |
| --- | --- | --- |
| Maya | Translator | fr |
| Jonas | Reviewer | approves translations before publish |
| Priya | PM / admin | manages the project, languages, and members |

### Correction-loop example

This sequence is the recurring story for BowMart correction-loop demos:

1. The AI assistant suggests `tableau de bord` for the German string
   `account.title`, carrying a French term into German.
2. **Maya** corrects the German to `Übersicht` (consistent with the
   `dashboard` concept in `context/terms.json`, whose German term is
   `Übersicht`).
3. The correction becomes a rule: "Dashboard in de is `Übersicht`, not
   `tableau de bord`," recorded as a project check.
4. **Jonas** reviews and approves the rule.
5. On the next locale and the next time the term appears, the check enforces the
   approved translation automatically, so the same mistake is not repeated.

The terms back this loop: the rule that Maya's correction produces matches the
`dashboard` concept, so the demo's "learned rule" and the committed
`context/terms.json` agree.
