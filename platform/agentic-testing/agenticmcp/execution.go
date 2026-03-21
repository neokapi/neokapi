package agenticmcp

// AgenticEventType identifies the kind of agentic event.
type AgenticEventType string

const (
	EventExecStarted   AgenticEventType = "exec.started"
	EventExecCompleted AgenticEventType = "exec.completed"
	EventExecFailed    AgenticEventType = "exec.failed"
	EventExecProgress  AgenticEventType = "exec.progress"
	EventExecToolCall  AgenticEventType = "exec.tool_call"
	EventObservation   AgenticEventType = "agent.observation"
	EventSuggestion    AgenticEventType = "agent.suggestion"
)

// AgenticEvent is the common envelope for all agentic testing events.
type AgenticEvent struct {
	Type        AgenticEventType `json:"type"`
	ExecutionID string           `json:"execution_id"`
	Workspace   string           `json:"workspace"`
	Agent       string           `json:"agent"`
	Role        string           `json:"role"`
	Timestamp   string           `json:"timestamp"`
	Data        map[string]any   `json:"data,omitempty"`
}
