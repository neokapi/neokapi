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
// plus candidate terms, and persists the result for review. The job
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

// ArtefactKind names what a proposed artefact is. The set is deliberately
// small: these are the two things a corpus can be read for today. It is open by
// intent — gates and redaction rules are governance bound at a point in exactly
// the same way — but a kind arrives with the inference that produces it, never
// before it.
type ArtefactKind string

const (
	// ArtefactVoice is a draft voice profile with its per-field evidence.
	ArtefactVoice ArtefactKind = "voice"
	// ArtefactTerms is a set of candidate concepts for the vocabulary.
	ArtefactTerms ArtefactKind = "terms"
)

// ArtefactProposal is one thing a scan proposes, and the point it would govern.
//
// At is that point as an axis map — the same open shape
// SyncContextEntry.coordinates carries. An EMPTY At means the project's default
// point, whatever defaults.coordinates resolves to: a scan that finds no
// structure proposes one voice for the project, which is the onboarding path
// and stays a single click.
//
// Only the fields matching Kind are populated.
type ArtefactProposal struct {
	At   map[string]string `json:"at,omitempty"`
	Kind ArtefactKind      `json:"kind"`

	// voice
	Voice    *coreprofile.VoiceProfile `json:"voice,omitempty"`
	Evidence *tools.DraftEvidence      `json:"evidence,omitempty"`

	// terms
	Terms []tools.TermEntry `json:"terms,omitempty"`
}

// ContextScanResult is the result payload persisted on a completed scan: what
// the corpus proposed, and the per-source accounting of what was read.
//
// Artefacts are proposed AT POINTS rather than as one profile, so a later scan
// against a new corpus can refine what sits at a point instead of producing a
// rival profile beside it.
type ContextScanResult struct {
	// Axes are the dimensions the corpus varies along. Empty is the ordinary
	// answer for a project whose content is uniform, and for a first scan of one
	// small site — not a failure, and not a reason to withhold the artefacts.
	Axes      []tools.AxisProposal `json:"axes,omitempty"`
	Artefacts []ArtefactProposal   `json:"artefacts"`
	Sources   []ContextScanSource  `json:"sources"`
	Truncated bool                 `json:"truncated"`
}

// Voice returns the first proposed voice profile and its evidence — what a
// single-point scan produces. Callers read through this rather than walking
// Artefacts, so adding a kind cannot silently change what they see.
func (r ContextScanResult) Voice() (*coreprofile.VoiceProfile, *tools.DraftEvidence) {
	for _, a := range r.Artefacts {
		if a.Kind == ArtefactVoice {
			return a.Voice, a.Evidence
		}
	}
	return nil, nil
}

// Terms returns the candidate concepts across every proposed terms artefact.
func (r ContextScanResult) Terms() []tools.TermEntry {
	var out []tools.TermEntry
	for _, a := range r.Artefacts {
		if a.Kind == ArtefactTerms {
			out = append(out, a.Terms...)
		}
	}
	return out
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
