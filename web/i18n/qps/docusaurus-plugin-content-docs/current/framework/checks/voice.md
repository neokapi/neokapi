---
sidebar_position: 3
title: Voice profiles
description: The voice profile is one checkset over neokapi's content-verification engine — a machine-readable profile of tone, style, and vocabulary whose findings annotate Blocks like every other check.
keywords: [voice profile, voice profile, content checks, writing style, terminology, MCP, AI assistant]
---

import { PipelineDiagram } from "@neokapi/docs-shared";

# ▒ Ṽöîçé þŕöƒîļéš ▒

▒ Ŵĥéŕé [ţéŕḿîñöļöĝý](/framework/terminology) éñšüŕéš ýöü üšé ţĥé ŕîĝĥţ ŵöŕđš,
à ṽöîçé þŕöƒîļé đéšçŕîƃéš ĥöŵ ýöü šàý ţĥéḿ — ţĥé þéŕšöñàļîţý, ƒöŕḿàļîţý, àñđ
ŵŕîţîñĝ þàţţéŕñš ţĥàţ ḿàķé çöñţéñţ ŕéçöĝñîžàƃļé. ñéöķàþî çàþţüŕéš à ṽöîçé
àš à ḿàçĥîñé-ŕéàđàƃļé þŕöƒîļé àñđ ŕüñš îţ àš **öñé çĥéçķšéţ öṽéŕ ţĥé šàḿé
[çöñţéñţ-ṽéŕîƒîçàţîöñ éñĝîñé](/framework/checks)** ţĥàţ þöŵéŕš ţéŕḿîñöļöĝý,
đö-ñöţ-ţŕàñšļàţé, àñđ þļàçéĥöļđéŕ îñţéĝŕîţý: éṽéŕý çĥéçķéŕ éḿîţš ţĥé šàḿé
ƒîñđîñĝš îñţö ţĥé šàḿé `Ƃļöçķ` àññöţàţîöñ, šö ṽöîçé îš öñé çĥéçķ àḿöñĝ
ḿàñý, ñöţ à šéþàŕàţé šýšţéḿ. Ţĥé Ĝö ļîƃŕàŕý ļîṽéš îñ `çöŕé/þŕöƒîļé/`. ▒

▒ Üšéđ ţĥîš ŵàý, à ṽöîçé þŕöƒîļé ķééþš àñ ÀÎ àššîšţàñţ öñ-ṽöîçé ţĥé ŵàý à ţéšţ ķééþš
çöđé çöŕŕéçţ: ļöàđ ţĥé þŕöƒîļé îñţö çöñţéẋţ (öŕ éẋþöšé îţ öṽéŕ
[ḾÇÞ](/reference/mcp)) šö ĝéñéŕàţéđ çöþý îš öñ-ṽöîçé ƒŕöḿ ţĥé ƒîŕšţ đŕàƒţ, ţĥéñ
**çĥéçķ** àñýţĥîñĝ ţĥàţ đŕîƒţš àñđ çàŕŕý ţĥé šàḿé ṽöîçé ţĥŕöüĝĥ éṽéŕý
ţŕàñšļàţîöñ. Ţĥé ƒîñđîñĝš — ţĥé šþéçîƒîç ţéŕḿš àñđ ŕüļéš ţĥàţ ƃŕöķé — àŕé ţĥé
šüƃšţàñçé; ţĥé 0–100 ŕöļļ-üþ îš à çöñṽéñîéñçé, ĥöñéšţ öñļý ŵĥéñ çàļîƃŕàţéđ
àĝàîñšţ à ļàƃéļéđ šéţ. ▒

## ▒ Ṽöîçé þŕöƒîļéš ŵîţĥ ţĥé ÇĻÎ ▒

▒ Ţĥé `ķàþî ṽöîçé` çöḿḿàñđ ĝŕöüþ ŵöŕķš àĝàîñšţ à þŕöƒîļé ƒŕöḿ à ƃüîļţ-îñ šţàŕţéŕ
þàçķ (`--þàçķ`), ţĥé ļöçàļ ṽöîçé šţöŕé (`--þŕöƒîļé`), öŕ à šţàñđàļöñé
ĝîţ-šĥàŕéàƃļé ÝÀḾĻ ƒîļé (`--þŕöƒîļé-ƒîļé`): ▒

```bash
# Print the rendered guide (paste into an assistant, or pipe to a file)
kapi voice guide --pack friendly-dtc

# Score text: file argument, --input-text, or stdin. --min-score gates CI (exit 3).
kapi voice check --profile-file voice.yaml --min-score 80 release-notes.md

# Rewrite off-voice content (add --ai for tone/style as well as vocabulary)
kapi voice rewrite --profile-file voice.yaml --input-text "Leverage our solution"

# Manage profiles in the local store
kapi voice profiles
```

