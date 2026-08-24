---
sidebar_position: 1
slug: /framework/go-quickstart
title: Use neokapi from Go
description: A minimal end-to-end Go program that uses the neokapi framework as a library — register the built-in formats, read a file into the streaming content model, run a built-in tool, walk the Blocks, and write bilingual XLIFF.
keywords: [neokapi, go library, quickstart, framework, content model, format reader, format writer, pipeline, golang]
---

# ▒ Üšé ñéöķàþî ƒŕöḿ Ĝö ▒

▒ ñéöķàþî îš à Ĝö ƒŕàḿéŵöŕķ ƒîŕšţ: à ƒöŕḿàţ-àŵàŕé çöñţéñţ éñĝîñé ýöü çàñ îḿþöŕţ àñđ
đŕîṽé đîŕéçţļý. Îţ þàŕšéš àñý ƒöŕḿàţ îñţö öñé çöñţéñţ ḿöđéļ, éđîţš öŕ ţŕàñšļàţéš
ţĥé çöñţéñţ îñšîđé îţ, àñđ ŵŕîţéš îţ ƃàçķ ƃýţé-ƒöŕ-ƃýţé. Ţĥé [`ķàþî` ÇĻÎ àñđ
đéšķţöþ àþþ](/kapi/overview) àñđ [ñéöķàþî-î18ñ](/react/introduction) àŕé šüŕƒàçéš
ƃüîļţ öñ ţöþ öƒ îţ, ƃüţ ţĥé šàḿé çöñţéñţ ḿöđéļ, ƒöŕḿàţ ŕéàđéŕš àñđ ŵŕîţéŕš, ţööļš,
àñđ šţŕéàḿîñĝ þîþéļîñé àŕé à Ĝö ļîƃŕàŕý ýöü çàñ îḿþöŕţ đîŕéçţļý. Ţĥîš þàĝé ŵàļķš
ţĥé šĥöŕţéšţ þàţĥ ƒŕöḿ `ĝö ĝéţ` ţö à ŵöŕķîñĝ þŕöĝŕàḿ — ţàķîñĝ ţĥé ŕöüñđ-ţŕîþ
ŕöüţé: ŕéàđ à ƒîļé, ƒîļļ îñ à ţàŕĝéţ, àñđ ŵŕîţé îţ ƃàçķ àš ƃîļîñĝüàļ ẊĻÎƑƑ. ▒

▒ Îƒ ýöü ŵàñţ ţĥé çöñçéþţš ƃéĥîñđ ţĥé çöđé ƒîŕšţ, ŕéàđ
[Àŕçĥîţéçţüŕé](/framework/architecture), ţĥé
[Çöñţéñţ Ḿöđéļ](/framework/content-model), àñđ [Ţööļš](/framework/tools). Ţĥîš
þàĝé àššüḿéš öñļý ţĥàţ ýöü ĥàṽé ţĥöšé öþéñ îñ àñöţĥéŕ ţàƃ. ▒

## ▒ Îñšţàļļ ▒

▒ Ţĥé ƒŕàḿéŵöŕķ ḿöđüļé îš `ĝîţĥüƃ.çöḿ/ñéöķàþî/ñéöķàþî`. Àđđ îţ ţö ýöüŕ ḿöđüļé: ▒

```bash
go get github.com/neokapi/neokapi
```

## ▒ À çöḿþļéţé þŕöĝŕàḿ ▒

▒ Ţĥé þŕöĝŕàḿ ƃéļöŵ ŕéàđš à šḿàļļ ĴŠÖÑ ḿéššàĝé çàţàļöĝ, ŕüñš ţĥé ƃüîļţ-îñ
`þšéüđö-ţŕàñšļàţé` [ţööļ](/framework/tools) ţö ƒîļļ îñ à ţàŕĝéţ, ŵàļķš ţĥé
ŕéšüļţîñĝ [Ƃļöçķš](/framework/content-model), àñđ ŵŕîţéš ţĥé šţŕéàḿ ƃàçķ öüţ àš
ƃîļîñĝüàļ ẊĻÎƑƑ 2.ẋ. Éṽéŕý šýḿƃöļ îš þàŕţ öƒ ţĥé þüƃļîç ƒŕàḿéŵöŕķ šüŕƒàçé. ▒

