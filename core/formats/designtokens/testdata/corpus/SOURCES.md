# designtokens corpus — provenance

Genuine W3C Design Tokens Community Group (DTCG) token files vendored verbatim
from upstream open-source repositories for byte-faithful round-trip and
consumer-acceptance testing. All sources are permissively licensed.

| Local file | Upstream repo | License | Path |
|---|---|---|---|
| `style-dictionary-demo.tokens.json` | `amzn/style-dictionary` | Apache-2.0 | `docs/public/demo-tokens.json` |

## Pinned reference

- `amzn/style-dictionary` — tag `v5.4.1` (verified byte-identical to `main`
  at fetch time).

## Exact fetch command

```sh
curl -sSL "https://raw.githubusercontent.com/amzn/style-dictionary/v5.4.1/docs/public/demo-tokens.json" \
  -o style-dictionary-demo.tokens.json
```

## Constructs covered

`style-dictionary-demo.tokens.json` is the canonical style-dictionary DTCG demo
and exercises the full breadth of the format:

- **Simple types** — `color` (hex), `dimension` (px), `fontFamily`,
  `fontWeight` (keyword), `number`.
- **Composite types** — `typography`, `transition`, `border`, `strokeStyle`
  (incl. a `dashArray` object form), and `shadow` (incl. a multi-value array
  of aliases).
- **`cubicBezier`** and **`duration`** value types.
- **`$type` cascade** — group-level `$type` inherited by leaf tokens (the file
  declares `$type` on groups, not on every token).
- **Curly-brace aliases** — `{text.fonts.sans}`, `{colors.black}`, etc.

The file carries no `$description`, so it exercises the
zero-translatable-surface path (it still round-trips byte-for-byte). The
in-package fixture `../tokens.tokens.json` complements it with
`$description`/`$extensions`/`$deprecated`/per-token `$type` for the
translation-path invariants and the positive schema-validation check.

## Note on official-schema validation (acceptance test)

`acceptance_test.go` validates output against the OFFICIAL W3C DTCG JSON Schema
(2025.10), vendored under `../schema/dtcg/2025.10/` (see that directory's
`SOURCES.md`). The official schema cannot model the DTCG `$type` cascade, so a
cascade-only token with a bare ambiguous string value (e.g. the fontWeight
keyword `"thin"` in this demo) is rejected at the source — a documented JSON
Schema limitation, not a kapi defect. The acceptance contract is therefore
EQUIVALENCE: kapi's output validates iff the source did. The positive
validation path is exercised separately on `../tokens.tokens.json`, which the
official schema accepts at the source.