▒ Ƃöţĥ `çĥéçķ` àñđ `ŕéŵŕîţé` ŕüñ à ƒàšţ, öƒƒļîñé ŕüļé-ƃàšéđ ṽöçàƃüļàŕý þàšš ƃý
đéƒàüļţ; þàšš `--àî` ţö àđđ àñ ĻĻḾ àñàļýšîš öƒ ţöñé, šţýļé, àñđ çļàŕîţý. ▒

## ▒ Ṽöîçé þŕöƒîļéš ▒

▒ À þŕöƒîļé çàþţüŕéš ţöñé, šţýļé, àñđ ṽöçàƃüļàŕý àš ŕüļéš: ▒

```yaml
name: "Acme Corp"
description: "Professional yet approachable B2B SaaS voice"

tone:
  personality: [knowledgeable, helpful, confident]
  formality: neutral
  emotion: warm
  humor: light

style:
  active_voice: true
  sentence_length: medium
  person_pov: second # "you" / "your"
  contractions: sometimes

vocabulary:
  preferred_terms:
    - term: "workspace"
      note: "Use instead of 'account' or 'organization'"
  forbidden_terms:
    - term: "leverage"
      replacement: "use"
      severity: minor
  competitor_terms:
    - term: "Slack"
      replacement: "messaging platform"
      severity: critical

examples:
  - before: "Users can leverage the platform to achieve synergy."
    after: "Your team can use the workspace to collaborate more effectively."
    explanation: "Active voice, preferred terms, removed jargon"
    category: style
```

▒ Þŕöƒîļéš šüþþöŕţ **ļöçàļé öṽéŕŕîđéš** (é.ĝ. `ƒöŕḿàļ` àñđ ţĥîŕđ-þéŕšöñ ÞÖṼ ƒöŕ
`ĵà`) àñđ **çĥàññéļ öṽéŕŕîđéš** (é.ĝ. çàšüàļ, ƒŕéǫüéñţ ĥüḿöŕ ƒöŕ
`šöçîàļ_ḿéđîà`). Çĥàññéļ öṽéŕŕîđéš ŕéþļàçé ŵĥöļé Ţöñé/Šţýļé šéçţîöñš; ļöçàļé
öṽéŕŕîđéš ḿéŕĝé îñđîṽîđüàļ ƒîéļđš. ▒

## ▒ Çöḿþļîàñçé šçöŕîñĝ ▒

▒ Çöḿþļîàñçé îš šçöŕéđ 0–100 àçŕöšš ƒîṽé đîḿéñšîöñš — Ţöñé, Šţýļé, Ṽöçàƃüļàŕý,
Çļàŕîţý, àñđ öṽéŕàļļ ṽöîçé çöḿþļîàñçé. Éàçĥ ƒîñđîñĝ ŕéđüçéš ţĥé šçöŕé ƃý îţš
šéṽéŕîţý ŵéîĝĥţ: ▒

| Severity   | Weight | Example                   |
| ---------- | ------ | ------------------------- |
| `Neutral`  | 0      | Informational note        |
| `Minor`    | 1      | Slight tone inconsistency |
| `Major`    | 5      | Wrong term used           |
| `Critical` | 25     | Competitor term used      |

## ▒ Šţàŕţéŕ þàçķš ▒

▒ Ƃüîļţ-îñ þàçķš þŕöṽîđé ŕéàđý-ţö-üšé šţàŕţîñĝ þöîñţš — `þŕöƒéššîöñàļ-ƃ2ƃ`,
`ƒŕîéñđļý-đţç`, `ţéçĥñîçàļ-đöçš`, `ḿàŕķéţîñĝ-ƃļöĝ`, àñđ `çüšţöḿéŕ-šüþþöŕţ` —
éàçĥ ŵîţĥ ţöñé šéţţîñĝš, šţýļé ŕüļéš, ṽöçàƃüļàŕý çöñšţŕàîñţš, àñđ ƃéƒöŕé/àƒţéŕ
éẋàḿþļéš ţö çüšţöḿîžé. ▒

## ▒ Þîþéļîñé îñţéĝŕàţîöñ ▒

▒ Ţĥé `ṽöîçé-çĥéçķ` ţööļ ŕüñš îñ ţĥé þîþéļîñé àļöñĝšîđé öţĥéŕ ţööļš: ▒

