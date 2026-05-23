package xcstrings

// This file defines the in-memory model of an Apple String Catalog
// (.xcstrings) and the path scheme used to address individual translatable
// string values inside it.
//
// A catalog is a JSON document of the shape:
//
//	{
//	  "sourceLanguage" : "en",
//	  "strings" : {
//	    "<key>" : {
//	      "extractionState" : "manual",
//	      "comment" : "developer note",
//	      "localizations" : {
//	        "<lang>" : { "stringUnit" : { "state" : "translated", "value" : "…" } },
//	        "<lang>" : { "variations" : { … } }
//	      }
//	    }
//	  },
//	  "version" : "1.0"
//	}
//
// Each leaf "value" string is translatable. The path scheme assigns every
// such value a stable address so the reader can emit one Block per value and
// the writer can splice translations back into the exact same location. The
// path encodes the entry key, the localization language, and the variation
// trail (plural category, device class, substitution name) leading to the
// value.

// catalog is the decoded top-level structure. Field order in the raw JSON is
// preserved separately via the token stream used for byte-faithful writing;
// this struct is only used to drive Block emission and to look up which leaf
// values are translatable.
type catalog struct {
	SourceLanguage string
	Version        string
	// keys preserves entry order as it appears in the source document.
	keys    []string
	entries map[string]*entry
}

// entry is a single string-catalog key.
type entry struct {
	Comment         string
	ExtractionState string // "manual", "stale", "extracted_with_value", "migrated", ""
	// localizations maps a BCP-47 language tag to its localization payload.
	// langOrder preserves the order languages appear in the source.
	langOrder     []string
	localizations map[string]*localization
}

// localization is the payload under a single language: either a plain
// stringUnit or a variations subtree.
type localization struct {
	StringUnit *stringUnit
	Variations *variations
}

// stringUnit is the leaf {state, value} pair.
type stringUnit struct {
	State string
	Value string
}

// variations holds the plural / device / substitutions subtrees that can
// appear under a localization. Each is optional. Substitutions can co-occur
// with plural or device variations on the same localization; Apple nests
// substitution argument plural rules under their own subtree.
type variations struct {
	// plural maps a CLDR plural category (zero/one/two/few/many/other) to a
	// leaf stringUnit. pluralOrder preserves source order.
	pluralOrder []string
	plural      map[string]*stringUnit

	// device maps a device class (iphone/ipad/mac/appletv/applewatch/…) to a
	// leaf stringUnit. deviceOrder preserves source order.
	deviceOrder []string
	device      map[string]*stringUnit

	// substitutions maps a named argument to its substitution definition.
	// substitutionOrder preserves source order.
	substitutionOrder []string
	substitutions     map[string]*substitution
}

// substitution is a named substitution argument with its own variations
// subtree (typically plural categories). argNum and formatSpecifier are
// non-translatable metadata preserved on round-trip.
type substitution struct {
	ArgNum          int
	HasArgNum       bool
	FormatSpecifier string
	// Each substitution carries its own variation subtree (plural categories
	// most commonly, occasionally device).
	vars *variations
}
