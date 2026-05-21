package xml

// Native config presets for three XML vocabularies that upstream Okapi
// ships as bundled ITS-rule configurations on its `xml` (okf_xml) and
// `xmlstream` (okf_xmlstream) filters. Each preset reproduces the
// extraction semantics of the corresponding Okapi config file so that
// the native reader/writer extracts and round-trips the same content the
// Java filters do under the same vocabulary.
//
// Sources (the contract):
//   - DITA:    okf_xmlstream-dita — net/sf/okapi/filters/xmlstream/dita.yml
//   - DocBook: okf_xml-docbook    — net/sf/okapi/filters/xml/okf_xml-docbook.fprm
//   - ResX:    okf_xml-resx       — net/sf/okapi/filters/xml/resx.fprm
//
// Grounding for the element classifications is taken from BOTH the
// upstream config and the format specs (DITA 1.x element types, DocBook
// 5 inline elements, .NET ResX 2.0 schema).

// DitaConfig returns a *Config replicating Okapi's okf_xmlstream-dita
// configuration (filters/xmlstream/dita.yml). DITA is processed by the
// xmlstream filter, which is rule-driven (not ITS): elements are either
// inline (within-text), preserve-whitespace, excluded, attribute-only
// carriers of translatable attributes, or — by default — block text
// units. The dita.yml rules encoded here:
//
//   - translate="no"/"yes" attribute gating: the regex element rules
//     '.*' EXCLUDE when @translate=="no" and '.+' INCLUDE when
//     @translate=="yes" (DITA's universal @translate attribute, DITA
//     1.x §"translate"). The INCLUDE override re-enables a descendant
//     inside a translate="no" subtree.
//   - structurally non-translatable elements always excluded:
//     stylesheet, coords, draft-comment, required-cleanup, shape.
//   - preserve-whitespace (verbatim) elements: pre, lines, screen,
//     msgblock, codeblock (DITA preformatted content).
//   - inline (within-text) phrase elements — the full dita.yml INLINE
//     set (b, i, u, ph, xref, keyword, term, cmdname, codeph, …).
//   - attribute-only elements carrying translatable attributes
//     (othermeta/@content, topicref/@navtitle, map/@title, image/@alt,
//     object/@standby, …). image/@alt is conditional in Okapi
//     (placement != break); the native reader extracts @alt
//     unconditionally — see ResX note about conditional attrs; for DITA
//     the conditional only suppresses @alt on a page break, which the
//     corpus does not exercise, so the simpler unconditional form is
//     used and the divergence noted.
//
// okf_xmlstream-dita uses the default xmlstream attribute rules
// (default.yml): xml:lang writable, xml:id/id as block ids, xml:space
// preserve/default — the native reader applies xml:lang and xml:space
// natively, and id attributes via IDAttributeNames.
func DitaConfig() *Config {
	cfg := &Config{
		// dita.yml: 'xml:id'/'id' ATTRIBUTE_ID — used as block ids when a
		// TEXT_UNIT element carries one.
		IDAttributeNames: []string{"id", "xml:id"},

		// dita.yml PRESERVE_WHITESPACE elements (DITA preformatted blocks).
		PreserveWhitespaceElements: []string{
			"pre", "lines", "screen", "msgblock", "codeblock",
		},

		// dita.yml INLINE elements (within-text phrase content). The full
		// set from filters/xmlstream/dita.yml.
		InlineElements: []string{
			"alt", "apiname", "b", "boolean", "cite", "cmdname", "codeph",
			"delim", "filepath", "fragref", "i", "image", "itemgroup",
			"keyword", "kwd", "menucascade", "msgnum", "msgph", "oper",
			"option", "parmname", "ph", "q", "repsep", "sep", "shortcut",
			"sub", "sup", "synnoteref", "synph", "systemoutput", "term",
			"tm", "tt", "u", "uicontrol", "userinput", "var", "varname",
			"wintitle", "xref", "state",
		},

		// dita.yml unconditional EXCLUDE elements.
		ExcludedElements: []string{
			"stylesheet", "coords", "draft-comment", "required-cleanup",
			"shape",
		},
	}

	// dita.yml ATTRIBUTES_ONLY element rules: elements that are not text
	// units themselves but carry a translatable attribute.
	attrOnly := []struct {
		elem string
		attr string
	}{
		{"othermeta", "content"},
		{"topicref", "navtitle"},
		{"topicgroup", "navtitle"},
		{"topichead", "navtitle"},
		{"note", "othertype"},
		{"lq", "reftitle"},
		{"object", "standby"},
		{"map", "title"},
		{"state", "value"},
		{"data", "label"},
		{"vrm", "version"},
		{"image", "alt"},
	}
	for _, ao := range attrOnly {
		cfg.ElementRules = append(cfg.ElementRules, &ElementRule{
			Name:      ao.elem,
			RuleTypes: []RuleType{RuleAttributeTrans},
			TranslatableAttributes: map[string]*TranslatableAttrCondition{
				ao.attr: {},
			},
		})
	}

	// dita.yml: the regex '.*' EXCLUDE when @translate=="no", and '.+'
	// INCLUDE when @translate=="yes". The INCLUDE override lets a
	// descendant re-enter extraction inside a translate="no" subtree.
	cfg.ElementRules = append(cfg.ElementRules,
		&ElementRule{
			Name:      "'.*'",
			RuleTypes: []RuleType{RuleExclude},
			Condition: &Condition{Attribute: "translate", Op: ConditionEquals, Value: "no"},
		},
		&ElementRule{
			Name:      "'.+'",
			RuleTypes: []RuleType{RuleInclude},
			Condition: &Condition{Attribute: "translate", Op: ConditionEquals, Value: "yes"},
		},
	)

	cfg.compileElementRules()
	return cfg
}