<PipelineDiagram
  stages={[
    { label: "recycle", role: "translate" },
    { label: "term-lookup", role: "annotate" },
    { label: "translate", sub: "LLM", role: "translate" },
    { label: "voice-check", sub: "LLM", role: "qa" },
    { label: "qa", sub: "LLM", role: "qa" },
  ]}
/>

▒ Îţ üšéš àñ ĻĻḾ ţö àñàļýžé çöñţéñţ àĝàîñšţ ţĥé þŕöƒîļé àñđ àţţàçĥéš çöḿþļîàñçé
šçöŕéš àñđ ƒîñđîñĝš ţö éàçĥ Ƃļöçķ àš àññöţàţîöñš. Ţĥé ƒàšţéŕ, ŕüļé-ƃàšéđ
`ṽöîçé-ṽöçàƃ-çĥéçķ` ţööļ çĥéçķš ƒöŕƃîđđéñ àñđ çöḿþéţîţöŕ ţéŕḿš ŵîţĥöüţ ĻĻḾ
çàļļš. Ṽöîçé ṽöçàƃüļàŕý àļšö ƒļöŵš ţĥŕöüĝĥ öŕđîñàŕý ţéŕḿîñöļöĝý ţööļš —
þŕéƒéŕŕéđ ţéŕḿš šüŕƒàçé îñ `ţéŕḿ-ļööķüþ`, ƒöŕƃîđđéñ/çöḿþéţîţöŕ ţéŕḿš ţŕîĝĝéŕ
`ţéŕḿ-éñƒöŕçé` ṽîöļàţîöñš — šö ṽöîçé ĝüàŕđŕàîļš àñđ ţéŕḿîñöļöĝý šĥàŕé öñé
éñƒöŕçéḿéñţ þàţĥ. ▒

## ▒ ḾÇÞ îñţéĝŕàţîöñ ▒

▒ ÀÎ àĝéñţš ŕéàçĥ ṽöîçé çĥéçķîñĝ ţĥŕöüĝĥ ţĥé `ķàþî ḿçþ` šéŕṽéŕ: ▒

```json
{
  "mcpServers": {
    "kapi": {
      "command": "kapi",
      "args": ["mcp"]
    }
  }
}
```

▒ Àĝéñţš çàñ šçöŕé çöñţéñţ ƒöŕ ṽöîçé çöḿþļîàñçé ŵîţĥ ţĥé `ṽöîçé_çĥéçķ` ḾÇÞ ţööļ,
ƒéţçĥ ţĥé ĝüîđé ŵîţĥ `ṽöîçé_ĝüîđé`, àñđ ŕéŵŕîţé öƒƒ-ṽöîçé çöþý ŵîţĥ
`ṽöîçé_ŕéŵŕîţé`. Šéŕṽéŕ đéþļöýḿéñţš
çàñ éẋþöšé àñ ĤŢŢÞ ḾÇÞ éñđþöîñţ šö àĝéñţš çöñšüḿé þŕöƒîļéš àñđ šçöŕîñĝ ŵîţĥöüţ à
ļöçàļ ÇĻÎ þŕöçéšš. ▒

## ▒ Ĝö ļîƃŕàŕý ▒

### ▒ Šţöŕé ▒

```go
type Store interface {
    CreateProfile(ctx context.Context, profile *VoiceProfile) error
    GetProfile(ctx context.Context, id string) (*VoiceProfile, error)
    UpdateProfile(ctx context.Context, profile *VoiceProfile) error
    DeleteProfile(ctx context.Context, id string) error
    ListProfiles(ctx context.Context, workspaceID string) ([]*VoiceProfile, error)

    StoreScore(ctx context.Context, score *StoredScore) error
    GetScores(ctx context.Context, projectID string, locale model.LocaleID) ([]*StoredScore, error)
    GetScoreTrends(ctx context.Context, projectID string, days int) ([]*ScoreTrend, error)

    StoreCorrection(ctx context.Context, correction *Correction) error
    GetSuggestedRules(ctx context.Context, workspaceID string, minCount int) ([]*SuggestedRule, error)

    Close() error
}
```

▒ `ŠţöŕéđŠçöŕé`, `ŠçöŕéŢŕéñđ`, àñđ ţĥé öţĥéŕ üñǫüàļîƒîéđ ţýþéš àŕé đéçļàŕéđ îñ ţĥé
`þŕöƒîļé` þàçķàĝé; `ḿöđéļ.ĻöçàļéÎĐ` îš ţĥé ƂÇÞ-47 ļöçàļé ţýþé ƒŕöḿ
`ĝîţĥüƃ.çöḿ/ñéöķàþî/ñéöķàþî/çöŕé/ḿöđéļ`. ▒

