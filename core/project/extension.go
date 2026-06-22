package project

import (
	"fmt"
	"sync"

	"gopkg.in/yaml.v3"
)

// ExtensionDecoder validates one Extras key's value at load time. The
// decoder typically unmarshals the node into a typed schema and returns
// any validation error.
type ExtensionDecoder interface {
	Decode(node yaml.Node) error
}

// ExtensionDecoderFunc adapts a plain function to ExtensionDecoder.
type ExtensionDecoderFunc func(node yaml.Node) error

// Decode implements ExtensionDecoder.
func (f ExtensionDecoderFunc) Decode(node yaml.Node) error { return f(node) }

// Scope identifies which Extras map an extension applies to.
type Scope int

const (
	// ScopeProject is the top-level Extras map on KapiProject.
	ScopeProject Scope = iota
	// ScopeDefaults is the Extras map on Defaults.
	ScopeDefaults
	// ScopeCollection is the Extras map on a ContentCollection.
	ScopeCollection
	// ScopeItem is the Extras map on a ContentItem.
	ScopeItem
)

// String returns a human-readable name for the scope.
func (s Scope) String() string {
	switch s {
	case ScopeProject:
		return "project"
	case ScopeDefaults:
		return "defaults"
	case ScopeCollection:
		return "collection"
	case ScopeItem:
		return "item"
	default:
		return fmt.Sprintf("scope(%d)", int(s))
	}
}

// Extension declares one (Scope, Name) → Decoder binding registered with
// the framework. Platform layers (bowrain, future others) call
// RegisterExtension or RegisterExtensionGroup at init() to teach the
// framework how to validate the YAML keys they own.
type Extension struct {
	// Name is the YAML key this extension matches at the given Scope.
	Name string
	// Scope identifies which Extras map this binding applies to.
	Scope Scope
	// Group is a logical name shared by related extensions (e.g.
	// "bowrain"). A recipe's `requires:` list is matched against
	// Groups, not individual Names.
	Group string
	// Decoder validates the YAML value. Optional — when nil, the
	// extension is registered as a marker only (so HasExtensionGroup
	// reports the Group present) without doing schema validation.
	Decoder ExtensionDecoder
}

var (
	extMu      sync.RWMutex
	extensions = map[extKey]Extension{}
	extGroups  = map[string]struct{}{}
)

type extKey struct {
	scope Scope
	name  string
}

// RegisterExtension records one (Scope, Name) → Decoder binding. Safe to
// call from package init(). Re-registering the same (Scope, Name) panics
// — it almost always indicates competing init() functions registering the
// same key.
func RegisterExtension(ext Extension) {
	if ext.Name == "" {
		panic("project: RegisterExtension: empty Name")
	}
	k := extKey{ext.Scope, ext.Name}
	extMu.Lock()
	defer extMu.Unlock()
	if _, exists := extensions[k]; exists {
		panic(fmt.Sprintf("project: RegisterExtension: duplicate registration for %s/%s", ext.Scope, ext.Name))
	}
	extensions[k] = ext
	if ext.Group != "" {
		extGroups[ext.Group] = struct{}{}
	}
}

// RegisterExtensionGroup is sugar for registering many extensions under
// the same group: it stamps Group on each Extension and registers them in
// order.
func RegisterExtensionGroup(group string, exts []Extension) {
	for _, ext := range exts {
		ext.Group = group
		RegisterExtension(ext)
	}
}

// ExtensionRegistered reports whether an extension is already registered
// for (scope, name) and, if so, the Group that claimed it. Callers that
// discover the same extension from more than one source — e.g. a binary
// that both compiles in a platform's schema package (blank import) and
// then rediscovers the same plugin through its manifest — use this to skip
// an idempotent re-registration instead of tripping RegisterExtension's
// duplicate-registration panic.
func ExtensionRegistered(scope Scope, name string) (group string, ok bool) {
	e, found := extensionFor(scope, name)
	if !found {
		return "", false
	}
	return e.Group, true
}

// HasExtensionGroup reports whether at least one Extension with this
// Group has been registered. Used by KapiProject.Validate to enforce
// recipe-level `requires:` declarations.
func HasExtensionGroup(group string) bool {
	extMu.RLock()
	defer extMu.RUnlock()
	_, ok := extGroups[group]
	return ok
}

// ResetExtensionsForTest clears the registry. Tests that register
// extensions should call this in setup so they start from a clean slate.
// Never called from production code.
func ResetExtensionsForTest() {
	extMu.Lock()
	defer extMu.Unlock()
	extensions = map[extKey]Extension{}
	extGroups = map[string]struct{}{}
}

// extensionFor returns the registered extension for (scope, name), or
// the zero Extension and false when none is registered.
func extensionFor(scope Scope, name string) (Extension, bool) {
	extMu.RLock()
	defer extMu.RUnlock()
	e, ok := extensions[extKey{scope, name}]
	return e, ok
}

// validateExtras walks one Extras map and runs any registered decoder
// against each key. Unknown keys (no registered decoder) are preserved
// verbatim — the framework stays forward-compatible with extensions the
// loading binary doesn't know about. A recipe that wants to reject
// unknown extensions should declare `requires: [...]`.
func validateExtras(scope Scope, prefix string, extras map[string]yaml.Node) error {
	for key, node := range extras {
		ext, ok := extensionFor(scope, key)
		if !ok {
			continue
		}
		if ext.Decoder == nil {
			continue
		}
		if err := ext.Decoder.Decode(node); err != nil {
			return fmt.Errorf("%s%s: %w", prefix, key, err)
		}
	}
	return nil
}
