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
	ID            string `json:"id"`
	WorkspaceSlug string `json:"workspace_slug"`
	WorkspaceID   string `json:"workspace_id,omitempty"` // billing workspace ID (set when created from workspace context)
	ProjectID     string `json:"project_id"`
	ItemName      string `json:"item_name"`
	// Stream scopes the job's reads and writes (targets are per-stream
	// overlays). Empty means "main" — the worker defaults it, so an old row
	// or a stream-naive caller keeps today's behavior. Distinct from the
	// __sync_push__ convention of carrying the stream in TargetLocale, which
	// was display-only punning; content-affecting code reads this field.
	Stream           string `json:"stream,omitempty"`
	TargetLocale     string `json:"target_locale"`
	ProviderConfigID string `json:"provider_config_id"`
	Model            string `json:"model,omitempty"` // deployment/model name (e.g. "gpt-4o", "gpt-4o-mini")
	PushID           string `json:"push_id,omitempty"`
	StepID           string `json:"step_id,omitempty"` // automation step ID for run visibility (Bowrain AD-013)
	// CreatedBy is the user who asked for this job, empty for the jobs the
	// platform starts itself (automation fan-out, forge ingest, model sweep).
	// It exists so a failure can reach the person waiting on it: without it
	// every failure is system-initiated as far as the notification path can
	// tell, and the only honest recipient is the workspace's owners.
	CreatedBy   string    `json:"created_by,omitempty"`
	Status      JobStatus `json:"status"`
	Progress    int       `json:"progress"` // 0-100
	TotalBlocks int       `json:"total_blocks"`
	DoneBlocks  int       `json:"done_blocks"`
	// ViaMemory/ViaAI record the content memory-first split for this job: how many blocks were
	// recycled from the project content memory vs. sent to the AI translator. The
	// convergence produce emitter sums these across a locale's jobs to report a
	// truthful "content memory N · AI M" (theme A2).
	ViaMemory   int       `json:"via_tm"`
	ViaAI       int       `json:"via_ai"`
	BatchSize   int       `json:"batch_size,omitempty"`
	Concurrency int       `json:"concurrency,omitempty"`
	TokensUsed  int       `json:"tokens_used"`
	Error       string    `json:"error,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// IsPlatformProvider returns true if the job should use the platform-provided
// Azure OpenAI service (managed identity auth) rather than a user-configured provider.
func (j *TranslationJob) IsPlatformProvider() bool {
	return j.ProviderConfigID == "" || j.ProviderConfigID == "platform"
}

// SyncPushItemName is the sentinel ItemName that marks a job row as a sync
// protocol push rather than a translation, alongside ForgeIngestItemName and
// ModelSweepItemName. The literal predates the constant, which is why the
// router still reads the way it does.
const SyncPushItemName = "__sync_push__"

// Kind names what this job is, for the humans who read about it rather than
// for the router. The queue carries four kinds of work on one table and tells
// them apart by sentinel ItemNames; a notification that said "job failed" and
// nothing else would make the reader open the queue to learn which one.
func (j *TranslationJob) Kind() string {
	switch {
	case j.ItemName == SyncPushItemName:
		return "sync"
	case j.IsForgeIngest():
		return "ingest"
	case j.IsModelSweep():
		return "model-sweep"
	default:
		return "translation"
	}
}
