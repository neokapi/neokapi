package jobs

import "context"

// JobStore persists translation jobs.
type JobStore interface {
	CreateJob(ctx context.Context, job *TranslationJob) error
	GetJob(ctx context.Context, id string) (*TranslationJob, error)
	ListJobs(ctx context.Context, workspaceSlug string, limit int) ([]*TranslationJob, error)
	UpdateJobProgress(ctx context.Context, id string, doneBlocks, totalBlocks int) error
	UpdateJobStatus(ctx context.Context, id string, status JobStatus, errMsg string) error
	DeleteJob(ctx context.Context, id string) error
	ListJobsByPushID(ctx context.Context, pushID string) ([]*TranslationJob, error)
	// ClaimJob atomically transitions a job from queued to processing.
	// Returns true if this caller won the claim, false if another worker already claimed it.
	ClaimJob(ctx context.Context, id string) (bool, error)
}
