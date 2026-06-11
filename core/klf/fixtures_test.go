package klf

// Shared fixtures, ported line-for-line from
// packages/kapi-format/examples/{files-heading,tag-chip,shopping-cart,annotations}.ts.
// These are the golden blocks that must round-trip through the Go
// reader/writer byte-for-byte equivalent to the TypeScript side.

// filesHeading — inline <span> paired code wrapping "(count matched)"
// and a {count} variable placeholder between the opening and closing
// halves.
func filesHeading() *Block {
	return &Block{
		ID:           "files-heading",
		Hash:         "2xykvb",
		Translatable: true,
		Type:         BlockTypeJSXElement,
		Source: []Run{
			{Text: &TextRun{Text: "Files "}},
			{PcOpen: &PcOpenRun{
				ID: "1", Type: "jsx:element", SubType: "span",
				Data:  `<span className="muted">`,
				Equiv: "muted", Disp: "span",
			}},
			{Text: &TextRun{Text: "("}},
			{Ph: &PlaceholderRun{
				ID: "2", Type: "jsx:var", SubType: "number",
				Data: "{count}", Equiv: "count", Disp: "count",
			}},
			{Text: &TextRun{Text: " matched)"}},
			{PcClose: &PcCloseRun{
				ID: "1", Type: "jsx:element", SubType: "span",
				Data: "</span>", Equiv: "muted",
			}},
		},
		Placeholders: []Placeholder{
			{Name: "muted", Kind: PlaceholderElement, SourceExpr: `<span className="muted">...</span>`, JSType: "ReactNode"},
			{Name: "count", Kind: PlaceholderVariable, SourceExpr: "count", JSType: "number"},
		},
		Properties: BlockProperties{
			File: "src/FilesHeading.tsx", Line: 4,
			Component: "FilesHeading", JSXPath: "FilesHeading > h2", Element: "h2",
		},
		Preview: &BlockPreviewHints{
			SampleValues: map[string]any{"count": float64(3)},
		},
	}
}

const filesHeadingExpectedHTML = `<kat-block id="files-heading" data-type="jsx:element">` +
	`Files <span data-neokapi-span="1">(` +
	`<span class="neokapi-var" data-var="count" data-type="number">count</span>` +
	` matched)</span>` +
	`</kat-block>`

// tagChip — three chips (two jsx:node placeholders and one jsx:var
// placeholder) separated by spaces, with optional jsx:node
// placeholders for conditional JSX expressions.
func tagChip() *Block {
	return &Block{
		ID:           "tag-chip",
		Hash:         "2GcSuQ",
		Translatable: true,
		Type:         BlockTypeJSXElement,
		Source: []Run{
			{Ph: &PlaceholderRun{
				ID: "1", Type: "jsx:node", SubType: "logical-and",
				Data:  `index !== undefined && <span className="badge">{index}</span>`,
				Equiv: "badge", Disp: "⟨badge⟩",
			}},
			{Text: &TextRun{Text: " "}},
			{Ph: &PlaceholderRun{
				ID: "2", Type: "jsx:var", SubType: "string",
				Data: "{label}", Equiv: "label", Disp: "label",
			}},
			{Text: &TextRun{Text: " "}},
			{Ph: &PlaceholderRun{
				ID: "3", Type: "jsx:node", SubType: "logical-and",
				Data:  `!deletable && <span className="required">*</span>`,
				Equiv: "required", Disp: "⟨required⟩",
			}},
		},
		Placeholders: []Placeholder{
			{Name: "badge", Kind: PlaceholderNode, SourceExpr: `index !== undefined && <span className="badge">{index}</span>`, JSType: "ReactNode", Optional: true},
			{Name: "label", Kind: PlaceholderVariable, SourceExpr: "label", JSType: "string"},
			{Name: "required", Kind: PlaceholderNode, SourceExpr: `!deletable && <span className="required">*</span>`, JSType: "ReactNode", Optional: true},
		},
		Properties: BlockProperties{
			File: "src/TagChip.tsx", Line: 3,
			Component: "TagChip", JSXPath: "TagChip > span[data-tag-chip]", Element: "span",
			LocNote: "Tag chip shown in the sidebar list of filters.",
		},
		Preview: &BlockPreviewHints{
			StoryID: "components-tagchip--default",
			SampleValues: map[string]any{
				"label":     "react",
				"index":     float64(3),
				"deletable": true,
			},
		},
	}
}

