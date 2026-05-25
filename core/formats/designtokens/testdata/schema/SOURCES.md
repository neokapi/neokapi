# DTCG JSON Schema — provenance

The OFFICIAL W3C Design Tokens Community Group (DTCG) Format JSON Schema, stable
revision **2025.10**, vendored verbatim under `dtcg/2025.10/` and used by
`acceptance_test.go` (via `ajv-cli`) to validate kapi's design-tokens output.

| Local tree | Upstream repo | License | Upstream path |
|---|---|---|---|
| `dtcg/2025.10/**` | `design-tokens/community-group` | W3C Software and Document License | `www/public/schemas/2025.10/**` |

The W3C Software and Document License is permissive (allows copying,
modification, and redistribution with attribution).

## Files vendored

```
dtcg/2025.10/format.json                       # root schema
dtcg/2025.10/format/group.json
dtcg/2025.10/format/groupOrToken.json
dtcg/2025.10/format/token.json
dtcg/2025.10/format/tokenType.json
dtcg/2025.10/format/values/{border,color,cubicBezier,dimension,duration,
                            fontFamily,fontWeight,gradient,number,shadow,
                            strokeStyle,transition,typography}.json
```

The `$ref`s between these files are relative (resolved against each file's
`$id`), so `ajv-cli` loads them with `-r "format/*.json"` and
`-r "format/values/*.json"` alongside the root `-s format.json`.

## Exact fetch commands

```sh
BASE="https://raw.githubusercontent.com/design-tokens/community-group/main/www/public/schemas/2025.10"
DEST="dtcg/2025.10"
mkdir -p "$DEST/format/values"

for f in format.json format/group.json format/groupOrToken.json \
         format/token.json format/tokenType.json; do
  curl -sSL "$BASE/$f" -o "$DEST/$f"
done

for v in border color cubicBezier dimension duration fontFamily fontWeight \
         gradient number shadow strokeStyle transition typography; do
  curl -sSL "$BASE/format/values/$v.json" -o "$DEST/format/values/$v.json"
done
```

Fetched from the `main` branch. The 2025.10 revision is the first stable
release of the DTCG format and is not expected to change under this path.

## Known limitation (documented, not a kapi bug)

JSON Schema cannot express the DTCG `$type` cascade (a group's `$type` inherited
by its leaf tokens). The official schema only validates a token's `$value`
against its type when the token itself declares `$type`. Cascade-only tokens
fall into a catch-all `oneOf` over all value types; unambiguous values (a hex
colour, a px dimension, …) still validate, but a bare ambiguous string such as
the fontWeight keyword `"thin"` does not. The acceptance test accounts for this
by asserting schema-validity EQUIVALENCE (kapi output validates iff the source
did) and exercising the positive path on a fixture the schema accepts.
