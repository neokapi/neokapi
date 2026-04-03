package jobs

import "time"

// ExtractionJobStatus represents the lifecycle state of an extraction job.
type ExtractionJobStatus string

const (
	ExtractionStatusQueued     ExtractionJobStatus = "queued"
	ExtractionStatusProcessing ExtractionJobStatus = "processing"
	ExtractionStatusCompleted  ExtractionJobStatus = "completed"
	ExtractionStatusFailed     ExtractionJobStatus = "failed"
)

// ExtractionJob represents an async entity/term extraction request.
type ExtractionJob struct {
	ID            string              `json:"id"`
	WorkspaceSlug string              `json:"workspace_slug"`
	ProjectID     string              `json:"project_id"`
	ItemName      string              `json:"item_name"`
	Locale        string              `json:"locale"`
	PushID        string              `json:"push_id,omitempty"`
	StepID        string              `json:"step_id,omitempty"` // automation step ID (AD-035)
	Model         string              `json:"model,omitempty"`
	Status        ExtractionJobStatus `json:"status"`
	TotalBlocks   int                 `json:"total_blocks"`
	DoneBlocks    int                 `json:"done_blocks"`
	ItemsCreated  int                 `json:"items_created"` // review queue items created
	Error         string              `json:"error,omitempty"`
	CreatedAt     time.Time           `json:"created_at"`
	UpdatedAt     time.Time           `json:"updated_at"`
}
