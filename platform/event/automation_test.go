package event

import (
	"sync"
	"testing"
	"time"

	platev "github.com/neokapi/neokapi/platform/event"
	"github.com/stretchr/testify/assert"
)

func TestAutomationRuleMatching(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()

	var executed []string
	var mu sync.Mutex

	engine := NewAutomationEngine(bus, func(action AutomationAction, event platev.Event) error {
		mu.Lock()
		executed = append(executed, action.Type)
		mu.Unlock()
		return nil
	})
	defer engine.Close()

	engine.AddRule(AutomationRule{
		Name:      "auto-translate",
		EventType: platev.EventBlockCreated,
		Actions:   []AutomationAction{{Type: "flow", Config: map[string]string{"flow": "translate"}}},
	})

	bus.Publish(platev.Event{Type: platev.EventBlockCreated})
	bus.Publish(platev.Event{Type: platev.EventBlockUpdated}) // Should not trigger

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	assert.Len(t, executed, 1)
	assert.Equal(t, "flow", executed[0])
	mu.Unlock()
}

func TestAutomationConditionEvaluation(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()

	var triggered int
	var mu sync.Mutex

	engine := NewAutomationEngine(bus, func(action AutomationAction, event platev.Event) error {
		mu.Lock()
		triggered++
		mu.Unlock()
		return nil
	})
	defer engine.Close()

	engine.AddRule(AutomationRule{
		Name:      "priority-only",
		EventType: platev.EventBlockUpdated,
		Conditions: []AutomationCondition{
			{Field: "priority", Operator: "equals", Value: "high"},
		},
		Actions: []AutomationAction{{Type: "notify"}},
	})

	bus.Publish(platev.Event{Type: platev.EventBlockUpdated, Data: map[string]string{"priority": "low"}})
	bus.Publish(platev.Event{Type: platev.EventBlockUpdated, Data: map[string]string{"priority": "high"}})

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	assert.Equal(t, 1, triggered)
	mu.Unlock()
}

func TestAutomationLoopPrevention(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()

	var count int
	var mu sync.Mutex

	engine := NewAutomationEngine(bus, func(action AutomationAction, event platev.Event) error {
		mu.Lock()
		count++
		mu.Unlock()
		// Simulate re-emitting an event (which would loop without prevention).
		bus.Publish(platev.Event{
			Type:        platev.EventBlockUpdated,
			CausationID: NextCausationID(event),
		})
		return nil
	})
	defer engine.Close()
	engine.SetMaxChainDepth(3)

	engine.AddRule(AutomationRule{
		Name:      "loopy",
		EventType: platev.EventBlockUpdated,
		Actions:   []AutomationAction{{Type: "flow"}},
	})

	bus.Publish(platev.Event{Type: platev.EventBlockUpdated})
	time.Sleep(500 * time.Millisecond)

	mu.Lock()
	assert.LessOrEqual(t, count, 3, "loop should be broken at max chain depth")
	mu.Unlock()
}

func TestAutomationPause(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()

	var count int
	var mu sync.Mutex

	engine := NewAutomationEngine(bus, func(action AutomationAction, event platev.Event) error {
		mu.Lock()
		count++
		mu.Unlock()
		return nil
	})
	defer engine.Close()

	engine.AddRule(AutomationRule{
		Name:      "test",
		EventType: platev.EventBlockCreated,
		Actions:   []AutomationAction{{Type: "flow"}},
	})

	engine.Pause()
	bus.Publish(platev.Event{Type: platev.EventBlockCreated})
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	assert.Equal(t, 0, count)
	mu.Unlock()

	engine.Resume()
	bus.Publish(platev.Event{Type: platev.EventBlockCreated})
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	assert.Equal(t, 1, count)
	mu.Unlock()
}

func TestBrandVoiceEventTypes(t *testing.T) {
	tests := []struct {
		eventType platev.EventType
		value     string
	}{
		{platev.EventBrandVoiceCheckStarted, "brand.voice.check.started"},
		{platev.EventBrandVoiceCheckCompleted, "brand.voice.check.completed"},
		{platev.EventBrandVoiceGateFailed, "brand.voice.gate.failed"},
		{platev.EventBrandVoiceGatePassed, "brand.voice.gate.passed"},
		{platev.EventBrandVoiceDrift, "brand.voice.drift"},
		{platev.EventBrandVoiceCorrected, "brand.voice.corrected"},
		{platev.EventBrandProfileUpdated, "brand.profile.updated"},
	}
	for _, tt := range tests {
		assert.Equal(t, platev.EventType(tt.value), tt.eventType)
	}
}

func TestIsBrandVoiceEvent(t *testing.T) {
	tests := []struct {
		eventType platev.EventType
		want      bool
	}{
		{platev.EventBrandVoiceCheckStarted, true},
		{platev.EventBrandVoiceCheckCompleted, true},
		{platev.EventBrandVoiceGateFailed, true},
		{platev.EventBrandVoiceGatePassed, true},
		{platev.EventBrandVoiceDrift, true},
		{platev.EventBrandVoiceCorrected, true},
		{platev.EventBrandProfileUpdated, true},
		{platev.EventBlockCreated, false},
		{platev.EventFlowStarted, false},
		{platev.EventQualityGatePass, false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, IsBrandVoiceEvent(tt.eventType), "IsBrandVoiceEvent(%q)", tt.eventType)
	}
}

func TestAutomationBrandVoiceRule(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()

	var executed []platev.EventType
	var mu sync.Mutex

	engine := NewAutomationEngine(bus, func(action AutomationAction, event platev.Event) error {
		mu.Lock()
		executed = append(executed, event.Type)
		mu.Unlock()
		return nil
	})
	defer engine.Close()

	engine.AddRule(AutomationRule{
		Name:      "brand-voice-gate",
		EventType: platev.EventBrandVoiceGateFailed,
		Actions:   []AutomationAction{{Type: "notify", Config: map[string]string{"channel": "brand-alerts"}}},
	})

	bus.Publish(platev.Event{Type: platev.EventBrandVoiceCheckStarted}) // Should not trigger
	bus.Publish(platev.Event{Type: platev.EventBrandVoiceGateFailed})   // Should trigger
	bus.Publish(platev.Event{Type: platev.EventBrandVoiceGatePassed})   // Should not trigger

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	assert.Len(t, executed, 1)
	assert.Equal(t, platev.EventBrandVoiceGateFailed, executed[0])
	mu.Unlock()
}

// Leader gating test removed — AD-036 replaces leader election with
// distributed event bus (consumer groups handle exactly-once delivery).
