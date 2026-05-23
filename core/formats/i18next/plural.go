package i18next

import (
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// This file recognises i18next plural and context sibling keys from their key
// suffix and produces the metadata that the reader attaches to each block. It
// does NOT merge siblings — each key stays a one-to-one block so the JSON
// writer's per-value token replacement keeps the round-trip byte-faithful.
//
// i18next plural conventions (https://www.i18next.com/translation-function/plurals):
//
//	v4 (CLDR):   key_zero key_one key_two key_few key_many key_other
//	legacy:      key            (singular)         key_plural (plural)
//	             key_0 key_1 key_2 …               (indexed forms)
//
// Context conventions (https://www.i18next.com/translation-function/context):
//
//	key            (base)
//	key_male       (context "male")
//	key_female     (context "female")
//
// Plural and context can combine (e.g. friend_male_one). i18next splits a key
// on the last underscore-delimited suffix(es); we mirror that, peeling a
// trailing plural suffix first and then a trailing context suffix.

// cldrPluralCategories are the CLDR plural categories used by i18next v4 as key
// suffixes. "other" is always present; the rest appear per the target
// language's plural rules.
var cldrPluralCategories = map[string]bool{
	"zero":  true,
	"one":   true,
	"two":   true,
	"few":   true,
	"many":  true,
	"other": true,
}

// keyInfo is the i18next interpretation of a single key's suffix structure.
type keyInfo struct {
	base            string // the base key with plural/context suffixes removed
	pluralCategory  string // CLDR category ("one", "other", …) or "" if not plural
	legacyPlural    bool   // true when matched via key_plural / key_N legacy forms
	legacyPluralRaw string // the raw legacy suffix ("plural", "0", "1", …) when legacyPlural
	context         string // context value ("male", …) or "" if no context
}

// analyzeKey decomposes the immediate (leaf) key name into its i18next base,
// plural category, and context. allowLegacy enables the legacy key_plural /
// key_N plural forms in addition to the v4 CLDR suffixes.
func analyzeKey(key string, allowLegacy bool) keyInfo {
	info := keyInfo{base: key}

	// Peel a trailing plural suffix first.
	if idx := strings.LastIndex(info.base, "_"); idx >= 0 {
		suffix := info.base[idx+1:]
		prefix := info.base[:idx]
		switch {
		case cldrPluralCategories[suffix]:
			info.pluralCategory = suffix
			info.base = prefix
		case allowLegacy && suffix == "plural":
			info.pluralCategory = "other"
			info.legacyPlural = true
			info.legacyPluralRaw = suffix
			info.base = prefix
		case allowLegacy && isDigits(suffix):
			info.pluralCategory = legacyIndexCategory(suffix)
			info.legacyPlural = true
			info.legacyPluralRaw = suffix
			info.base = prefix
		}
	}

	// Then peel a trailing context suffix from whatever base remains. A context
	// is any remaining underscore-delimited suffix that is not itself a plural
	// marker — i18next uses an arbitrary string after the key separator.
	if idx := strings.LastIndex(info.base, "_"); idx >= 0 {
		suffix := info.base[idx+1:]
		prefix := info.base[:idx]
		if suffix != "" && prefix != "" && !cldrPluralCategories[suffix] &&
			!(allowLegacy && (suffix == "plural" || isDigits(suffix))) {
			info.context = suffix
			info.base = prefix
		}
	}

	return info
}

// isPlural reports whether the key carries a plural suffix.
func (k keyInfo) isPlural() bool { return k.pluralCategory != "" }

// hasContext reports whether the key carries a context suffix.
func (k keyInfo) hasContext() bool { return k.context != "" }

// isDecorated reports whether the key carries any i18next suffix at all.
func (k keyInfo) isDecorated() bool { return k.isPlural() || k.hasContext() }

// legacyIndexCategory maps a legacy numeric plural index to a CLDR-ish category
// for annotation purposes. i18next's pre-v4 numeric forms used 0 for the
// singular-ish case and 1 for the plural-ish case; higher indices are language
// specific. We map 0→one and 1→other (the common English mapping) and leave any
// higher index annotated by its raw index via legacyPluralRaw.
func legacyIndexCategory(digits string) string {
	switch digits {
	case "0":
		return "one"
	case "1":
		return "other"
	default:
		return "other"
	}
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// leafKey extracts the immediate (leaf) key name from a JSON key path. The JSON
// reader records the raw dotted/bracketed path on the block (block.Name when
// UseKeyAsName produces a slash path, or the json.keypath property). We work
// from the dotted keypath the reader stores so array indices and nesting are
// handled uniformly.
func leafKey(keypath string) string {
	// keypath is dotted (a.b.c) with optional [n] array indices. The leaf is the
	// final dotted segment; strip any trailing array index.
	seg := keypath
	if idx := strings.LastIndex(seg, "."); idx >= 0 {
		seg = seg[idx+1:]
	}
	if br := strings.IndexByte(seg, '['); br >= 0 {
		seg = seg[:br]
	}
	return seg
}

// parentPath returns the key path with the leaf segment removed, used to scope
// plural-group identifiers so identically named bases in different namespaces
// do not collide.
func parentPath(keypath string) string {
	if idx := strings.LastIndex(keypath, "."); idx >= 0 {
		return keypath[:idx]
	}
	return ""
}

// pluralGroupID builds a stable identifier shared by every sibling block in a
// plural group: the parent namespace path plus the base key. Tooling can use
// this to reassemble a plural set from the flat block stream.
func pluralGroupID(keypath, base string) string {
	parent := parentPath(keypath)
	if parent == "" {
		return base
	}
	return parent + "." + base
}

// annotateBlock applies i18next plural/context metadata to a block based on the
// analysis of its key. The block's identity (ID, Name, json.keypath, source
// runs) is left untouched so the JSON writer round-trips it byte-faithfully.
func annotateBlock(block *model.Block, keypath string, cfg *Config) {
	if block.Properties == nil {
		block.Properties = make(map[string]string)
	}

	info := analyzeKey(leafKey(keypath), cfg.LegacyPluralForms)
	if !info.isDecorated() {
		return
	}

	block.Properties["i18next.baseKey"] = info.base

	if info.isPlural() {
		block.Properties["i18next.pluralCategory"] = info.pluralCategory
		block.Properties["i18next.pluralGroup"] = pluralGroupID(keypath, info.base)
		if info.legacyPlural {
			block.Properties["i18next.pluralLegacy"] = "true"
			block.Properties["i18next.pluralLegacyForm"] = info.legacyPluralRaw
		}
	}

	if info.hasContext() {
		block.Properties["i18next.context"] = info.context
	}
}
