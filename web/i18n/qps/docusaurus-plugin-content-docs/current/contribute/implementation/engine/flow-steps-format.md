---
sidebar_position: 5
title: Flow Steps Format
description: Implementation note — the YAML steps-based flow format that compiles to the internal nodes-and-edges graph, including format detection rules, the FlowStep struct, and the StepsToGraph compilation algorithm.
keywords: [flow steps format, YAML, steps, nodes, edges, StepsToGraph, flow compilation, implementation note]
---

# ▒ Ƒļöŵ Šţéþš Ƒöŕḿàţ ▒

▒ Ţĥé šţéþš-ƃàšéđ ƒļöŵ ƒöŕḿàţ þŕöṽîđéš ĥüḿàñ-ƒŕîéñđļý ÝÀḾĻ àüţĥöŕîñĝ ţĥàţ çöḿþîļéš ţö ţĥé îñţéŕñàļ ñöđéš+éđĝéš ĝŕàþĥ ŕéþŕéšéñţàţîöñ. ▒

## ▒ Ƒöŕḿàţ Đéţéçţîöñ ▒

▒ Ţĥé ƒļöŵ þàŕšéŕ àüţö-đéţéçţš ţĥé ƒöŕḿàţ: ▒

- ▒ `šþéç.šţéþš` þŕéšéñţ -> šţéþš ƒöŕḿàţ, çöḿþîļéđ ṽîà `ŠţéþšŢöĜŕàþĥ()` ▒
- ▒ `šþéç.ñöđéš` þŕéšéñţ -> ĝŕàþĥ ƒöŕḿàţ, üšéđ đîŕéçţļý ▒
- ▒ Ƃöţĥ éñṽéļöþéđ (`àþîṼéŕšîöñ: ṽ1`) àñđ ƃàŕé ÝÀḾĻ àŕé šüþþöŕţéđ ▒

## ▒ Šţéþš Ƒöŕḿàţ Šþéç ▒

```go
type FlowStep struct {
    Tool     string         `yaml:"tool,omitempty"`     // tool name
    Config   map[string]any `yaml:"config,omitempty"`   // tool parameters
    Label    string         `yaml:"label,omitempty"`    // display label
    Parallel []FlowStep     `yaml:"parallel,omitempty"` // fan-out branches
}
```

## ▒ Ţŕàñšƒöŕḿéŕš àš öŕđéŕéđ šţéþš ▒

▒ Ţööļš ţĥàţ ŕéŵŕîţé ţĥé šöüŕçé/ḿöđéļ (ŕéđàçţîöñ, à šîḿþļîƒîéŕ, ñöŕḿàļîžàţîöñ)
àŕé öŕđîñàŕý éñţŕîéš îñ `šţéþš:`; ţĥéŕé îš ñö šéþàŕàţé šţŕüçţüŕàļ šţàĝé
([É-03](../../architecture/engine/e-03-tool-system.md)). À ƒļöŵ ţĥàţ đéçļàŕéš ţĥé
ŕéḿöṽéđ `šöüŕçé_ţŕàñšƒöŕḿš:` ƒîéļđ îš ŕéĵéçţéđ ƃý `ŠţéþšŢöĜŕàþĥ` ŵîţĥ à
ḿîĝŕàţîöñ éŕŕöŕ þöîñţîñĝ àţ É-03 àñđ đîŕéçţîñĝ ţĥé àüţĥöŕ ţö ļîšţ ţĥé
ţŕàñšƒöŕḿéŕš àš öŕđéŕéđ šţéþš. ▒

```yaml
steps:
  - tool: redact          # applied inline; later steps see the redacted source
  - tool: translate
  - tool: qa
```

▒ Ţŕàñšƒöŕḿéŕ öŕđéŕîñĝ îš ṽàļîđàţéđ ƃý ţĥé þļàçéḿéñţ þàšš
(`çöŕé/ƒļöŵ/þļàçéḿéñţ.ĝö`), ŵĥîçĥ ŕüñš ƃéšîđé đàţà-ƒļöŵ ṽàļîđàţîöñ àţ éṽéŕý
ƒļöŵ ƃüîļđ/ļöàđ ĝàţé àñđ éḿîţš ţĥéšé đîàĝñöšţîçš: ▒

| Rule id | Severity | Trigger |
| --- | --- | --- |
| `transformer-after-target` | error | a transformer follows a step that produces a committed target; exempt when the transformer produces the target port itself (e.g. `unredact`) |
| `transformer-after-remote-egress` | error | a recoverable transformer (`redact`) follows a step with the remote-source-egress side effect; exempt for the step(s) producing an input the transformer's config-resolved contract requires |
| `transformer-late-placement` | warning | a transformer sits later than its earliest valid slot (after its last required input), forcing avoidable overlay rebasing |

## ▒ Çöḿþîļàţîöñ ▒

▒ `ŠţéþšŢöĜŕàþĥ(šþéç)` ĝéñéŕàţéš: ▒

1. ▒ Ţööļ ñöđéš ƒŕöḿ `šţéþš`, çĥàîñéđ šéǫüéñţîàļļý ▒
2. ▒ Þàŕàļļéļ ƃŕàñçĥéš ƒöŕ `þàŕàļļéļ:` ƃļöçķš (ţéé ƒŕöḿ þŕéṽîöüš, ĵöîñ àţ ñéẋţ) ▒

▒ Àüţö-àššîĝñéđ ÎĐš ƒöļļöŵ `ţööļ-Ñ` þàţţéŕñ. Þöšîţîöñš àüţö-ļàýöüţ ļéƒţ-ţö-ŕîĝĥţ.
Ţĥé ĝŕàþĥ îš ţööļ ñöđéš öñļý; ţĥé ƒļöŵ'š šöüŕçé àñđ šîñķ àŕé ƃîñđîñĝš ŕéšöļṽéđ
àţ ŕüñ ţîḿé ([É-04](../../architecture/engine/e-04-flows-and-io-binding.md)), ñöţ ñöđéš. ▒

## ▒ Éẋàḿþļéš ▒

### ▒ Ļîñéàŕ þîþéļîñé ▒

```yaml
steps:
  - tool: recycle
    config: { fuzzyThreshold: 75 }
  - tool: translate
  - tool: qa
```

### ▒ Ƒàñ-öüţ ▒

```yaml
steps:
  - parallel:
      - tool: translate
      - tool: term-check
  - tool: qa
```

### ▒ Šçŕîþţ šţéþ ▒

```yaml
steps:
  - tool: script
    label: Filter long segments
    config:
      code: |
        if (part.type === "block") {
          if (part.block.source[0].content.text.length > 200) emit(part);
          else skip();
        } else emit(part);
```
