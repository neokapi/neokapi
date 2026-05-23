# RESX real-world corpus — provenance

Every file in this directory is a GENUINE, unmodified `.resx` fetched from a
permissively-licensed public repository. The exact commit is pinned so the
fetch is reproducible. These files exercise the byte-faithful round-trip of the
native `resx` reader/writer against real .NET resource files (embedded XSD
schema, `<resheader>` boilerplate, `<comment>` notes, `{0}` composite-format
placeholders, typed/binary `<data>`, and the compact `<resheader>text` form).

All sources are MIT-licensed. The files are used here verbatim, solely as test
fixtures, with attribution below.

| File | Upstream repo | License | Commit |
|---|---|---|---|
| `roslyn-RebuildResources.resx` | [dotnet/roslyn](https://github.com/dotnet/roslyn) | MIT | `cdbc5c9269f117bd678202b81cbaa09f837b1123` |
| `roslyn-ErrorString.resx` | [dotnet/roslyn](https://github.com/dotnet/roslyn) | MIT | `cdbc5c9269f117bd678202b81cbaa09f837b1123` |
| `roslyn-XmlLiterals.resx` | [dotnet/roslyn](https://github.com/dotnet/roslyn) | MIT | `cdbc5c9269f117bd678202b81cbaa09f837b1123` |
| `stylecop-DocumentationResources.de-DE.resx` | [DotNetAnalyzers/StyleCopAnalyzers](https://github.com/DotNetAnalyzers/StyleCopAnalyzers) | MIT | `513ba0fefdf66e86c030c00a73812322585358ae` |

## Feature coverage

- `roslyn-RebuildResources.resx` — UTF-8 BOM, embedded `xsd:schema`, full
  `<resheader>` 2.0 boilerplate, `<comment>` notes, `{0}` placeholders.
- `roslyn-ErrorString.resx` — dense `{0}`/`{1}` composite-format placeholders
  and many `<comment>` developer notes.
- `roslyn-XmlLiterals.resx` — typed/binary `<data>` carrying `type=`
  (`System.Drawing.Color`, `System.Drawing.Icon`) and
  `mimetype="application/x-microsoft.net.object.binary.base64"`, including a
  `<comment>` inside a typed entry. The byte-faithful non-translatable
  passthrough case.
- `stylecop-DocumentationResources.de-DE.resx` — the COMPACT `<resheader>text`
  and `<data ...>text` form (resheader/value text not wrapped in `<value>`),
  plus typed (`System.Drawing.Color`) and binary base64 `<data>`.

## Exact fetch commands

```bash
SHA="cdbc5c9269f117bd678202b81cbaa09f837b1123"
SHA2="513ba0fefdf66e86c030c00a73812322585358ae"

curl -sSL "https://raw.githubusercontent.com/dotnet/roslyn/${SHA}/src/Compilers/Core/Rebuild/RebuildResources.resx" \
  -o roslyn-RebuildResources.resx
curl -sSL "https://raw.githubusercontent.com/dotnet/roslyn/${SHA}/src/Compilers/Core/MSBuildTask/ErrorString.resx" \
  -o roslyn-ErrorString.resx
curl -sSL "https://raw.githubusercontent.com/dotnet/roslyn/${SHA}/src/EditorFeatures/VisualBasicTest/Formatting/XmlLiterals.resx" \
  -o roslyn-XmlLiterals.resx
curl -sSL "https://raw.githubusercontent.com/DotNetAnalyzers/StyleCopAnalyzers/${SHA2}/StyleCop.Analyzers/StyleCop.Analyzers/DocumentationRules/DocumentationResources.de-DE.resx" \
  -o stylecop-DocumentationResources.de-DE.resx
```
