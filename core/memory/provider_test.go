package memory_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	corememory "github.com/neokapi/neokapi/core/memory"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// What this package promises, asserted against the interface rather than
// against any implementation of it.
//
// The four interfaces this replaced were each individually reasonable and
// collectively unusable, and the properties below are the ones whose absence
// made them so. A test that only exercised one implementation would not have
// caught any of them.

// TestNullProviderAnswersEveryQuestion.
//
// The point of a null provider is that a caller needs no special case for "no
// corpus". If it answered some questions and not others, every caller would
// need to know which — which is the type-assertion problem in a different hat.
func TestNullProviderAnswersEveryQuestion(t *testing.T) {
	t.Parallel()

	var p corememory.Provider = corememory.NullProvider{}

	m, ok := p.Lookup(t.Context(), corememory.Request{Text: "anything", Source: "en", Target: "nb"})
	assert.False(t, ok)
	assert.Empty(t, m.TargetRuns)

	v, ok := p.PriorVersion(t.Context(), corememory.VersionRequest{
		Unit: "cta.start", Source: "en", Target: "nb", GovernedBy: "fp",
	})
	assert.False(t, ok)
	assert.Empty(t, v.Target)
}

// TestEveryMethodIsRequired: the interface has no optional half.
//
// Optionality used to be expressed by omitting a method and having callers type
// assert, which meant a capability could switch itself off silently — and a
// caller could not distinguish "this store keeps no version chains" from "this
// block has no history". Both are now the same answer, deliberately, because a
// caller should not behave differently on them.
func TestEveryMethodIsRequired(t *testing.T) {
	t.Parallel()

	var names []string
	for m := range reflect.TypeFor[corememory.Provider]().Methods() {
		names = append(names, m.Name)
	}

	assert.ElementsMatch(t, []string{"Lookup", "PriorVersion"}, names,
		"a new method is a new obligation for every implementation — which is the point, but say so deliberately")
}

// TestOneIdentityVocabulary is the property that made a unified interface
// possible at all.
//
// Before, LookupBlock took a whole block and called the location `at`, while
// PriorVersion took a unit and called it `point`. Two names for one idea across
// two interfaces is how a caller ends up passing the wrong one.
func TestOneIdentityVocabulary(t *testing.T) {
	t.Parallel()

	req := reflect.TypeFor[corememory.Request]()
	vreq := reflect.TypeFor[corememory.VersionRequest]()

	_, hasPoint := req.FieldByName("Point")
	assert.True(t, hasPoint, "a lookup names where it is happening")
	_, hasVersionPoint := vreq.FieldByName("Point")
	assert.True(t, hasVersionPoint, "and so does a version query, by the same name")

	for _, tp := range []reflect.Type{req, vreq} {
		for f := range tp.Fields() {
			assert.NotEqual(t, "At", f.Name, "%s: the location is Point everywhere", tp.Name())
			assert.NotEqual(t, "Fingerprint", f.Name,
				"%s: the fingerprint is GovernedBy, which says what it is for", tp.Name())
		}
	}
}

// TestRequestsAreStructs: every question is asked with a struct, so adding a
// field is additive rather than a break.
//
// This is not style. Adding the point to the exact lookup and the fingerprint
// to the version lookup each broke every implementation of the old interfaces,
// and the second time it happened the answer was to add another interface.
func TestRequestsAreStructs(t *testing.T) {
	t.Parallel()

	for m := range reflect.TypeFor[corememory.Provider]().Methods() {
		require.Equal(t, 2, m.Type.NumIn(), "%s takes a context and one request", m.Name)
		assert.Equal(t, "Context", m.Type.In(0).Name(), "%s takes the run's context", m.Name)
		assert.Equal(t, reflect.Struct, m.Type.In(1).Kind(),
			"%s asks with a struct, so a new field does not break implementations", m.Name)
	}
}

// recordingProvider answers nothing and remembers what it was asked.
type recordingProvider struct{ seen []corememory.Request }

func (p *recordingProvider) Lookup(_ context.Context, req corememory.Request) (corememory.Match, bool) {
	p.seen = append(p.seen, req)
	return corememory.Match{}, false
}

func (p *recordingProvider) PriorVersion(context.Context, corememory.VersionRequest) (corememory.Version, bool) {
	return corememory.Version{}, false
}

// TestAProviderNeedsNothingButTheTwoMethods: implementing the contract should
// take a type with two methods and no embedding, no framework type, and no
// knowledge of who is calling.
func TestAProviderNeedsNothingButTheTwoMethods(t *testing.T) {
	t.Parallel()

	var p corememory.Provider = &recordingProvider{}
	_, ok := p.Lookup(t.Context(), corememory.Request{
		Block:  model.NewBlock("b", "Get started"),
		Source: "en",
		Target: "nb",
		Point:  "acme\x1fweb\x1fsite",
	})
	assert.False(t, ok)

	rec := p.(*recordingProvider)
	require.Len(t, rec.seen, 1)
	assert.Equal(t, "acme\x1fweb\x1fsite", rec.seen[0].Point)
}

// TestConfigKeysAreDistinct: the provider and the source locale travel in the
// same config map and must not collide with each other or with anything a tool
// already reads.
func TestConfigKeysAreDistinct(t *testing.T) {
	t.Parallel()

	assert.NotEqual(t, corememory.ConfigKey, corememory.SourceLocaleKey)
	for _, k := range []string{corememory.ConfigKey, corememory.SourceLocaleKey} {
		assert.NotEmpty(t, k)
		assert.False(t, strings.ContainsAny(k, " \t"), "a config key is a plain identifier")
	}
}
