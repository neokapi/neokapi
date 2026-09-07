package event

import (
	"sync"
	"testing"
	"time"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEngineAnnotatesTheRuleIdentity proves the engine hands the executor the
// rule's id beside its name, which is what lets an execution record trace back
// to the rule after a rename.
func TestEngineAnnotatesTheRuleIdentity(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()

	var mu sync.Mutex
	var got []AutomationAction
	engine := NewAutomationEngine(bus, func(action AutomationAction, _ platev.Event) error {
		mu.Lock()
		got = append(got, action)
		mu.Unlock()
		return nil
	})
	defer engine.Close()

	engine.AddRule(AutomationRule{
		ID:        "rule-42",
		Name:      "translate-on-create",
		EventType: platev.EventBlockCreated,
		Actions:   []AutomationAction{{Type: "flow"}, {Type: "notify"}},
	})

	bus.Publish(platev.Event{Type: platev.EventBlockCreated})

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(got) == 2
	}, 2*time.Second, 10*time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	for _, action := range got {
		assert.Equal(t, "rule-42", action.RuleID, "every action carries the rule id")
		assert.Equal(t, "translate-on-create", action.Name)
	}
}

// TestBuiltInRuleID covers the id a platform rule carries: it has no stored
// row to be looked up in, so its name is encoded in the id under a prefix that
// cannot collide with a stored rule's.
func TestBuiltInRuleID(t *testing.T) {
	id := BuiltInRuleID("auto-extract-on-push")
	assert.Equal(t, "builtin:auto-extract-on-push", id)

	name, ok := BuiltInRuleName(id)
	assert.True(t, ok)
	assert.Equal(t, "auto-extract-on-push", name)

	name, ok = BuiltInRuleName("aB3xY9zQ")
	assert.False(t, ok, "a stored rule's id is not a built-in one")
	assert.Empty(t, name)
}
