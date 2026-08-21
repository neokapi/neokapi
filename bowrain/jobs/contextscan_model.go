package jobs

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/ai/tools"
	coreprofile "github.com/neokapi/neokapi/core/profile"
)

// ContextScanJobStatus represents the lifecycle state of a brand-scan job.
type ContextScanJobStatus string

const (
	ContextScanStatusQueued     ContextScanJobStatus = "queued"
	ContextScanStatusProcessing ContextScanJobStatus = "processing"
	ContextScanStatusCompleted  ContextScanJobStatus = "completed"
	ContextScanStatusFailed     ContextScanJobStatus = "failed"
)

// ContextScanJob is an async AI brand-onboarding scan (epic 016): the worker
// assembles a corpus from the request's sources, infers a draft voice profile
// plus candidate glossary terms, and persists the result for review. The job
// mirrors ExtractionJob in shape, but carries the translation store's
// claim-epoch lease and retry-attempt tracking because the scan is billed
// against platform credits.
type ContextScanJob struct {
	ID            string               `json:"id"`
	WorkspaceID   string               `json:"workspace_id,omitempty"` // billing workspace ID
	WorkspaceSlug string               `json:"workspace_slug"`
	Status        ContextScanJobStatus `json:"status"`
	Progress      int                  `json:"progress"` // 0-100
	Message       string               `json:"message,omitempty"`
	Request       json.RawMessage      `json:"request,omitempty"`
	Result        json.RawMessage      `json:"result,omitempty"`
	Error         string               `json:"error,omitempty"`
	Attempts      int                  `json:"attempts"`
	ClaimEpoch    int64                `json:"claim_epoch"`
	TokensUsed    int                  `json:"tokens_used"`
	CreatedAt     time.Time            `json:"created_at"`
	UpdatedAt     time.Time            `json:"updated_at"`
}

// MaxContextScanURLs caps the number of user-supplied URLs per scan request.
const MaxContextScanURLs = 5

// ContextScanRequest is the request payload persisted on a brand-scan job:
// the "give us anything" input surface (paste text, links, uploaded files,
// a repository URL) plus optional naming hints for the draft profile.
type ContextScanRequest struct {
	PasteText   string   `json:"paste_text,omitempty"`
	URLs        []string `json:"urls,omitempty"`
	RepoURL     string   `json:"repo_url,omitempty"`
	UploadKeys  []string `json:"upload_keys,omitempty"` // blob keys from the uploads endpoint
	ProfileName string   `json:"profile_name,omitempty"`
	Domain      string   `json:"domain,omitempty"`
}

// HasSource reports whether the request names at least one scan source.
func (r *ContextScanRequest) HasSource() bool {
	if r == nil {
		return false
	}
	return strings.TrimSpace(r.PasteText) != "" ||
		len(r.URLs) > 0 ||
		strings.TrimSpace(r.RepoURL) != "" ||
		len(r.UploadKeys) > 0
}

// ContextScanSource records one source's contribution to the scan result.
// Runes is zero for a source that was skipped (unreadable, unsupported, or
// deferred file type); the skip reason is emitted to the step logs.
type ContextScanSource struct {
	Kind  string `json:"kind"`
	Label string `json:"label"`
	Runes int    `json:"runes"`
}

// ContextScanResult is the result payload persisted on a completed brand-scan
// job: the draft voice profile, its per-field evidence sidecar, candidate
// glossary terms, and the per-source corpus accounting.
type ContextScanResult struct {
	Profile   *coreprofile.VoiceProfile `json:"profile"`
	Evidence  *tools.DraftEvidence      `json:"evidence"`
	Terms     []tools.TermEntry         `json:"terms"`
	Sources   []ContextScanSource       `json:"sources"`
	Truncated bool                      `json:"truncated"`
}

// ContextScanUploadEnvelope is the blob payload written by the brand-scan
// uploads endpoint. Blob keys are content hashes with no metadata, so the
// original filename and content type — which drive format-reader selection in
// contextscan.ExtractFile — travel inside the blob itself.
type ContextScanUploadEnvelope struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type,omitempty"`
	Data        []byte `json:"data"`
}

// ContextScanUploadCleanup names one terminal brand-scan job whose uploaded
// source envelopes are due for deletion, with the blob keys its stored
// request carries. Produced by SweepExpiredContextScanUploads and consumed by
// the ContextScanUploadSweeper.
type ContextScanUploadCleanup struct {
	JobID      string
	UploadKeys []string
}

// contextScanCorpusMaxRunes bounds the assembled scan corpus (contract: the
// sources merge into one source-tagged corpus capped at 100k runes).
const contextScanCorpusMaxRunes = 100000