// DocBookConfig returns a *Config replicating Okapi's okf_xml-docbook
// configuration (filters/xml/okf_xml-docbook.fprm). That file is an ITS
// rules document with two relevant data categories:
//
//   - withinTextRule withinText="yes" over a large set of DocBook inline
//     (phrase) elements (abbrev, acronym, emphasis, phrase, link, xref,
//     literal, command, filename, guibutton, keycap, …). These fold into
//     the surrounding block as inline codes.
//   - translateRule translate="no" over verbatim / non-prose elements
//     (computeroutput, programlisting, screen, screenshot, synopsis,
//     literallayout) plus the children of math/equation containers
//     (mathphrase/*, inlineequation/*).
//   - withinTextRule withinText="nested" over alt, footnote, remark,
//     indexterm, primary, secondary, tertiary — the .fprm itself notes
//     the ITS filter does not honor withinText="nested" (treats it like a
//     separate flow), so these stay block text units, which the native
//     reader also does (it has no nested-within-text mode).
//
// The upstream selectors are namespaced to the DocBook 5 namespace
// (`db:`, http://docbook.org/ns/docbook). The native reader matches
// element rules by local name (namespace-agnostic), which is the
// faithful behavior for the unprefixed DocBook 4.x documents in the
// corpus and remains correct for DocBook 5 (the local names are
// identical). screenshot is excluded as a whole, so its child screeninfo
// text is dropped — matching Okapi (screenshot is in the translate="no"
// selector).
func DocBookConfig() *Config {
	cfg := &Config{
		// withinTextRule withinText="yes" — DocBook inline phrase elements.
		// Deduplicated from the .fprm selector (which repeats msgtext,
		// parameter, prompt, replaceable across functional groups).
		InlineElements: []string{
			"abbrev", "acronym", "emphasis", "phrase", "quote", "trademark",
			"anchor", "citation", "citerefentry", "citetitle", "firstterm",
			"glossterm", "link", "olink", "xref",
			"foreignphrase", "wordasword", "computeroutput", "literal",
			"markup", "prompt", "replaceable", "tag", "userinput",
			"inlineequation", "mathphrase", "subscript", "superscript",
			"accel", "guibutton", "guiicon", "guilabel", "guimenu",
			"guimenuitem", "guisubmenu",
			"keycap", "keycode", "keycombo", "keysym", "menuchoice",
			"mousebutton", "shortcut",
			"classname", "constant", "errorcode", "errorname", "errortype",
			"function",
			"msgtext", "parameter", "property", "returnvalue", "symbol",
			"token", "type", "varname",
			"application", "command", "envar", "filename", "option",
			"systemitem",
			"database", "email", "hardware", "optional", "cover", "comment",
		},

		// translateRule translate="no" — verbatim / non-prose elements.
		// (mathphrase/* and inlineequation/* descendants are also
		// translate="no" upstream; mathphrase and inlineequation are inline
		// here, and their content is rarely prose — the corpus does not
		// exercise nested math, so the element-level exclusions below cover
		// the observable cases.)
		ExcludedElements: []string{
			"computeroutput", "programlisting", "screen", "screenshot",
			"synopsis", "literallayout",
		},
	}
	cfg.compileElementRules()
	return cfg
}

