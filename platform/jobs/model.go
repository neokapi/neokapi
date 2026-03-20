package jobs

import "time"

// JobStatus represents the lifecycle state of a translation job.
type JobStatus string

const (
	StatusQueued     JobStatus = "queued"
	StatusProcessing JobStatus = "processing"
	StatusCompleted  JobStatus = "completed"
	StatusFailed     JobStatus = "failed"
)

// TranslationJob represents an async translation request.
type TranslationJob struct {
	ID               string    `json:"id"`
	WorkspaceSlug    string    `json:"workspace_slug"`
	WorkspaceID      string    `json:"workspace_id,omitempty"` // billing workspace ID (set when created from workspace context)
	ProjectID        string    `json:"project_id"`
	ItemName         string    `json:"item_name"`
	TargetLocale     string    `json:"target_locale"`
	ProviderConfigID string    `json:"provider_config_id"`
	Model            string    `json:"model,omitempty"` // deployment/model name (e.g. "gpt-4o", "gpt-4o-mini")
	PushID           string    `json:"push_id,omitempty"`
	Status           JobStatus `json:"status"`
	Progress         int       `json:"progress"` // 0-100
	TotalBlocks      int       `json:"total_blocks"`
	DoneBlocks       int       `json:"done_blocks"`
	BatchSize        int       `json:"batch_size,omitempty"`
	Concurrency      int       `json:"concurrency,omitempty"`
	TokensUsed       int       `json:"tokens_used"`
	Error            string    `json:"error,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// IsPlatformProvider returns true if the job should use the platform-provided
// Azure OpenAI service (managed identity auth) rather than a user-configured provider.
func (j *TranslationJob) IsPlatformProvider() bool {
	return j.ProviderConfigID == "" || j.ProviderConfigID == "platform"
}
