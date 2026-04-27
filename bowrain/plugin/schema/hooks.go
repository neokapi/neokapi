package schema

import "fmt"

// Hook trigger names. Hooks run synchronously around lifecycle operations.
const (
	HookPrePush  = "pre-push"
	HookPostPush = "post-push"
	HookPrePull  = "pre-pull"
	HookPostPull = "post-pull"
	HookPreFlow  = "pre-flow"
	HookPostFlow = "post-flow"
)

// Automation action types.
const (
	ActionRunFlow       = "run_flow"
	ActionWaitTranslate = "wait_translate"
	ActionPull          = "pull"
	ActionPush          = "push"
)

// HooksSpec maps lifecycle trigger names to a list of flow names that should
// run when the trigger fires. Hooks complement server-side automation by
// providing a synchronous local hook point.
type HooksSpec map[string][]string

// Validate checks that all triggers and flow names in the hooks block are
// well-formed.
func (h HooksSpec) Validate() error {
	for trigger, flows := range h {
		if err := ValidateHookTrigger(trigger); err != nil {
			return err
		}
		for i, flowName := range flows {
			if flowName == "" {
				return fmt.Errorf("hooks[%s][%d]: flow name is required", trigger, i)
			}
		}
	}
	return nil
}

// AutomationSpec defines a single local automation rule. Automations group
// one or more actions under a trigger and may be enabled/disabled.
type AutomationSpec struct {
	Name    string         `yaml:"name" json:"name"`
	Trigger string         `yaml:"trigger" json:"trigger"`
	Actions []ActionConfig `yaml:"actions" json:"actions"`
	Enabled *bool          `yaml:"enabled,omitempty" json:"enabled,omitempty"`
}

// IsEnabled reports whether the automation is enabled. Defaults to true
// when the field is unset.
func (a AutomationSpec) IsEnabled() bool {
	return a.Enabled == nil || *a.Enabled
}

// Validate checks that this automation rule is well-formed.
func (a AutomationSpec) Validate() error {
	if a.Name == "" {
		return fmt.Errorf("name is required")
	}
	if err := ValidateHookTrigger(a.Trigger); err != nil {
		return fmt.Errorf("trigger: %w", err)
	}
	for j, action := range a.Actions {
		if err := ValidateActionType(action.Type); err != nil {
			return fmt.Errorf("actions[%d]: %w", j, err)
		}
	}
	return nil
}

// ActionConfig describes a single action in an automation rule.
type ActionConfig struct {
	Type   string            `yaml:"type" json:"type"`
	Config map[string]string `yaml:"config,omitempty" json:"config,omitempty"`
}

// ValidateHookTrigger checks that trigger is one of the known hook names.
func ValidateHookTrigger(trigger string) error {
	switch trigger {
	case HookPrePush, HookPostPush, HookPrePull, HookPostPull, HookPreFlow, HookPostFlow:
		return nil
	}
	return fmt.Errorf("unknown trigger %q (expected one of %q, %q, %q, %q, %q, %q)",
		trigger,
		HookPrePush, HookPostPush, HookPrePull, HookPostPull, HookPreFlow, HookPostFlow)
}

// ValidateActionType checks that action is one of the known action names.
func ValidateActionType(action string) error {
	switch action {
	case ActionRunFlow, ActionWaitTranslate, ActionPull, ActionPush:
		return nil
	}
	return fmt.Errorf("unknown action type %q (expected one of %q, %q, %q, %q)",
		action,
		ActionRunFlow, ActionWaitTranslate, ActionPull, ActionPush)
}