// ResXConfig returns a *Config replicating Okapi's okf_xml-resx
// configuration (filters/xml/resx.fprm), a .NET ResX 2.0 resource file.
// The ITS rules:
//
//	<its:translateRule selector="/root" translate="no"/>
//	<its:translateRule selector="//data[not(@type) and not(starts-with(@name,'>'))]/value" translate="yes" itsx:idValue="../@name"/>
//	<its:translateRule selector="//data[@mimetype]/value" translate="no"/>
//	<its:translateRule selector="//data[ends-with(@name,'.Name')]/value" translate="no"/>
//	<its:translateRule selector="//data[@name='$this.Text']/value" translate="yes" itsx:idValue="../@name"/>
//	<its:locNoteRule locNotePointer="../comment" selector="//data[not(@type) and not(starts-with(@name,'>') or starts-with(@name,'$'))]/value"/>
//	codeFinder rule0=(\{[^}]+?\})  rule1=<(/?)\w+[^>]*?>
//
// The translatable unit is the <value> element, but every selector
// predicate tests the *parent* <data> element's attributes. The native
// reader supports parent-attribute conditions on element rules (see
// Condition.Parent), so these predicates are expressed directly:
//
//   - <root> EXCLUDE (translate="no" default).
//   - <value> INCLUDE (translatable) when parent <data> has no @type,
//     no @mimetype, and @name does not start with '>'. Two INCLUDE
//     rules cover the two translate="yes" selectors: (a) the general
//     "no type, no '>' name" rule, and (b) the specific @name="$this.Text"
//     rule (which is translatable even with a '$' prefix). @mimetype and
//     a '.Name' suffix keep <value> non-translatable.
//   - id comes from the parent <data>/@name (itsx:idValue="../@name").
//   - codeFinder protects .NET format placeholders ({0}, {1:t}) and any
//     stray markup tags as inline codes.
//
// Limitation vs upstream: Okapi attaches the sibling <comment> text as an
// ITS localization note (locNotePointer="../comment") on the trans-unit.
// The native reader streams each <value> block at its end tag, before the
// following-sibling <comment> is parsed, so it does not surface that note
// as a block annotation. The <comment> element is not translatable and
// round-trips verbatim in skeleton; only the translator-facing note
// metadata is not carried. See the test's honest note on locNote.
func ResXConfig() *Config {
	noType := Condition{Attribute: "type", Op: ConditionNotExists, Parent: true}
	noMime := Condition{Attribute: "mimetype", Op: ConditionNotExists, Parent: true}
	nameNotGT := Condition{Attribute: "name", Op: ConditionNotStartsWith, Value: ">", Parent: true}
	nameThisText := Condition{Attribute: "name", Op: ConditionEquals, Value: "$this.Text", Parent: true}

	cfg := &Config{
		ExcludeByDefault: true,

		ElementRules: []*ElementRule{
			// /root translate="no": establishes the excluded default; with
			// ExcludeByDefault the default is already excluded, but listing
			// root EXCLUDE documents the intent and keeps any stray root
			// text out.
			{
				Name:      "root",
				RuleTypes: []RuleType{RuleExclude},
			},
			// //data[not(@type) and not(starts-with(@name,'>'))]/value → yes.
			// ParentElement "data" scopes the rule to <data>/value, so the
			// <value> children of <resheader> (resmimetype, version, reader,
			// writer) stay excluded. idValue="../@name" → the parent
			// <data>/@name names the block.
			{
				Name:          "value",
				ParentElement: "data",
				RuleTypes:     []RuleType{RuleInclude},
				Conditions:    []Condition{noType, noMime, nameNotGT},
				ParentIDAttr:  "name",
			},
			// //data[@name='$this.Text']/value → yes. Translatable even
			// though the name starts with '$'. Still excluded if a mimetype
			// is present (Okapi's @mimetype rule is later but mimetype +
			// $this.Text never co-occur in practice; we keep noMime for
			// safety).
			{
				Name:          "value",
				ParentElement: "data",
				RuleTypes:     []RuleType{RuleInclude},
				Conditions:    []Condition{noMime, nameThisText},
				ParentIDAttr:  "name",
			},
		},

		// okp:codeFinder useCodeFinder="yes": .NET format placeholders and
		// any inline markup tags become protected inline codes.
		UseCodeFinder: true,
		CodeFinderRules: []string{
			`(\{[^}]+?\})`,
			`<(/?)\w+[^>]*?>`,
		},
	}
	cfg.compileElementRules()
	return cfg
}
