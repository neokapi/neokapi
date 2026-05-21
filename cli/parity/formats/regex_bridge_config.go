//go:build parity

package formats

import (
	"encoding/json"
	"fmt"
)

// regexBridgeConfig translates a neokapi-keyed okf_regex spec config
// map into the parameter shape the okapi-bridge daemon's RegexFilter
// (okf_regex) expects.
//
// THE TRANSPORT PROBLEM
// ─────────────────────
// Okapi's RegexFilter is rule-driven: its translatable strings are
// defined by a `rules` ArrayList<Rule> on the filter's Parameters
// object. That list is serialised through Okapi's StringParameters
// "preset" group format (`ruleCount.i`, `rule0.expr`,
// `rule0.ruleType.i`, `rule0.groupSource.i`, …) — a fragile, ordered,
// nested encoding that cannot ride through the bridge's flat gRPC
// FilterParams map<string,string>. With no rules the filter emits zero
// Blocks, so every rule-driven example used to record an expected_fail.
//
// THE TRANSPORT FIX
// ─────────────────
// Instead of reproducing Okapi's StringParameters group serialisation
// in Go (and risking ordering/escaping drift), we ship the rules as a
// self-contained JSON array under a single reserved bridge parameter,
// `regexRulesJson`. The okapi-bridge daemon detects this reserved key,
// parses the JSON, builds real net.sf.okapi.filters.regex.Rule objects
// (setExpression / setRuleType / setSourceGroup / setNameGroup /
// setNoteGroup), appends them to the RegexFilter's Parameters rule
// list, and calls compileRules(). This mirrors Stage A's fprmContent
// reserved-key pattern: no proto change, JSON in a reserved string slot.
//
// JSON CONTRACT (key: regexRulesJson)
// ───────────────────────────────────
//
//	[
//	  {
//	    "expr":        "<Java/RE2 regex>",  // ← neokapi Rule.Pattern
//	    "ruleType":    1,                   // ← RULETYPE_CONTENT (default)
//	    "sourceGroup": 2,                   // ← neokapi Rule.SourceGroup
//	    "nameGroup":   1,                   // ← neokapi Rule.IDGroup   (0 → -1)
//	    "noteGroup":   0                    // ← neokapi Rule.NoteGroup (0 → -1)
//	  },
//	  …
//	]
//
// Field mapping neokapi → Okapi Rule:
//
//	Pattern     → expr        (Rule.setExpression)
//	SourceGroup → sourceGroup (Rule.setSourceGroup)
//	IDGroup     → nameGroup   (Rule.setNameGroup)
//	NoteGroup   → noteGroup   (Rule.setNoteGroup)
//	(constant)  → ruleType = 1 (RULETYPE_CONTENT)
//
// Okapi uses -1 (not 0) as the "no group" sentinel, while neokapi uses
// 0. The bridge handler maps a neokapi 0 in nameGroup/noteGroup back to
// Okapi's -1.
//
// RULETYPE_CONTENT (1), NOT RULETYPE_STRING (0)
// ─────────────────────────────────────────────
// Verified head-to-head against Okapi 1.48.0's RegexFilter: STRING-type
// rules (the macStrings preset shape) treat the source group as a
// *delimited* string — Okapi scans for the configured startString /
// endString delimiters (default `"`) INSIDE the captured group and
// strips them, so the rule's source group must capture the surrounding
// quotes. neokapi's source group captures the INNER text directly. With
// an inner-capture group, STRING-type extraction yields zero TextUnits
// (no delimiter found at the group boundary). CONTENT-type rules emit
// the source group verbatim with no delimiter scanning — this is exactly
// neokapi's behaviour. The bridge handler therefore also forces
// startString / endString empty so no delimiter logic ever runs.
//
// REGEX-OPTIONS CONVERGENCE
// ─────────────────────────
//   - `regexOptions`: Okapi's RegexFilter.Parameters.reset() defaults
//     regexOptions to 40 (Pattern.DOTALL | Pattern.MULTILINE), which it
//     applies as GLOBAL flags when compiling each rule. The native Go
//     reader compiles each pattern verbatim via regexp.Compile with NO
//     global flags — RE2 semantics where `.` does not match newlines and
//     `^`/`$` are not multiline unless the pattern itself uses inline
//     `(?s)` / `(?m)`. The spec patterns carry those inline flags where
//     needed (e.g. INI's `(?m)`), so to converge we force the bridge to
//     regexOptions=0 and let the inline flags drive — identical to
//     native. Without this, e.g. id_and_text's `(.+)` would match across
//     newlines on the bridge (DOTALL) but stop at newline natively.
//
// ESCAPE MODES (escapeType): NOT CONVERGED — bridge-gap
// ─────────────────────────────────────────────────────
// neokapi's escapeType discriminator decodes escapes (`\"`→`"`,
// `\n`→LF, `""`→`"`) inside the *extracted source text*. Okapi's
// RegexFilter applies useBSlashEscape / useDoubleCharEscape only inside
// its STRING-type delimiter-scanning loop (and, empirically against
// 1.48.0, only on the write/merge path — the extracted source TextUnit
// retains the raw escapes). Under CONTENT-type extraction Okapi performs
// no escape decoding at all. The translator still emits the escape
// booleans for completeness, but the two escape examples
// (backslash_escape, double_char_escape) remain expected_fail: this is a
// genuine extraction-layer semantic difference, not a transport gap.
//
// Native config receives the original neokapi-keyed map untouched; only
// the bridge dispatch goes through this translator. spec.yaml stays
// monolingual in neokapi terms.
//
// The translator never mutates its input; it returns a fresh map.
func regexBridgeConfig(cfg map[string]any) (map[string]any, error) {
	out := map[string]any{
		// Force RE2-equivalent regex semantics on the bridge: no global
		// DOTALL/MULTILINE. Inline (?s)/(?m) in the patterns drive
		// behaviour, exactly as native regexp.Compile does.
		"regexOptions": 0,
		// Empty start/end delimiters so Okapi never runs its STRING-type
		// delimiter scan/strip — CONTENT-type rules emit the source group
		// verbatim, matching the native reader's inner-capture semantics.
		"startString": "",
		"endString":   "",
	}

	escapeType := EscapeNone
	for key, val := range cfg {
		switch key {
		case "rules":
			rules, err := parseRegexRules(val)
			if err != nil {
				return nil, fmt.Errorf("regexBridgeConfig: rules: %w", err)
			}
			blob, err := json.Marshal(rules)
			if err != nil {
				return nil, fmt.Errorf("regexBridgeConfig: marshal rules: %w", err)
			}
			out["regexRulesJson"] = string(blob)

		case "escapeType":
			s, ok := val.(string)
			if !ok {
				return nil, fmt.Errorf("regexBridgeConfig: escapeType: expected string, got %T", val)
			}
			escapeType = s

		case "escapeChar":
			// neokapi's escapeChar selects which doubled character is
			// collapsed under doublechar mode. Okapi's
			// useDoubleCharEscape is hard-wired to the rule's quoting
			// character (the doubled string delimiter), so there is no
			// separate bridge knob. The spec only ever uses the default
			// '"', which is also Okapi's, so this is a no-op for the
			// covered examples. Accept and ignore the key.
			if _, ok := val.(string); !ok {
				return nil, fmt.Errorf("regexBridgeConfig: escapeChar: expected string, got %T", val)
			}

		default:
			return nil, fmt.Errorf("regexBridgeConfig: unknown spec key %q", key)
		}
	}

	switch escapeType {
	case "", EscapeNone:
		out["useBSlashEscape"] = false
		out["useDoubleCharEscape"] = false
	case EscapeBackslash:
		out["useBSlashEscape"] = true
		out["useDoubleCharEscape"] = false
	case EscapeDoubleChar:
		out["useBSlashEscape"] = false
		out["useDoubleCharEscape"] = true
	default:
		return nil, fmt.Errorf("regexBridgeConfig: unknown escapeType %q", escapeType)
	}

	return out, nil
}