▒ Ţĥé ƒŕàḿéŵöŕķ šĥîþš à ŠǪĻîţé ƃàçķéñđ (`ṽöîçé/šǫļîţé.ĝö`) ƃüîļţ öñ
ţĥé šĥàŕéđ `çöŕé/šţöŕàĝé` ḿîĝŕàţîöñ šýšţéḿ, ŵîţĥ ĴŠÖÑ çöļüḿñš ƒöŕ ţĥé çöḿþļéẋ
ţöñé/šţýļé/ṽöçàƃüļàŕý ƒîéļđš. Ţĥé îñţéŕƒàçé îš đéšîĝñéđ ƒöŕ éẋţéñšîöñ — šéŕṽéŕ
đéþļöýḿéñţš çàñ àđđ à ŵöŕķšþàçé-šçöþéđ ÞöšţĝŕéŠǪĻ ƃàçķéñđ. ▒

### ▒ Šçöŕîñĝ àñđ ŕéšöļüţîöñ ▒

```go
import "github.com/neokapi/neokapi/core/profile"

findings := []profile.VoiceFinding{
    {Dimension: profile.DimensionVocabulary, Severity: profile.SeverityMajor,
        Message: "Forbidden term: leverage", Suggestion: "use"},
    {Dimension: profile.DimensionTone, Severity: profile.SeverityMinor,
        Message: "Tone is too formal for this profile"},
}
score := profile.CalculateScore(findings) // score.Overall = 94 (100 - 5 - 1)

// ResolveProfile applies locale then channel overrides to a base profile
resolved := profile.ResolveProfile(base, "ja", "")
```

### ▒ Þîþéļîñé ţööļš ▒

```go
import (
    aitool "github.com/neokapi/neokapi/core/ai/tools"
    "github.com/neokapi/neokapi/core/profile"
    "github.com/neokapi/neokapi/core/tools"
)

// LLM-based: structured findings scored via CalculateScore, attached as a
// VoiceAnnotation plus voice-score / voice-findings properties
checkTool := aitool.NewVoiceCheckTool(llmProvider, profile)

// Rule-based: fast forbidden/competitor-term enforcement, no LLM calls
vocabTool := tools.NewVoiceVocabCheckTool(profile, terminology)
```

### ▒ Šţàŕţéŕ þàçķš ▒

```go
import "github.com/neokapi/neokapi/core/profile/packs"

names, _ := packs.List()          // the five built-in pack names
profile, _ := packs.Load("professional-b2b")
all, _ := packs.LoadAll()
```

▒ Þàçķš àŕé ÝÀḾĻ ƒîļéš éḿƃéđđéđ ṽîà `ĝö:éḿƃéđ`; éàçĥ ŕéţüŕñš à
`*þŕöƒîļé.ṼöîçéÞŕöƒîļé` ŕéàđý ţö üšé öŕ çüšţöḿîžé. ▒

### ▒ Çöñţéñţ ḿöđéļ îñţéĝŕàţîöñ ▒

▒ `ṼöîçéÀññöţàţîöñ` îš à ŕéĝîšţéŕéđ þàýļöàđ (`ṽöîçé`) šţöŕéđ àš à
ƃļöçķ-šçöþéđ **àññöţàţîöñ** ([Ƒ-02](/contribute/architecture/foundations/f-02-content-model)),
ţĥé çöüñţéŕþàŕţ ţö þöšîţîöñàļ öṽéŕļàýš ļîķé `ţéŕḿ` àñđ `éñţîţý`. Îţ îš ŕéàçĥéđ
ţĥŕöüĝĥ ţĥé ƃļöçķ'š `Àññö`/`ŠéţÀññö` ĥéļþéŕš àñđ ŕéĝîšţéŕéđ ƒöŕ ŵîŕé/šţöŕé
ŕéĥýđŕàţîöñ ṽîà `ḿöđéļ.ŔéĝîšţéŕÞàýļöàđ`: ▒

```go
type VoiceAnnotation struct {
    ProfileID string              `json:"profile_id"`
    Score     int                 `json:"score"` // 0-100 overall
    Findings  []VoiceFinding `json:"findings"`
    Position  model.RunRange      `json:"position"`
}

func (a *VoiceAnnotation) AnnotationType() string { return "voice" }
```

▒ Þŕöƒîļéš šéŕîàļîžé àš ƃöţĥ ĴŠÖÑ àñđ ÝÀḾĻ, šö ţĥéý çàñ ƃé àüţĥöŕéđ ƃý ĥàñđ öŕ
çöñšţŕüçţéđ þŕöĝŕàḿḿàţîçàļļý àš à `*þŕöƒîļé.ṼöîçéÞŕöƒîļé`. ▒
