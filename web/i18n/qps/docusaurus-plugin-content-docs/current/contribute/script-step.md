---
sidebar_position: 5
title: JavaScript Script Step
description: The script step lets you run custom ES5 JavaScript on each Part flowing through a neokapi pipeline — using the goja pure-Go runtime, with access to emit(), skip(), and log() for controlling Part flow.
keywords: [script step, JavaScript, ES5, goja, pipeline, custom processing, Part, neokapi]
---

# ▒ ĴàṽàŠçŕîþţ Šçŕîþţ Šţéþ ▒

▒ Ţĥé šçŕîþţ šţéþ ļéţš ýöü ŕüñ çüšţöḿ ĴàṽàŠçŕîþţ (ÉŠ5) öñ éàçĥ Þàŕţ ƒļöŵîñĝ ţĥŕöüĝĥ ţĥé þîþéļîñé. Îţ üšéš ţĥé [ĝöĵà](https://github.com/dop251/goja) ÉŠ5 ŕüñţîḿé -- à þüŕé Ĝö ĴàṽàŠçŕîþţ éñĝîñé ŵîţĥ ñö ÇĜö đéþéñđéñçý. ▒

## ▒ Ĥöŵ îţ ŵöŕķš ▒

▒ Ţĥé šçŕîþţ ţööļ ŕéçéîṽéš éàçĥ Þàŕţ öñé àţ à ţîḿé. Ƒöŕ éàçĥ Þàŕţ, ýöüŕ šçŕîþţ ŕüñš ŵîţĥ àççéšš ţö à `þàŕţ` öƃĵéçţ àñđ ţĥŕéé çöñţŕöļ ƒüñçţîöñš: `éḿîţ()`, `šķîþ()`, àñđ `ļöĝ()`. Îƒ ýöüŕ šçŕîþţ çàļļš ñéîţĥéŕ `éḿîţ()` ñöŕ `šķîþ()`, ţĥé Þàŕţ þàššéš ţĥŕöüĝĥ üñçĥàñĝéđ. ▒

▒ Ýöü çàñ ŵŕîţé ýöüŕ ļöĝîç îñ éîţĥéŕ öƒ ţŵö éǫüîṽàļéñţ ƒöŕḿš: àš **ţöþ-ļéṽéļ çöđé** àĝàîñšţ ţĥé îḿþļîçîţ `þàŕţ` ĝļöƃàļ (üšéđ ţĥŕöüĝĥöüţ ţĥé éẋàḿþļéš ƃéļöŵ), öŕ àš à **`ƒüñçţîöñ þŕöçéšš(þàŕţ)`** ţĥàţ ţĥé ţööļ çàļļš öñçé þéŕ Þàŕţ (šéé [Ƒüñçţîöñ ƒöŕḿ](#function-form)). Ƃöţĥ ĥàṽé îđéñţîçàļ çàþàƃîļîţîéš; ţĥé ƒüñçţîöñ ƒöŕḿ àđđš à ŕéţüŕñ-ṽàļüé çöñṽéñîéñçé. ▒

## ▒ Ţĥé ĴàṽàŠçŕîþţ ÀÞÎ ▒

### ▒ Ţĥé þàŕţ öƃĵéçţ ▒

```javascript
part.type; // "block", "data", "media", "layer-start", "layer-end",
// "group-start", "group-end"
```

▒ Ƒöŕ ƃļöçķ þàŕţš, ţĥé `ƃļöçķ` þŕöþéŕţý þŕöṽîđéš àççéšš ţö ţŕàñšļàţàƃļé çöñţéñţ: ▒

```javascript
part.block.id; // Block ID string
part.block.translatable; // boolean

// Source runs (flat array)
part.block.source[0].content.text; // text of the first run

// Target runs by locale (object)
part.block.targets["fr"][0].content.text; // French target's first run text
```

### ▒ éḿîţ(þàŕţ) ▒

▒ Éḿîţ à ḿöđîƒîéđ (öŕ ñéŵ) Þàŕţ ţö ţĥé öüţþüţ çĥàññéļ. Îƒ ýöü çàļļ `éḿîţ()`, ţĥé öŕîĝîñàļ Þàŕţ îš **ñöţ** ƒöŕŵàŕđéđ àüţöḿàţîçàļļý -- öñļý ŵĥàţ ýöü éḿîţ ŕéàçĥéš đöŵñšţŕéàḿ ţööļš. ▒

```javascript
// Modify a target translation and emit
if (part.block.targets["fr"]) {
  var seg = part.block.targets["fr"][0];
  seg.content.text = seg.content.text.toUpperCase();
}
emit(part);
```

▒ Ƃý đéƒàüļţ ţĥé šçŕîþţ ḿàý ŕéàđ ţĥé šöüŕçé ƃüţ öñļý **ţàŕĝéţ** éđîţš àŕé ŕéàđ
ƃàçķ — ţĥé šöüŕçé îš ŕéàđ-öñļý (šéé [Çöñƒîĝüŕàţîöñ ŕéƒéŕéñçé](#configuration-reference)). ▒

### ▒ šķîþ() ▒

▒ Đŕöþ ţĥé çüŕŕéñţ Þàŕţ éñţîŕéļý. Îţ ŵîļļ ñöţ ŕéàçĥ đöŵñšţŕéàḿ ţööļš öŕ ţĥé ŵŕîţéŕ. ▒

```javascript
if (part.type === "block" && part.block.source[0].content.text === "") {
  skip();
}
```

### ▒ ļöĝ(ḿéššàĝé) ▒

▒ Ŵŕîţé à ḿéššàĝé ţö šţđéŕŕ ƒöŕ đéƃüĝĝîñĝ. ▒

```javascript
log("Processing block: " + part.block.id);
```

### ▒ Çöñţŕöļ ƒļöŵ šüḿḿàŕý ▒

| Script behavior                | Result                                        |
| ------------------------------ | --------------------------------------------- |
| No `emit()` or `skip()` called | Part passes through unchanged                 |
| `emit(part)` called            | Only emitted parts are forwarded              |
| `skip()` called                | Part is dropped                               |
| `emit()` called multiple times | All emitted parts are forwarded (one-to-many) |

### ▒ Ƒüñçţîöñ ƒöŕḿ ▒

▒ Îñšţéàđ öƒ ŵŕîţîñĝ ţöþ-ļéṽéļ çöđé àĝàîñšţ ţĥé îḿþļîçîţ `þàŕţ` ĝļöƃàļ, ýöü çàñ đéƒîñé à `þŕöçéšš` ƒüñçţîöñ. Ţĥé ţööļ çàļļš îţ öñçé þéŕ Þàŕţ, þàššîñĝ ţĥé þàŕţ àš îţš àŕĝüḿéñţ: ▒

```javascript
function process(part) {
  if (part.type === "block" && part.block.targets["fr"]) {
    var seg = part.block.targets["fr"][0];
    seg.content.text = seg.content.text.toUpperCase();
  }
  return part; // emit the (modified) part
}
```

▒ Îñšîđé `þŕöçéšš`, `éḿîţ()`, `šķîþ()`, àñđ `ļöĝ()` ƃéĥàṽé éẋàçţļý àš îñ ţĥé îḿþļîçîţ-ĝļöƃàļš ƒöŕḿ. Àš à çöñṽéñîéñçé, ţĥé ƒüñçţîöñ'š **ŕéţüŕñ ṽàļüé** îš àļšö ĥöñöŕéđ — ƃüţ öñļý ŵĥéñ ýöü ĥàṽé _ñöţ_ àļŕéàđý çàļļéđ `éḿîţ()` öŕ `šķîþ()` (àñ éẋþļîçîţ çàļļ àļŵàýš ŵîñš): ▒

| `process(part)` returns | Result                                          |
| ----------------------- | ----------------------------------------------- |
| a part object           | that part is emitted                            |
| an array of parts       | all are emitted (one-to-many)                   |
| `null`                  | the part is dropped (equivalent to `skip()`)    |
| nothing (`undefined`)   | the part passes through unchanged               |

▒ Ţĥé ƒüñçţîöñ ƒöŕḿ ŵöŕķš îđéñţîçàļļý ŵîţĥ `--çöđé`, `--šçŕîþţ-ƒîļé`, àñđ ţĥé ÝÀḾĻ `çöđé`/`šçŕîþţƑîļé` çöñƒîĝ. Îţ ŕéàđš ñàţüŕàļļý ƒöŕ ţŕàñšƒöŕḿš ţĥàţ çöḿþüţé àñđ ŕéţüŕñ à ŕéšüļţ; ţĥé îḿþļîçîţ-ĝļöƃàļš ƒöŕḿ îš ţéŕšéŕ ƒöŕ šîḿþļé ƒîļţéŕš. Šöüŕçé éđîţš šţîļļ ŕéǫüîŕé `àļļöŵŠöüŕçéḾüţàţîöñ` (šéé [Çöñƒîĝüŕàţîöñ ŕéƒéŕéñçé](#configuration-reference)) îñ éîţĥéŕ ƒöŕḿ. ▒

## ▒ ÇĻÎ üšàĝé ▒

### ▒ Îñļîñé çöđé ▒

```bash
kapi exec script -i input.xliff --code 'if (part.type === "block") {
  var text = part.block.source[0].content.text;
  if (text.length > 100) { skip(); }
}'
```

### ▒ Šçŕîþţ ƒîļé ▒

```bash
kapi exec script -i input.xliff --script-file filter.js
```

▒ Ŵĥéŕé `ƒîļţéŕ.ĵš` çöñţàîñš: ▒

```javascript
if (part.type === "block") {
  var text = part.block.source[0].content.text;
  if (text.length <= 5) {
    skip();
  }
}
```

## ▒ ÝÀḾĻ ƒļöŵ üšàĝé ▒

▒ Üšé ţĥé šçŕîþţ šţéþ îñļîñé îñ à ƒļöŵ đéƒîñîţîöñ: ▒

```yaml
steps:
  - tool: script
    label: Filter short segments
    config:
      code: |
        if (part.type === 'block') {
          var text = part.block.source[0].content.text;
          if (text.length < 3) {
            skip();
          }
        }

  - tool: pseudo-translate
    config:
      targetLocale: fr
```

▒ Öŕ ŕéƒéŕéñçé àñ éẋţéŕñàļ ƒîļé: ▒

```yaml
steps:
  - tool: script
    config:
      scriptFile: ./scripts/filter.js

  - tool: pseudo-translate
    config:
      targetLocale: fr
```

## ▒ Éẋàḿþļéš ▒

### ▒ Ƒîļţéŕ ƃý šöüŕçé ţéẋţ ļéñĝţĥ ▒

▒ Šķîþ ƃļöçķš ŵĥéŕé ţĥé šöüŕçé ţéẋţ îš šĥöŕţéŕ ţĥàñ à ţĥŕéšĥöļđ: ▒

```javascript
if (part.type === "block") {
  var text = part.block.source[0].content.text;
  if (text.length < 10) {
    skip();
  }
}
```

### ▒ Ḿöđîƒý ţàŕĝéţ ţéẋţ ▒

▒ Àþþéñđ à ḿàŕķéŕ ţö àļļ Ƒŕéñçĥ ţŕàñšļàţîöñš: ▒

```javascript
if (part.type === "block" && part.block.targets["fr"]) {
  var seg = part.block.targets["fr"][0];
  seg.content.text = seg.content.text + " [REVIEW]";
  emit(part);
}
```

### ▒ Çöñđîţîöñàļ ŕöüţîñĝ ▒

▒ Öñļý þàšš ţŕàñšļàţàƃļé ƃļöçķš ţĥŕöüĝĥ ţö đöŵñšţŕéàḿ ţööļš: ▒

```javascript
if (part.type !== "block") {
  // Let structural parts (layers, data) pass through
  emit(part);
} else if (part.block.translatable) {
  emit(part);
} else {
  skip();
}
```

### ▒ Ţŕàñšƒöŕḿ šöüŕçé ţéẋţ ▒

▒ Ñöŕḿàļîžé ŵĥîţéšþàçé îñ ţĥé šöüŕçé ƃéƒöŕé ţŕàñšļàţîöñ. Šöüŕçé éđîţš àŕé
**îĝñöŕéđ ƃý đéƒàüļţ** — ţĥé šöüŕçé îš ŕéàđ-öñļý ţö ţĥé šçŕîþţ (îḿḿüţàƃîļîţý
çöñţŕàçţ). Öþţ îñ ŵîţĥ `àļļöŵŠöüŕçéḾüţàţîöñ: ţŕüé`, àñđ þļàçé ţĥé šţéþ àĥéàđ
öƒ ţĥé šţéþš ţĥàţ šĥöüļđ öƃšéŕṽé ţĥé ŕéŵŕîţţéñ šöüŕçé — ţýþîçàļļý ƒîŕšţ (šéé
[ƒļöŵ àüţĥöŕîñĝ](/contribute/flow-authoring)): ▒

```yaml
steps:
  - tool: script
    config:
      allowSourceMutation: true
      code: |
        if (part.type === 'block') {
          var text = part.block.source[0].content.text;
          text = text.replace(/\s+/g, ' ').replace(/^\s+|\s+$/g, '');
          part.block.source[0].content.text = text;
          emit(part);
        }
  - tool: translate
```

### ▒ Ļöĝ àñđ þàšš ţĥŕöüĝĥ ▒

▒ Îñšþéçţ ţĥé þîþéļîñé ŵîţĥöüţ çĥàñĝîñĝ àñýţĥîñĝ: ▒

```javascript
if (part.type === "block") {
  log("Block " + part.block.id + ": " + part.block.source[0].content.text);
}
// No emit() or skip() -- part passes through unchanged
```

## ▒ Çöñƒîĝüŕàţîöñ ŕéƒéŕéñçé ▒

| Property              | Type    | Description                                                                       |
| --------------------- | ------- | --------------------------------------------------------------------------------- |
| `source`              | string  | Mode selector: `inline` (default) or `file`                                       |
| `code`                | string  | Inline JavaScript code (ES5)                                                      |
| `scriptFile`          | string  | Path to a `.js` file                                                              |
| `allowSourceMutation` | boolean | Permit the script to modify the source text. Off by default — the source is read-only and source edits are ignored unless this is set. |

▒ Þŕöṽîđé éîţĥéŕ `çöđé` öŕ `šçŕîþţƑîļé`. Ţĥé öþţîöñàļ `šöüŕçé` ƒîéļđ šéļéçţš ţĥé
ḿöđé éẋþļîçîţļý (`îñļîñé` öŕ `ƒîļé`) ƒöŕ ÜÎ àñđ ṽàļîđàţîöñ; ŵĥéñ öḿîţţéđ, ţĥé
ḿöđé îš îñƒéŕŕéđ ƒŕöḿ ŵĥîçĥéṽéŕ öƒ `çöđé`/`šçŕîþţƑîļé` îš šéţ. ▒

## ▒ Ñöţéš ▒

- ▒ Ţĥé ŕüñţîḿé îš ÉŠ5 öñļý (ñö `ļéţ`, `çöñšţ`, àŕŕöŵ ƒüñçţîöñš, öŕ ţéḿþļàţé ļîţéŕàļš). Üšé `ṽàŕ` ƒöŕ ṽàŕîàƃļé đéçļàŕàţîöñš. ▒
- ▒ Éàçĥ ţööļ îñšţàñçé ĝéţš îţš öŵñ ĝöĵà ŕüñţîḿé, šö ţĥéŕé îš ñö šĥàŕéđ šţàţé ƃéţŵééñ þàŕàļļéļ þîþéļîñé ƃŕàñçĥéš. ▒
- ▒ Ţĥé šçŕîþţ ŕüñš šýñçĥŕöñöüšļý ƒöŕ éàçĥ Þàŕţ. Ļöñĝ-ŕüññîñĝ šçŕîþţš ŵîļļ ƃļöçķ ţĥé þîþéļîñé. ▒
- ▒ Ţàŕĝéţ ţéẋţ éđîţš öñ ƃļöçķ þàŕţš àŕé ŕéàđ ƃàçķ. Šöüŕçé éđîţš àŕé ŕéàđ ƃàçķ öñļý ŵĥéñ `àļļöŵŠöüŕçéḾüţàţîöñ: ţŕüé`; öţĥéŕŵîšé ţĥé šöüŕçé îš ŕéàđ-öñļý. Çĥàñĝéš ţö öţĥéŕ Þàŕţ ţýþéš àŕé ñöţ þéŕšîšţéđ. ▒
