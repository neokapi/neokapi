package event

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
)

// MaxChainDepth is the default maximum causation chain depth before
// automation stops to prevent infinite loops.
const MaxChainDepth = 5

// builtInRulePrefix marks the id of a rule the platform ships rather than one
// a user authored. A built-in rule has no row in automation_rules, so its id
// is derived from its name; the prefix keeps it from colliding with a stored
// rule's id and tells a reader which kind of rule an execution record names.
const builtInRulePrefix = "builtin:"

// BuiltInRuleID is the execution-record id of a platform rule named name.
func BuiltInRuleID(name string) string { return builtInRulePrefix + name }

// BuiltInRuleName returns the rule name behind a built-in rule id. The second
// result is false for an id that names a stored rule, and the name is empty
// then, so a caller that ignores it cannot mistake the id for a name.
func BuiltInRuleName(ruleID string) (string, bool) {
	name, ok := strings.CutPrefix(ruleID, builtInRulePrefix)
	if !ok {
		return "", false
	}
	return name, true
}

// AutomationAction defines what happens when a rule triggers.
type AutomationAction struct {
	Type   string            `json:"type"`             // "flow", "webhook", "notify"
	Config map[string]string `json:"config,omitempty"` // Action-specific configuration
	Name   string            `json:"-"`                // Rule name (set at runtime by engine, not persisted)
	// RuleID identifies the rule this action was dispatched from, so the
	// execution record traces back to it after the rule is renamed or another
	// rule takes the same name. Set at runtime by the engine beside Name, and
	// not persisted: an action stored on a rule already sits inside it.
	RuleID string `json:"-"`
}

// AutomationCondition defines when a rule should trigger.
type AutomationCondition struct {
	Field    string // Event data field to check
	Operator string // "equals", "contains", "exists"
	Value    string // Expected value
}

// AutomationRule defines an event-triggered automation.
type AutomationRule struct {
	// ID identifies the rule in the execution record: the stored rule's row
	// id for a rule a user authored, and BuiltInRuleID(name) for one of the
	// platform's own, which have no row.
	ID        string
	Name      string
	EventType platev.EventType
	// ProjectID scopes the rule to one project's events. Empty matches every
	// project, which is what the platform's built-in rules want; a rule a
	// user authored on a project carries that project so it never fires on
	// another project's events.
	ProjectID  string
	Conditions []AutomationCondition
	Actions    []AutomationAction
}

// ActionExecutor is called when an automation rule fires.
type ActionExecutor func(action AutomationAction, event platev.Event) error

// AutomationEngine subscribes to events and evaluates automation rules.
type AutomationEngine struct {
	bus           platev.EventBus
	rules         []AutomationRule
	executor      ActionExecutor
	maxChainDepth int
	paused        atomic.Bool
	sub           *platev.Subscription
	mu            sync.RWMutex
}

// NewAutomationEngine creates an automation engine.
func NewAutomationEngine(bus platev.EventBus, executor ActionExecutor) *AutomationEngine {
	e := &AutomationEngine{
		bus:           bus,
		executor:      executor,
		maxChainDepth: MaxChainDepth,
	}
	e.sub = bus.SubscribeGroup("automations", e.handleEvent)
	return e
}

// AddRule registers an automation rule.
func (e *AutomationEngine) AddRule(rule AutomationRule) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rules = append(e.rules, rule)
}

// ReplaceRules atomically replaces all automation rules.
func (e *AutomationEngine) ReplaceRules(rules []AutomationRule) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rules = make([]AutomationRule, len(rules))
	copy(e.rules, rules)
}

// Rules returns a copy of the current rules.
func (e *AutomationEngine) Rules() []AutomationRule {
	e.mu.RLock()
	defer e.mu.RUnlock()
	rules := make([]AutomationRule, len(e.rules))
	copy(rules, e.rules)
	return rules
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

// handleEvent runs every rule the event matches. It always acknowledges.
//
// An executor failure is the executor's to record — it owns the run row that
// says what was attempted and how it ended. Redelivering the event would start
// the matched actions again from the top, including the ones that succeeded, so
// a webhook that half-fired would fire again.
func (e *AutomationEngine) handleEvent(event platev.Event) error {
	if e.paused.Load() {
		return nil
	}

	// Check causation chain depth.
	if depth := chainDepth(event.CausationID); depth >= e.maxChainDepth {
		return nil // Prevent infinite loops.
	}

	e.mu.RLock()
	rules := make([]AutomationRule, len(e.rules))
	copy(rules, e.rules)
	e.mu.RUnlock()

	for _, rule := range rules {
		if rule.EventType != "" && rule.EventType != event.Type {
			continue
		}
		if rule.ProjectID != "" && rule.ProjectID != event.ProjectID {
			continue
		}
		if !matchConditions(rule.Conditions, event) {
			continue
		}
		for _, action := range rule.Actions {
			// Annotate with the rule's identity for run tracking.
			action.Name = rule.Name
			action.RuleID = rule.ID
			if e.executor != nil {
				_ = e.executor(action, event)
			}
		}
	}
	return nil
}

func matchConditions(conditions []AutomationCondition, event platev.Event) bool {
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

// IsVoiceEvent reports whether an event type belongs to the voice family.
//
// Prefix, not a list, so an event added beside the others is routed without
// this function being edited. It matches the CURRENT spelling only: a stored
// event from before the rename keeps `brand.` in the audit log, and it is a
// record of what happened rather than something still being routed.
func IsVoiceEvent(t platev.EventType) bool {
	return strings.HasPrefix(string(t), "voice.")
}

// NextCausationID increments the causation chain.
func NextCausationID(event platev.Event) string {
	depth := chainDepth(event.CausationID)
	return fmt.Sprintf("%s:%d", event.ID, depth+1)
}
