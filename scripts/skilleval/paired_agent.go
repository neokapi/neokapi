package main

import "time"

// PairedAgentSpec fixes one host, model and reasoning setting for every condition.
type PairedAgentSpec struct {
	Host   string `json:"host"`
	Model  string `json:"model"`
	Effort string `json:"effort"`
}

// PairedLaunch contains one attempt's inputs. StateDir must be outside Workspace.
type PairedLaunch struct {
	Agent     PairedAgentSpec `json:"agent"`
	Condition string          `json:"condition"`
	Task      string          `json:"task,omitempty"`
	Workspace string          `json:"workspace"`
	StateDir  string          `json:"state_dir"`
	RepoRoot  string          `json:"repo_root"`
	// MainCheckout is the repository's main checkout when RepoRoot is a
	// worktree of it, "" otherwise. It holds the same answer key as the
	// worktree, and every other worktree of the repository.
	MainCheckout string `json:"main_checkout,omitempty"`
	KapiBin      string `json:"kapi_bin"`
	// CellKapi is KapiBin linked into the cell, which is what the agent's
	// commands and the MCP server run, so no path into the checkout reaches
	// the agent. Empty runs KapiBin itself.
	CellKapi string `json:"cell_kapi,omitempty"`
	// SkillSource is the folder the kapi skill is copied from: the study's
	// own copy of the shipped skill, taken when the study started.
	SkillSource string `json:"skill_source,omitempty"`
	// StudyDir is the study directory the cell lies in. The transcript audit
	// flags a path in it outside the cell.
	StudyDir string `json:"study_dir,omitempty"`
	// TmpDir is the cell's own temporary directory, the TMPDIR its host and
	// tools write to. It is short, because Claude Code puts sockets under it.
	TmpDir         string              `json:"tmp_dir,omitempty"`
	Prompt         string              `json:"-"`
	TranscriptPath string              `json:"transcript_path"`
	Timeout        time.Duration       `json:"timeout"`
	MaxTurns       int                 `json:"max_turns"`
	NoTools        bool                `json:"no_tools,omitempty"`
	Interference   *PairedInterference `json:"interference,omitempty"`
}

// PairedPrepared is an offline launch description. Blockers prohibit inference.
// Env is deliberately omitted from reports: account credentials stay private.
type PairedPrepared struct {
	MCPReadiness   *PairedMCPReadiness `json:"mcp_readiness,omitempty"`
	Surface        *PairedSurface      `json:"surface,omitempty"`
	Launch         PairedLaunch        `json:"launch"`
	Executable     string              `json:"executable"`
	Args           []string            `json:"args"`
	Env            []string            `json:"-"`
	Version        string              `json:"version"`
	AuthMode       string              `json:"auth_mode"`
	Blockers       []string            `json:"blockers"`
	IsolationNotes []string            `json:"isolation_notes"`
}

// PairedAgentResult reports observed execution, independently of content scoring.
// Token counts are usage observations, not subscription quota or dollar charges.
type PairedAgentResult struct {
	UsageObserved     bool     `json:"usage_observed"`
	ProtocolCompleted bool     `json:"protocol_completed"`
	Status            string   `json:"status"`
	Error             string   `json:"error,omitempty"`
	RequestedModel    string   `json:"requested_model"`
	ActualModel       string   `json:"actual_model,omitempty"`
	SessionID         string   `json:"session_id,omitempty"`
	DurationMS        int64    `json:"duration_ms"`
	InputTokens       int64    `json:"input_tokens"`
	CacheReadTokens   int64    `json:"cache_read_tokens"`
	CacheWriteTokens  int64    `json:"cache_write_tokens"`
	OutputTokens      int64    `json:"output_tokens"`
	Tools             []string `json:"tools"`
	RateLimited       bool     `json:"rate_limited"`
	// InfraFailure names the infrastructure failure that ended the session
	// (auth, overload, network), which says nothing about the agent.
	InfraFailure string `json:"infra_failure,omitempty"`
	QuotaStatus  string `json:"quota_status"`
	FinalText    string `json:"final_text,omitempty"`
	// Turns is the host's own count of model turns, where it reports one
	// (Claude's num_turns). Codex reports none.
	Turns *int64 `json:"turns,omitempty"`
	// ToolCalls counts the tool calls the host completed.
	ToolCalls int `json:"tool_calls"`
	// Refusals counts the refusal codes tool results carried: kapi's own
	// (stale, gate_failed, guard, …) and, prefixed host:, a host tool's
	// refusal of a stale write.
	Refusals map[string]int `json:"refusals,omitempty"`
	// OverrideAttempts lists each attempt to land an edit over a check: a gate
	// report, a person's actor claimed from the agent's shell, a blind write.
	OverrideAttempts []string `json:"override_attempts,omitempty"`
	// WriteRoutes lists the routes the agent's tool calls took to write the
	// task's files, in the order it first took each: contract (kapi apply,
	// ksed -i, apply_edits), merge (kapi merge) and native (the host's own
	// edit, write or patch tools, or a shell command rewriting a task file).
	WriteRoutes []string `json:"write_routes,omitempty"`
	// RouteAttempts lists kapi names the agent tried that its cell's PATH does
	// not hold. The cell cannot run them; they are recorded, not refused.
	RouteAttempts []string `json:"route_attempts,omitempty"`
	// OutsideCell lists the paths tool calls named outside the cell, each
	// prefixed with where it lies: study (another attempt or the study's
	// records), checkout, home or temp. A reviewer reads such an attempt's
	// transcript before counting it.
	OutsideCell []string `json:"outside_cell,omitempty"`
	// Interference records the other editor's change, for a task that has one.
	Interference *PairedInterferenceRecord `json:"interference,omitempty"`
}

// PairedInterferenceRecord says whether and when the other editor's change
// landed.
type PairedInterferenceRecord struct {
	Triggered bool   `json:"triggered"`
	Trigger   string `json:"trigger,omitempty"`
	AfterMS   int64  `json:"after_ms,omitempty"`
	// AgentWroteFirst reports that the file already held the agent's write when
	// the change landed, so no stale read was exercised.
	AgentWroteFirst bool   `json:"agent_wrote_first,omitempty"`
	Applied         bool   `json:"applied"`
	Error           string `json:"error,omitempty"`
}
