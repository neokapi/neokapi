package arb

// This file defines the in-memory model of a Flutter Application Resource
// Bundle (.arb) and the addressing scheme used to round-trip individual
// message values.
//
// An ARB file is a flat JSON object of the shape:
//
//	{
//	  "@@locale": "en",
//	  "@@last_modified": "2024-01-15T10:30:00.000Z",
//	  "appTitle": "Flutter Gallery",
//	  "@appTitle": { "description": "…", "placeholders": { … } },
//	  "itemCount": "{count, plural, =0{No items} other{{count} items}}",
//	  "@itemCount": { "placeholders": { "count": { "type": "int" } } }
//	}
//
// Three kinds of keys appear at the top level:
//
//   - "@@<name>" — file-global metadata (@@locale, @@last_modified,
//     @@author, @@context, …). Never translatable; preserved verbatim.
//   - "@<resourceId>" — the *attributes* object describing the sibling
//     resource (description, placeholders, type, …). Never translatable;
//     preserved verbatim. Its "description" is surfaced as the resource's
//     translator note.
//   - "<resourceId>" — a translatable message. Its string value is the
//     ICU MessageFormat message text. This is the only translatable content.
//
// ARB is *monolingual*: one file per locale, named by "@@locale" (or by the
// file name, e.g. app_fr.arb). The message value is the source text in the
// template locale and the target text in a translation. The reader therefore
// emits one Block per resource, carrying the value as source content; the
// writer splices a changed translation back into the exact same value
// position, leaving every other byte (key order, JSON escaping, @/@@ metadata)
// untouched.

// catalog is the decoded ARB document. Raw byte order is preserved separately
// via the token stream used for byte-faithful writing; this struct only drives
// Block emission and records which keys carry translatable values and which
// attributes describe them.
type catalog struct {
	// locale is the value of "@@locale" if present (empty otherwise).
	locale string
	// keyOrder preserves the order resource keys (the non-@ message keys)
	// appear in the source document.
	keyOrder []string
	// resources maps a resource id to its decoded message value.
	resources map[string]*resource
}

// resource is a single translatable message together with the description
// pulled from its sibling "@<id>" attributes object (if any).
type resource struct {
	// id is the resource key (e.g. "appTitle").
	id string
	// value is the raw decoded message string (ICU MessageFormat text).
	value string
	// description is the "description" field of the sibling "@<id>" attributes
	// object, surfaced as the Block's translator note. Empty when absent.
	description string
}
