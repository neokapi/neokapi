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
	Agent          PairedAgentSpec `json:"agent"`
	Condition      string          `json:"condition"`
	Workspace      string          `json:"workspace"`
	StateDir       string          `json:"state_dir"`
	RepoRoot       string          `json:"repo_root"`
	KapiBin        string          `json:"kapi_bin"`
	Prompt         string          `json:"-"`
	TranscriptPath string          `json:"transcript_path"`
	Timeout        time.Duration   `json:"timeout"`
	MaxTurns       int             `json:"max_turns"`
}

// PairedPrepared is an offline launch description. Blockers prohibit inference.
// Env is deliberately omitted from reports: account credentials stay private.
type PairedPrepared struct {
	Launch         PairedLaunch `json:"launch"`
	Executable     string       `json:"executable"`
	Args           []string     `json:"args"`
	Env            []string     `json:"-"`
	Version        string       `json:"version"`
	AuthMode       string       `json:"auth_mode"`
	Blockers       []string     `json:"blockers"`
	IsolationNotes []string     `json:"isolation_notes"`
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
	OutputTokens      int64    `json:"output_tokens"`
	Tools             []string `json:"tools"`
	RateLimited       bool     `json:"rate_limited"`
	QuotaStatus       string   `json:"quota_status"`
	FinalText         string   `json:"final_text,omitempty"`
}