const tagChipExpectedHTML = `<kat-block id="tag-chip" data-type="jsx:element">` +
	`<span class="neokapi-node" data-node="1" title="index !== undefined &amp;&amp; &lt;span className=&quot;badge&quot;&gt;{index}&lt;/span&gt;">badge</span>` +
	` ` +
	`<span class="neokapi-var" data-var="label" data-type="string">label</span>` +
	` ` +
	`<span class="neokapi-node" data-node="3" title="!deletable &amp;&amp; &lt;span className=&quot;required&quot;&gt;*&lt;/span&gt;">required</span>` +
	`</kat-block>`

// shoppingCart — one structured plural run with three forms (zero,
// one, other). The 'other' form contains a ph run (the {count}
// variable) followed by a text run.
func shoppingCart() *Block {
	return &Block{
		ID:           "shopping-cart-plural",
		Hash:         "9QpZ11",
		Translatable: true,
		Type:         BlockTypeJSXElement,
		Source: []Run{
			{Plural: &PluralRun{
				Pivot: "count",
				Forms: map[PluralForm][]Run{
					PluralZero: {{Text: &TextRun{Text: "Your cart is empty"}}},
					PluralOne:  {{Text: &TextRun{Text: "1 item in your cart"}}},
					PluralOther: {
						{Ph: &PlaceholderRun{
							ID: "1", Type: "jsx:var", SubType: "number",
							Data: "{count}", Equiv: "count", Disp: "count",
						}},
						{Text: &TextRun{Text: " items in your cart"}},
					},
				},
			}},
		},
		Placeholders: []Placeholder{
			{Name: "count", Kind: PlaceholderICUPivot, SourceExpr: "items", JSType: "number"},
		},
		Properties: BlockProperties{
			File: "src/ShoppingCart.tsx", Line: 4,
			Component: "ShoppingCart", JSXPath: "ShoppingCart > p > Plural", Element: "Plural",
		},
		Preview: &BlockPreviewHints{
			SampleValues: map[string]any{"count": float64(3)},
		},
	}
}

const shoppingCartExpectedHTML = `<kat-block id="shopping-cart-plural" data-type="jsx:element">` +
	`<span class="neokapi-plural" data-pivot="count">` +
	`<div class="neokapi-plural-form" data-form="zero">` +
	`<span class="neokapi-plural-form-label">plural:zero</span>` +
	`Your cart is empty` +
	`</div>` +
	`<div class="neokapi-plural-form" data-form="one">` +
	`<span class="neokapi-plural-form-label">plural:one</span>` +
	`1 item in your cart` +
	`</div>` +
	`<div class="neokapi-plural-form" data-form="other">` +
	`<span class="neokapi-plural-form-label">plural:other</span>` +
	`<span class="neokapi-var" data-var="count" data-type="number">count</span>` +
	` items in your cart` +
	`</div>` +
	`</span>` +
	`</kat-block>`

// fixtureDocument bundles the three example blocks into a single
// .klf document for round-trip / archive tests.
func fixtureDocument() *File {
	return &File{
		SchemaVersion: SchemaVersion,
		Kind:          Kind,
		Created:       "2026-04-15T10:00:00Z",
		Generator: GeneratorInfo{
			ID:           "@neokapi/kapi-format-examples",
			Version:      "0.0.1",
			Capabilities: []string{"extract", "preview"},
		},
		Project: ProjectInfo{
			ID:           "neokapi-kapi-format-examples",
			SourceLocale: "en",
		},
		Vocabulary: &Vocabulary{Extends: []string{"common-formatting", "rich-html", "rich-jsx"}},
		Documents: []Document{
			{
				ID:           "examples",
				DocumentType: DocumentTypeJSX,
				Path:         "examples/all.tsx",
				Blocks:       []Block{*filesHeading(), *tagChip(), *shoppingCart()},
			},
		},
	}
}