```go
package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"

	"golang.org/x/sync/errgroup"

	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tools"
)

const sourceJSON = `{
  "greeting": "Hello, world",
  "farewell": "Goodbye"
}`

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx := context.Background()

	const (
		sourceLocale = model.LocaleID("en")
		targetLocale = model.LocaleID("fr")
		outputPath   = "messages.xlf"
	)

	// 1. Build a format registry and register every built-in reader/writer.
	//    The registry maps a format id (e.g. "json", "xliff2") to a factory.
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	// 2. Create a reader for the source format and a writer for the output
	//    format. Here we read JSON and write bilingual XLIFF 2.x.
	reader, err := reg.NewReader("json")
	if err != nil {
		return fmt.Errorf("new json reader: %w", err)
	}
	defer reader.Close()

	writer, err := reg.NewWriter("xliff2")
	if err != nil {
		return fmt.Errorf("new xliff2 writer: %w", err)
	}
	defer writer.Close()

	// 3. Open the source document. A RawDocument carries the bytes, the
	//    source/target locales, and an io.ReadCloser the reader streams from.
	doc := &model.RawDocument{
		URI:          "messages.json",
		SourceLocale: sourceLocale,
		TargetLocale: targetLocale,
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(bytes.NewReader([]byte(sourceJSON))),
	}
	if err := reader.Open(ctx, doc); err != nil {
		return fmt.Errorf("open document: %w", err)
	}

	// 4. Pick a built-in tool. pseudo-translate writes a target for each
	//    Block by transforming the source text.
	pseudo := tools.NewPseudoTranslateTool(&tools.PseudoConfig{
		TargetLocale: targetLocale,
		Prefix:       "[",
		Suffix:       "]",
	})

	// 5. Configure the writer's output and target locale.
	if err := writer.SetOutput(outputPath); err != nil {
		return fmt.Errorf("set output: %w", err)
	}
	writer.SetLocale(targetLocale)

	// 6. Wire a streaming pipeline: reader -> tool -> inspect -> writer.
	//    Each stage runs in its own goroutine, connected by buffered channels
	//    of *model.Part, exactly as the executor does internally.
	toolIn := make(chan *model.Part, 64)   // reader -> tool
	writerIn := make(chan *model.Part, 64) // tool   -> inspect
	inspected := make(chan *model.Part, 64) // inspect -> writer

	g, gctx := errgroup.WithContext(ctx)

	// Reader stage: stream Parts out of the format reader. Each PartResult
	// pairs a *Part with an optional error.
	g.Go(func() error {
		defer close(toolIn)
		for result := range reader.Read(gctx) {
			if result.Error != nil {
				return fmt.Errorf("read: %w", result.Error)
			}
			select {
			case toolIn <- result.Part:
			case <-gctx.Done():
				return gctx.Err()
			}
		}
		return nil
	})

	// Tool stage: a tool's Process consumes Parts from its input channel,
	// transforms the ones it handles (here: Blocks), and relays the rest.
	g.Go(func() error {
		defer close(writerIn)
		return pseudo.Process(gctx, toolIn, writerIn)
	})

	// Inspection stage: walk the content model (Blocks, their source text,
	// and the target the tool just wrote) before handing Parts to the writer.
	g.Go(func() error {
		defer close(inspected)
		for part := range writerIn {
			if part.Type == model.PartBlock {
				if block, ok := part.Resource.(*model.Block); ok {
					fmt.Printf("block %-10s source=%q target=%q\n",
						block.ID, block.SourceText(), block.TargetText(targetLocale))
				}
			}
			select {
			case inspected <- part:
			case <-gctx.Done():
				return gctx.Err()
			}
		}
		return nil
	})

	// Writer stage: reconstruct the document from the Part stream.
	g.Go(func() error {
		return writer.Write(gctx, inspected)
	})

	if err := g.Wait(); err != nil {
		return fmt.Errorf("pipeline: %w", err)
	}

	fmt.Fprintf(os.Stdout, "wrote %s\n", outputPath)
	return nil
}
```

