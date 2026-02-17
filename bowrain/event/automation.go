package event

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// MaxChainDepth is the default maximum causation chain depth before
// automation stops to prevent infinite loops.
const MaxChainDepth = 5

// AutomationAction defines what happens when a rule triggers.
type AutomationAction struct {
	Type   string            // "flow", "webhook", "notify"
	Config map[string]string // Action-specific configuration
}

// AutomationCondition defines when a rule should trigger.
type AutomationCondition struct {
	Field    string // Event data field to check
	Operator string // "equals", "contains", "exists"
	Value    string // Expected value
}

// AutomationRule defines an event-triggered automation.
type AutomationRule struct {
	Name       string
	EventType  EventType
	Conditions []AutomationCondition
	Actions    []AutomationAction
}

// ActionExecutor is called when an automation rule fires.
type ActionExecutor func(action AutomationAction, event Event) error

// AutomationEngine subscribes to events and evaluates automation rules.
type AutomationEngine struct {
	bus           EventBus
	rules         []AutomationRule
	executor      ActionExecutor
	maxChainDepth int
	paused        atomic.Bool
	sub           *Subscription
	mu            sync.RWMutex
}

// NewAutomationEngine creates an automation engine.
func NewAutomationEngine(bus EventBus, executor ActionExecutor) *AutomationEngine {
	e := &AutomationEngine{
		bus:           bus,
		executor:      executor,
		maxChainDepth: MaxChainDepth,
	}
	e.sub = bus.SubscribeAll(e.handleEvent)
	return e
}

// AddRule registers an automation rule.
func (e *AutomationEngine) AddRule(rule AutomationRule) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rules = append(e.rules, rule)
}

// SetMaxChainDepth sets the maximum causation chain depth.
func (e *AutomationEngine) SetMaxChainDepth(depth int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.maxChainDepth = depth
}

// Pause temporarily stops automation processing.
func (e *AutomationEngine) Pause() { e.paused.Store(true) }

// Resume restarts automation processing.
func (e *AutomationEngine) Resume() { e.paused.Store(false) }

// Close unsubscribes from the event bus.
func (e *AutomationEngine) Close() {
	if e.sub != nil {
		e.bus.Unsubscribe(e.sub)
	}
}

func (e *AutomationEngine) handleEvent(event Event) {
	if e.paused.Load() {
		return
	}

	// Check causation chain depth.
	if depth := chainDepth(event.CausationID); depth >= e.maxChainDepth {
		return // Prevent infinite loops.
	}

	e.mu.RLock()
	rules := make([]AutomationRule, len(e.rules))
	copy(rules, e.rules)
	e.mu.RUnlock()

	for _, rule := range rules {
		if rule.EventType != "" && rule.EventType != event.Type {
			continue
		}
		if !matchConditions(rule.Conditions, event) {
			continue
		}
		for _, action := range rule.Actions {
			if e.executor != nil {
				_ = e.executor(action, event)
			}
		}
	}
}

func matchConditions(conditions []AutomationCondition, event Event) bool {
	for _, cond := range conditions {
		val, exists := event.Data[cond.Field]
		switch cond.Operator {
		case "equals":
			if val != cond.Value {
				return false
			}
		case "contains":
			if !strings.Contains(val, cond.Value) {
				return false
			}
		case "exists":
			if !exists {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// chainDepth extracts the depth from a causation ID chain (format: "event-id:depth").
func chainDepth(causationID string) int {
	if causationID == "" {
		return 0
	}
	parts := strings.Split(causationID, ":")
	if len(parts) < 2 {
		return 1
	}
	depth, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return 1
	}
	return depth
}

// NextCausationID increments the causation chain.
func NextCausationID(event Event) string {
	depth := chainDepth(event.CausationID)
	return fmt.Sprintf("%s:%d", event.ID, depth+1)
}
