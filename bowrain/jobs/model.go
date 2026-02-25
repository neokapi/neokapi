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
	ProjectID        string    `json:"project_id"`
	ItemName         string    `json:"item_name"`
	TargetLocale     string    `json:"target_locale"`
	ProviderConfigID string    `json:"provider_config_id"`
	Status           JobStatus `json:"status"`
	Progress         int       `json:"progress"`      // 0-100
	TotalBlocks      int       `json:"total_blocks"`
	DoneBlocks       int       `json:"done_blocks"`
	Error            string    `json:"error,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}
