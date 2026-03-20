package jobs

// AgentJobMessage is the payload sent via Service Bus for @bravo agent processing.
// The worker receives this, loads the user message from the agent store,
// spawns/reuses a Container App, streams the response, and publishes SSE
// events to Redis pub/sub.
type AgentJobMessage struct {
	ConversationID string `json:"conversation_id"`
	MessageID      string `json:"message_id"`
	WorkspaceID    string `json:"workspace_id"`
	UserID         string `json:"user_id"`
	WorkspaceRole  string `json:"workspace_role"`
	Content        string `json:"content"`        // user message text (denormalized for convenience)
	Mode           string `json:"mode,omitempty"` // "ask", "coworker", "bravo"
}
