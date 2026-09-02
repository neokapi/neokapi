package host

import (
	"reflect"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/ai/prompt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// promptContextAnswers maps each field of prompt.Context to the field of the
// review model that carries it.
//
// The invariant: a reviewer sees at least what the model was told. A field
// added to the translate prompt is a field the reviewer is owed, so adding one
// fails this test until the review model has said where it lands. Deleting an
// entry to make the test pass is the failure it exists to catch.
var promptContextAnswers = map[string]string{
	"Key":    "Neighbourhood.Key",
	"Before": "Neighbourhood.Before",
	"After":  "Neighbourhood.After",
	"Prior":  "History.Prior",
}

func TestReviewContextAnswersPromptContext(t *testing.T) {
	promptType := reflect.TypeFor[prompt.Context]()
	modelType := reflect.TypeFor[ReviewContext]()

	t.Run("every prompt field is answered", func(t *testing.T) {
		for f := range promptType.Fields() {
			answer, ok := promptContextAnswers[f.Name]
			require.Truef(t, ok,
				"prompt.Context.%s reaches the model but not the reviewer: add it to promptContextAnswers "+
					"and carry it on host.ReviewContext", f.Name)
			assert.NotEmptyf(t, answer, "prompt.Context.%s is mapped to nothing", f.Name)
		}
	})

	t.Run("every answer names a field the model has", func(t *testing.T) {
		for promptField, path := range promptContextAnswers {
			assert.Truef(t, reviewModelHasField(modelType, path),
				"promptContextAnswers[%q] names %q, which host.ReviewContext does not have", promptField, path)
		}
	})

	t.Run("every answer names a prompt field that exists", func(t *testing.T) {
		for promptField := range promptContextAnswers {
			_, ok := promptType.FieldByName(promptField)
			assert.Truef(t, ok, "promptContextAnswers names prompt.Context.%s, which no longer exists", promptField)
		}
	})
}

// reviewModelHasField walks a dotted field path through the model's types,
// following pointers and slices so "History.Prior" and "Neighbourhood.Before"
// both resolve.
func reviewModelHasField(t reflect.Type, path string) bool {
	for name := range strings.SplitSeq(path, ".") {
		for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice {
			t = t.Elem()
		}
		if t.Kind() != reflect.Struct {
			return false
		}
		f, ok := t.FieldByName(name)
		if !ok {
			return false
		}
		t = f.Type
	}
	return true
}