▒ Ŕüññîñĝ îţ þŕîñţš ŵĥàţ éàçĥ Ƃļöçķ ļööķš ļîķé àƒţéŕ ţĥé ţööļ àñđ ŵŕîţéš
`ḿéššàĝéš.ẋļƒ`: ▒

```text
block tu1        source="Hello, world" target="[Ĥéļļö, ŵöŕļđ]"
block tu2        source="Goodbye" target="[Ĝööđƃýé]"
wrote messages.xlf
```

```xml title="messages.xlf"
<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.2" version="2.2" srcLang="en" trgLang="fr">
  <file id="messages.json">
    <unit id="tu1" name="greeting">
      <segment>
        <source>Hello, world</source>
        <target>[Ĥéļļö, ŵöŕļđ]</target>
      </segment>
    </unit>
    <unit id="tu2" name="farewell">
      <segment>
        <source>Goodbye</source>
        <target>[Ĝööđƃýé]</target>
      </segment>
    </unit>
  </file>
</xliff>
```

▒ Ţĥîš éẋàçţ þŕöĝŕàḿ ļîṽéš îñ ţĥé ŕéþöšîţöŕý üñđéŕ
[`éẋàḿþļéš/ĝö-ǫüîçķšţàŕţ/`](https://github.com/neokapi/neokapi/tree/main/examples/go-quickstart)
àñđ îš ƃüîļţ àš þàŕţ öƒ ţĥé ƒŕàḿéŵöŕķ ḿöđüļé. ▒

## ▒ Ŵĥàţ éàçĥ þîéçé îš ▒

▒ Ţĥé þŕöĝŕàḿ ţöüçĥéš éṽéŕý çöŕé çöñçéþţ ţĥé ŕéšţ öƒ ţĥîš šéçţîöñ çöṽéŕš îñ
đéþţĥ. ▒

- ▒ **Ţĥé ŕéĝîšţŕý** ([`çöŕé/ŕéĝîšţŕý`](/framework/formats)) ḿàþš à ƒöŕḿàţ îđ ţö à
  ŕéàđéŕ àñđ ŵŕîţéŕ ƒàçţöŕý. `ƒöŕḿàţš.ŔéĝîšţéŕÀļļ` þöþüļàţéš îţ ŵîţĥ éṽéŕý
  ƃüîļţ-îñ [ƒöŕḿàţ](/framework/formats); `ÑéŵŔéàđéŕ` / `ÑéŵŴŕîţéŕ` ĥàñđ ƃàçķ à
  ƒŕéšĥ îñšţàñçé. Ţĥé ŕéĝîšţŕý àļšö đéţéçţš à ƒöŕḿàţ ƒŕöḿ à þàţĥ öŕ ḾÎḾÉ ţýþé
  ŵĥéñ ýöü đöñ'ţ ñàḿé öñé éẋþļîçîţļý. ▒
- ▒ **Ţĥé ŕéàđéŕ** ţüŕñš ţĥé šöüŕçé ƒîļé îñţö à šţŕéàḿ öƒ
  [Þàŕţš](/framework/content-model). `Öþéñ` ƃîñđš à `ŔàŵĐöçüḿéñţ`; `Ŕéàđ` ŕéţüŕñš
  à çĥàññéļ öƒ `ÞàŕţŔéšüļţ` (à `*Þàŕţ` þļüš àñ öþţîöñàļ éŕŕöŕ). À ḿöñöļîñĝüàļ
  ƒöŕḿàţ ļîķé ĴŠÖÑ éḿîţš öñé [Ƃļöçķ](/framework/content-model) þéŕ ţŕàñšļàţàƃļé
  ṽàļüé, šüŕŕöüñđéđ ƃý ļàýéŕ-šţàŕţ / ļàýéŕ-éñđ Þàŕţš ţĥàţ çàŕŕý ţĥé đöçüḿéñţ
  šţŕüçţüŕé. ▒
- ▒ **Ţĥé çöñţéñţ ḿöđéļ** ([`çöŕé/ḿöđéļ`](/framework/content-model)) îš ŵĥàţ ƒļöŵš
  öñ ţĥé çĥàññéļš. À `Þàŕţ` çàŕŕîéš à ţýþé đîšçŕîḿîñàţöŕ àñđ à `Ŕéšöüŕçé`; à
  `Ƃļöçķ` îš ţĥé ţŕàñšļàţàƃļé üñîţ, ŵîţĥ à ƒļàţ `Šöüŕçé []Ŕüñ`, à ḿàþ öƒ
  ṽàŕîàñţ-ķéýéđ `Ţàŕĝéţ`š, àñđ šţàñđ-öƒƒ öṽéŕļàýš. `ƃļöçķ.ŠöüŕçéŢéẋţ()` þŕöĵéçţš
  ţĥé šöüŕçé ŕüñš ţö þļàîñ ţéẋţ; `ƃļöçķ.ŠéţŢàŕĝéţŢéẋţ(ļöçàļé, …)` àñđ
  `ƃļöçķ.ŢàŕĝéţŢéẋţ(ļöçàļé)` ŕéàđ àñđ ŵŕîţé à ţàŕĝéţ. Îñļîñé ḿàŕķüþ (ĤŢḾĻ ţàĝš,
  ÎÇÜ þļàçéĥöļđéŕš) ļîṽéš îñ `Ŕüñ`š, ñöţ îñ ţĥé ţéẋţ, šö à ţööļ çàñ éđîţ ŵöŕđš
  ŵîţĥöüţ đîšţüŕƃîñĝ ţĥé ḿàŕķüþ. ▒
- ▒ **Ţĥé ţööļ** ([`çöŕé/ţööļš`](/framework/tools)) îš à šţàĝé ţĥàţ šàţîšƒîéš ţĥé
  `Þŕöçéšš(çţẋ, îñ, öüţ)` çöñţŕàçţ: îţ çöñšüḿéš Þàŕţš, ţŕàñšƒöŕḿš ţĥé öñéš îţ
  ĥàñđļéš, àñđ ŕéļàýš ţĥé ŕéšţ. `þšéüđö-ţŕàñšļàţé` ŵŕîţéš à ţàŕĝéţ ƒöŕ éàçĥ
  Ƃļöçķ; šŵàþ îţ ƒöŕ `çàšé-ţŕàñšƒöŕḿ`, `šéàŕçĥ-ŕéþļàçé`, öŕ àñý öţĥéŕ ƃüîļţ-îñ, öŕ
  çĥàîñ šéṽéŕàļ ţöĝéţĥéŕ. ▒
- ▒ **Ţĥé þîþéļîñé** ([`çöŕé/ƒļöŵ`](/framework/pipeline)) îš ţĥé çöñçüŕŕéñçý: éàçĥ
  šţàĝé îš à ĝöŕöüţîñé, ţĥé šţàĝéš àŕé ĵöîñéđ ƃý ƃüƒƒéŕéđ çĥàññéļš öƒ Þàŕţš, àñđ
  àñ `éŕŕĝŕöüþ` þŕöþàĝàţéš ţĥé ƒîŕšţ éŕŕöŕ àñđ çàñçéļš ţĥé ŕéšţ. Ţĥé éẋàḿþļé
  ŵîŕéš ţĥé çĥàîñ ƃý ĥàñđ ţö šĥöŵ ţĥé ḿéçĥàñîçš; ƒöŕ ƃàţçĥéš öƒ ƒîļéš ţĥéŕé îš à
  ĥîĝĥéŕ-ļéṽéļ éẋéçüţöŕ (ƃéļöŵ). ▒

## ▒ Ŕüññîñĝ ƒļöŵš îñšţéàđ öƒ ŵîŕîñĝ çĥàññéļš ▒

▒ Ŵîŕîñĝ ţĥé çĥàññéļš ƃý ĥàñđ, àš àƃöṽé, îš ţĥé çļéàŕéšţ ŵàý ţö šéé ĥöŵ Þàŕţš
ḿöṽé — ƃüţ ýöü ŕàŕéļý ñééđ ţö. Ƒöŕ à šîñĝļé ƒîļé, `ƒļöŵ.ÑéŵƑîļéŔüññéŕ` ŕüñš ţĥé
ŵĥöļé ŕéàđ → þŕöçéšš → ŵŕîţé þîþéļîñé (ƒöŕḿàţ đéţéçţîöñ, ŕéàđéŕ/ŵŕîţéŕ çŕéàţîöñ,
ţööļ çĥàîñ, öüţþüţ) ƒöŕ ýöü: ▒

```go
runner := flow.NewFileRunner(flow.FileRunnerConfig{
	FormatReg:    reg,
	SourceLocale: "en",
})
err := runner.RunFile(ctx, "pseudo", []tool.Tool{pseudo},
	"messages.json", "messages.out.json", "fr")
```

▒ Ƒöŕ ƃàţçĥéš öƒ ƒîļéš ŕüñ îñ þàŕàļļéļ, `ƒļöŵ.ÑéŵÉẋéçüţöŕ` ţàķéš à ƃüîļţ ƒļöŵ àñđ à
šļîçé öƒ îţéḿš àñđ ŕüñš ţĥéḿ çöñçüŕŕéñţļý, ƃöüñđéđ ƃý `ḾàẋÇöñçüŕŕéñçý`. Šéé
[Þîþéļîñé](/framework/pipeline) ƒöŕ ţĥé éẋéçüţöŕ öþţîöñš àñđ ţĥé çöñçüŕŕéñçý
ḿöđéļ, àñđ [Ƒļöŵš](/framework/flows) ƒöŕ çöḿþöšîñĝ ñàḿéđ ţööļ çĥàîñš. ▒

## ▒ Ŵĥéŕé ţö ĝö ñéẋţ ▒

- ▒ [Çöñţéñţ Ḿöđéļ](/framework/content-model) — Þàŕţš, Ƃļöçķš, Ŕüñš, Ţàŕĝéţš, àñđ
  öṽéŕļàýš îñ đéþţĥ. ▒
- ▒ [Ƒöŕḿàţš](/framework/formats) — ţĥé ƃüîļţ-îñ ŕéàđéŕš àñđ ŵŕîţéŕš, đéţéçţîöñ,
  àñđ ţĥé ĝéñéŕàţéđ [Ƒöŕḿàţ Ŕéƒéŕéñçé](/formats). ▒
- ▒ [Ţööļš](/framework/tools) — ţĥé ţööļ îñţéŕƒàçé, `ƂàšéŢööļ` đîšþàţçĥ, àñđ ţĥé
  ĝéñéŕàţéđ [Ţööļ Ŕéƒéŕéñçé](/tools). ▒
- ▒ [Þîþéļîñé](/framework/pipeline) àñđ [Ƒļöŵš](/framework/flows) — ţĥé éẋéçüţöŕ,
  çĥàññéļš, ƃàçķþŕéššüŕé, àñđ ñàḿéđ çöḿþöšîţîöñš. ▒
- ▒ [Îḿþļéḿéñţîñĝ à Ţööļ](/contribute/tools) àñđ
  [Îḿþļéḿéñţîñĝ à Ƒöŕḿàţ](/contribute/formats) — éẋţéñđ ţĥé ƒŕàḿéŵöŕķ ŵîţĥ ýöüŕ
  öŵñ šţàĝéš àñđ ŕéàđéŕš/ŵŕîţéŕš. ▒
```