// EscapeNone / EscapeBackslash / EscapeDoubleChar mirror the native
// regex.Config discriminator values without importing the framework
// package (the parity test tree builds under the `parity` tag and keeps
// its translators self-contained).
const (
	EscapeNone       = "none"
	EscapeBackslash  = "backslash"
	EscapeDoubleChar = "doublechar"
)

// ruleTypeContent is Okapi's RULETYPE_CONTENT
// (net.sf.okapi.filters.regex.Rule.RULETYPE_CONTENT). CONTENT emits the
// source group verbatim with no delimiter scanning — the behaviour that
// matches the native reader. (STRING=0 would require the source group to
// capture the surrounding quote delimiters; see the package doc.)
const ruleTypeContent = 1

// okapiRegexRule is the JSON shape carried under the regexRulesJson
// reserved bridge parameter. Field names match the keys the bridge
// handler reads, which in turn map onto net.sf.okapi.filters.regex.Rule
// setters.
type okapiRegexRule struct {
	Expr        string `json:"expr"`
	RuleType    int    `json:"ruleType"`
	SourceGroup int    `json:"sourceGroup"`
	NameGroup   int    `json:"nameGroup"`
	NoteGroup   int    `json:"noteGroup"`
}

// parseRegexRules converts the spec's neokapi-keyed `rules:` list into
// the Okapi-shaped JSON rules. The YAML decoder hands us []any (inline
// list of map[string]any) or []map[string]any (anchor-merged); accept
// both. neokapi's 0 "no group" sentinel for idGroup/noteGroup becomes
// Okapi's -1.
func parseRegexRules(v any) ([]okapiRegexRule, error) {
	var raw []any
	switch x := v.(type) {
	case []any:
		raw = x
	case []map[string]any:
		raw = make([]any, len(x))
		for i, m := range x {
			raw[i] = m
		}
	default:
		return nil, fmt.Errorf("expected list, got %T", v)
	}

	out := make([]okapiRegexRule, 0, len(raw))
	for i, elem := range raw {
		m, ok := elem.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("rule %d: expected map, got %T", i, elem)
		}
		r := okapiRegexRule{
			RuleType:  ruleTypeContent, // RULETYPE_CONTENT
			NameGroup: -1,              // Okapi sentinel for "no group"
			NoteGroup: -1,
		}

		pat, ok := m["pattern"].(string)
		if !ok || pat == "" {
			return nil, fmt.Errorf("rule %d: pattern is required and must be a non-empty string", i)
		}
		r.Expr = pat

		src, err := asRuleInt(m["sourceGroup"], "sourceGroup")
		if err != nil {
			return nil, fmt.Errorf("rule %d: %w", i, err)
		}
		if src < 1 {
			return nil, fmt.Errorf("rule %d: sourceGroup must be >= 1, got %d", i, src)
		}
		r.SourceGroup = src

		if _, present := m["idGroup"]; present {
			id, err := asRuleInt(m["idGroup"], "idGroup")
			if err != nil {
				return nil, fmt.Errorf("rule %d: %w", i, err)
			}
			if id > 0 {
				r.NameGroup = id
			}
		}

		if _, present := m["noteGroup"]; present {
			note, err := asRuleInt(m["noteGroup"], "noteGroup")
			if err != nil {
				return nil, fmt.Errorf("rule %d: %w", i, err)
			}
			if note > 0 {
				r.NoteGroup = note
			}
		}

		out = append(out, r)
	}
	return out, nil
}

// asRuleInt accepts the YAML decoder's int / int64 / float64 shapes.
func asRuleInt(v any, label string) (int, error) {
	switch x := v.(type) {
	case int:
		return x, nil
	case int64:
		return int(x), nil
	case float64:
		return int(x), nil
	default:
		return 0, fmt.Errorf("%s: expected int, got %T", label, v)
	}
}
