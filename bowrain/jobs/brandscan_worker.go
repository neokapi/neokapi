package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/neokapi/neokapi/bowrain/billing"
	"github.com/neokapi/neokapi/bowrain/brandscan"
	"github.com/neokapi/neokapi/bowrain/connector"
	platconn "github.com/neokapi/neokapi/bowrain/core/connector"
	"github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	corestorage "github.com/neokapi/neokapi/core/storage"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// BrandScanWorkerDeps holds dependencies for the brand-scan worker (epic 016).
type BrandScanWorkerDeps struct {
	Store BrandScanJobStore
	Queue Queue
	// BlobStore resolves the request's upload_keys (brand-scan upload
	// envelopes written by the server's uploads endpoint).
	BlobStore corestorage.BlobStore
	// Platform is the env-configured platform AI provider. Brand scans run
	// exclusively on the platform key (they are part of onboarding, before any
	// BYO key exists), so a nil Platform fails every scan with a clear error.
	Platform *PlatformProviderConfig
	// BillingHooks deducts platform credits for the scan's token usage.
	// Optional; nil (self-hosted / unbilled) disables credit deduction.
	BillingHooks *billing.UsageHooks
	// QuotaStore is the internal AI abuse cap (ai_usage / ai_quotas): scans
	// check the hard monthly token ceiling up front and record every phase's
	// usage, exactly like translation jobs, so brand scans are never invisible
	// to abuse detection. Distinct from the credit ledger (BillingHooks).
	// Optional; nil disables quota enforcement.
	QuotaStore QuotaStore
	// LogFunc emits structured step logs. Optional.
	// Signature: func(stepID, level, message string, data map[string]string).
	LogFunc func(stepID, level, message string, data map[string]string)
	// MaxJobAttempts bounds transient-failure retries before a job is failed.
	// Zero uses defaultMaxJobAttempts.
	MaxJobAttempts int

	// formatRegistry backs the git connector's file parsing; built lazily.
	formatRegistryOnce sync.Once
	formatRegistry     *registry.FormatRegistry
}

func (d *BrandScanWorkerDeps) maxJobAttempts() int {
	if d.MaxJobAttempts > 0 {
		return d.MaxJobAttempts
	}
	return defaultMaxJobAttempts
}

func (d *BrandScanWorkerDeps) formats() *registry.FormatRegistry {
	d.formatRegistryOnce.Do(func() {
		reg := registry.NewFormatRegistry()
		formats.RegisterAll(reg)
		d.formatRegistry = reg
	})
	return d.formatRegistry
}

// brand-scan progress milestones (contract: 10 reading-sources,
// 40 assembling-corpus, 60 drafting-voice, 80 extracting-terms, 100 done).
const (
	brandScanProgressReading    = 10
	brandScanProgressAssembling = 40
	brandScanProgressDrafting   = 60
	brandScanProgressTerms      = 80
)

// brandScanTermChunkRunes bounds how much corpus text a single term-extract
// prompt carries; the corpus is split into chunks of this size.
const brandScanTermChunkRunes = 12000

// brandScanMaxTermChunks caps the number of term-extract calls per scan so a
// full 100k-rune corpus cannot fan out into unbounded provider spend.
const brandScanMaxTermChunks = 6

// Bounds on user-supplied brand-scan sources beyond the per-fetch caps, so a
// pathological source set cannot exhaust the worker's memory or disk before
// AssembleCorpus applies the brandScanCorpusMaxRunes truncation.
const (
	// brandScanSourceTextCapBytes bounds both the running total of text
	// gathered across all sources and the repository markdown concatenation.
	// A rune occupies at most four bytes, so this budget can never starve the
	// brandScanCorpusMaxRunes corpus cap.
	brandScanSourceTextCapBytes = 4 * brandScanCorpusMaxRunes

	// brandScanRepoMaxFiles caps how many matched repository files are parsed.
	brandScanRepoMaxFiles = 200

	// brandScanRepoMaxFileBytes caps a single repository file read into memory.
	brandScanRepoMaxFileBytes = 2 << 20 // 2 MiB
)

// brandScanBudgetSkipReason is the user-facing skip reason for sources left
// unread because earlier sources already filled the gather budget.
const brandScanBudgetSkipReason = "not read: earlier sources already filled the scan corpus"

// RunBrandScanWorker runs the brand-scan worker loop. It blocks until ctx is
// cancelled. The loop mirrors RunWorkerWithDeps (the translation worker),
// including the transient-failure re-enqueue semantics, because brand scans
// carry the same lease/retry model.
func RunBrandScanWorker(ctx context.Context, deps *BrandScanWorkerDeps) error {
	slog.InfoContext(ctx, "brand-scan worker started")
	defer slog.InfoContext(ctx, "brand-scan worker stopped")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		jobID, ack, nack, err := deps.Queue.Dequeue(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			slog.WarnContext(ctx, "brand-scan dequeue error", "error", err)
			sleepCtx(ctx, 2*time.Second)
			continue
		}

		processErr := processBrandScanJob(ctx, deps, jobID)
		if processErr != nil {
			var te *transientError
			if errors.As(processErr, &te) {
				// Same rationale as the translation worker: publish a FRESH
				// message and ACK this delivery, falling back to NAK only when
				// the fresh enqueue fails (see RunWorkerWithDeps).
				if eqErr := deps.Queue.Enqueue(ctx, jobID); eqErr != nil {
					slog.WarnContext(ctx, "brand-scan transient failure; re-enqueue failed, nacking instead",
						"job_id", jobID, "error", processErr, "enqueue_error", eqErr)
					nack()
				} else {
					slog.WarnContext(ctx, "brand-scan transient failure; re-enqueued fresh message for retry",
						"job_id", jobID, "error", processErr)
					ack()
				}
				continue
			}
			slog.ErrorContext(ctx, "brand-scan job failed", "job_id", jobID, "error", processErr)
		}
		ack()
	}
}

// processBrandScanJob claims and executes one brand-scan job, mirroring
// processJobWithDeps' claim / lease-loss / transient-retry handling.
func processBrandScanJob(ctx context.Context, deps *BrandScanWorkerDeps, jobID string) error {
	claimed, epoch, err := deps.Store.ClaimBrandScanJob(ctx, jobID)
	if err != nil {
		return fmt.Errorf("claim brand-scan job: %w", err)
	}
	if !claimed {
		slog.DebugContext(ctx, "brand-scan job already claimed, skipping", "job_id", jobID)
		return nil
	}

	job, err := deps.Store.GetBrandScanJob(ctx, jobID)
	if err != nil {
		return fmt.Errorf("load brand-scan job: %w", err)
	}

	// Check the monthly token quota before starting. This is the internal abuse
	// cap (see jobs.QuotaStore) — a hard ceiling to bound runaway usage — not
	// the user-facing credit ledger; the route-level QuotaGuard checks only
	// credits, so without this pre-check a scan would bypass the abuse cap
	// entirely. Mirrors the translation worker's pre-check.
	if deps.QuotaStore != nil {
		remaining, qerr := deps.QuotaStore.CheckQuota(ctx, job.WorkspaceSlug)
		if qerr != nil {
			slog.WarnContext(ctx, "brand-scan quota check failed", "workspace", job.WorkspaceSlug, "error", qerr)
		} else if remaining <= 0 {
			_, _ = deps.Store.FailBrandScanJob(ctx, jobID, epoch, "workspace AI quota exceeded")
			emitBrandScanLog(deps, jobID, "error", "Brand scan failed: workspace AI quota exceeded", nil)
			return fmt.Errorf("workspace %s quota exceeded", job.WorkspaceSlug)
		}
	}

	if err := executeBrandScan(ctx, deps, job, epoch); err != nil {
		if errors.Is(err, errLeaseLost) {
			slog.InfoContext(ctx, "brand-scan lease lost mid-run; abandoning to fresh owner",
				"job_id", jobID)
			return nil
		}
		if isTransientError(err) {
			retry, rerr := deps.Store.RetryOrFail(ctx, jobID, epoch, deps.maxJobAttempts(), err.Error())
			if rerr != nil {
				slog.WarnContext(ctx, "brand-scan retry bookkeeping failed", "job_id", jobID, "error", rerr)
			}
			if retry {
				emitBrandScanLog(deps, jobID, "warn", "Transient error, retrying: "+err.Error(), nil)
				return &transientError{err: err}
			}
			emitBrandScanLog(deps, jobID, "error", "Brand scan failed after retries: "+err.Error(), nil)
			return err
		}
		// Epoch-guarded terminal write: a stale worker's permanent error must
		// not overwrite a fresh owner's completed (already billed) run.
		owner, ferr := deps.Store.FailBrandScanJob(ctx, jobID, epoch, err.Error())
		if ferr != nil {
			slog.WarnContext(ctx, "brand-scan failure bookkeeping failed", "job_id", jobID, "error", ferr)
		} else if !owner {
			slog.InfoContext(ctx, "brand-scan lease lost; leaving fresh owner's run untouched",
				"job_id", jobID)
			return nil
		}
		emitBrandScanLog(deps, jobID, "error", "Brand scan failed: "+err.Error(), nil)
		return err
	}
	return nil
}

// executeBrandScan runs the scan pipeline: gather sources → assemble corpus →
// infer voice draft → extract candidate terms → persist result. Credit
// deduction happens after the infer and term phases, each gated on a fresh
// lease check so a swept-and-reclaimed job cannot double-bill.
func executeBrandScan(ctx context.Context, deps *BrandScanWorkerDeps, job *BrandScanJob, epoch int64) error {
	stopHeartbeat := startLeaseHeartbeat(ctx, deps.Store, job.ID, epoch)
	defer stopHeartbeat()

	var req BrandScanRequest
	if err := json.Unmarshal(job.Request, &req); err != nil {
		return fmt.Errorf("decode brand-scan request: %w", err)
	}
	if !req.HasSource() {
		return errors.New("brand scan has no sources")
	}

	setBrandScanProgress(ctx, deps, job.ID, epoch, brandScanProgressReading, "reading-sources")

	sources, skipped := gatherBrandScanSources(ctx, deps, job.ID, &req)
	if len(sources) == 0 {
		reasons := make([]string, 0, len(skipped))
		for _, sk := range skipped {
			reasons = append(reasons, fmt.Sprintf("%s: %s", sk.label, sk.reason))
		}
		return fmt.Errorf("no readable brand material in the provided sources (%s)",
			strings.Join(reasons, "; "))
	}

	setBrandScanProgress(ctx, deps, job.ID, epoch, brandScanProgressAssembling, "assembling-corpus")
	corpus := brandscan.AssembleCorpus(sources, brandScanCorpusMaxRunes)
	if strings.TrimSpace(corpus.Text) == "" {
		return errors.New("the provided sources contain no analyzable text; " +
			"add pasted text, pages, or files with readable content and try again")
	}

	// Build the platform provider. Brand scans always run on the platform key
	// (the request carries no provider config), so credit deduction below is
	// unconditionally platform-metered.
	if deps.Platform == nil {
		return errors.New("platform provider not configured " +
			"(set BOWRAIN_PLATFORM_PROVIDER + key for self-hosted/local, " +
			"or BOWRAIN_OPENAI_ENDPOINT for Azure OpenAI)")
	}
	prov, provKind, err := deps.Platform.Build("")
	if err != nil {
		return fmt.Errorf("build platform provider: %w", err)
	}
	// Model label for the ai_usage abuse-cap records: the operator-configured
	// platform model when set, otherwise the provider kind (Azure path).
	usageModel := deps.Platform.Model
	if usageModel == "" {
		usageModel = provKind
	}

	// Phase: draft the voice profile.
	setBrandScanProgress(ctx, deps, job.ID, epoch, brandScanProgressDrafting, "drafting-voice")
	recorder := &usageRecordingProvider{LLMProvider: prov}
	draft, evidence, err := tools.InferVoiceProfile(ctx, recorder, corpus.Text, tools.InferOptions{
		ProfileName: req.ProfileName,
		Domain:      req.Domain,
	})
	if err != nil {
		if errors.Is(err, tools.ErrEmptyCorpus) {
			return errors.New("the provided sources contain no analyzable text; " +
				"add pasted text, pages, or files with readable content and try again")
		}
		return fmt.Errorf("infer voice profile: %w", err)
	}

	inferUsage := recorder.TotalUsage()
	inferTokens := inferUsage.TotalTokens()
	if inferTokens <= 0 {
		inferTokens = utf8.RuneCountInString(corpus.Text) / 4 // heuristic fallback
	}
	totalTokens := inferTokens

	// Lease gate + billing for the infer phase: deduct only while we still own
	// the job, so a retried/reclaimed run cannot double-bill (mirrors the
	// translation worker's per-chunk gate).
	owner, lerr := deps.Store.RenewLease(ctx, job.ID, epoch)
	if lerr != nil {
		return fmt.Errorf("renew lease: %w", lerr)
	}
	if !owner {
		return errLeaseLost
	}
	// Record the phase in ai_usage (the abuse cap sees ALL AI traffic), deduct
	// credits, and persist the running token total so a scan that fails after
	// this point still reports the tokens actually billed.
	recordBrandScanUsage(ctx, deps, job, usageModel, inferUsage, inferTokens)
	if deps.BillingHooks != nil && job.WorkspaceID != "" {
		deps.BillingHooks.DeductTokens(ctx, job.WorkspaceID, inferTokens, "brand_scan", job.ID+":infer")
	}
	if terr := deps.Store.AddBrandScanTokens(ctx, job.ID, epoch, inferTokens); terr != nil {
		slog.WarnContext(ctx, "brand-scan token bookkeeping failed", "job_id", job.ID, "error", terr)
	}

	// Phase: extract candidate glossary terms over the corpus using the
	// existing term-extract tool machinery. A failure here degrades to the
	// vocabulary-derived candidates rather than discarding the drafted voice.
	setBrandScanProgress(ctx, deps, job.ID, epoch, brandScanProgressTerms, "extracting-terms")
	terms, termUsage, termErr := extractBrandScanTerms(ctx, prov, corpus.Text, req.Domain)
	if termErr != nil {
		slog.WarnContext(ctx, "brand-scan term extraction failed; keeping vocabulary-derived candidates only",
			"job_id", job.ID, "error", termErr)
		emitBrandScanLog(deps, job.ID, "warn", "Term extraction failed: "+termErr.Error(), nil)
	}
	terms = mergeBrandScanTerms(terms, draft, req.Domain)

	if termTokens := termUsage.TotalTokens(); termTokens > 0 {
		owner, lerr := deps.Store.RenewLease(ctx, job.ID, epoch)
		if lerr != nil {
			return fmt.Errorf("renew lease: %w", lerr)
		}
		if !owner {
			return errLeaseLost
		}
		recordBrandScanUsage(ctx, deps, job, usageModel, termUsage, termTokens)
		if deps.BillingHooks != nil && job.WorkspaceID != "" {
			deps.BillingHooks.DeductTokens(ctx, job.WorkspaceID, termTokens, "brand_scan", job.ID+":terms")
		}
		if terr := deps.Store.AddBrandScanTokens(ctx, job.ID, epoch, termTokens); terr != nil {
			slog.WarnContext(ctx, "brand-scan token bookkeeping failed", "job_id", job.ID, "error", terr)
		}
		totalTokens += termTokens
	}

	// Persist the result. Skipped sources appear with zero runes so the review
	// UI can attribute what was (and was not) read.
	result := BrandScanResult{
		Profile:   draft,
		Evidence:  evidence,
		Terms:     terms,
		Truncated: corpus.Truncated,
	}
	for _, s := range corpus.Sources {
		result.Sources = append(result.Sources, BrandScanSource{
			Kind: string(s.Kind), Label: s.Label, Runes: s.Runes,
		})
	}
	for _, sk := range skipped {
		result.Sources = append(result.Sources, BrandScanSource{
			Kind: sk.kind, Label: sk.label, Runes: 0,
		})
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("encode brand-scan result: %w", err)
	}

	// Final lease-guarded persist: CompleteBrandScanJob writes the result and
	// terminal status only while we still own the claim epoch.
	owner, err = deps.Store.CompleteBrandScanJob(ctx, job.ID, epoch, resultJSON, totalTokens)
	if err != nil {
		return fmt.Errorf("complete brand-scan job: %w", err)
	}
	if !owner {
		return errLeaseLost
	}

	setBrandScanLogDone(deps, job.ID, len(terms), totalTokens)
	return nil
}

// skippedBrandScanSource records a source that yielded no text, with the
// user-facing reason (emitted to step logs and surfaced as a zero-rune source
// in the result).
type skippedBrandScanSource struct {
	kind   string
	label  string
	reason string
}

// gatherBrandScanSources fans the request out into source-tagged text: paste
// verbatim, URLs via the SSRF-guarded fetcher, uploads via blob download +
// format-reader extraction, and the repository via the git connector. A
// source that cannot be read becomes a per-source warning + skip, never a job
// failure on its own.
func gatherBrandScanSources(ctx context.Context, deps *BrandScanWorkerDeps, jobID string, req *BrandScanRequest) ([]brandscan.Source, []skippedBrandScanSource) {
	var sources []brandscan.Source
	var skipped []skippedBrandScanSource
	skip := func(kind, label, reason string) {
		skipped = append(skipped, skippedBrandScanSource{kind: kind, label: label, reason: reason})
		emitBrandScanLog(deps, jobID, "warn",
			fmt.Sprintf("Skipped %s source %q: %s", kind, label, reason),
			map[string]string{"kind": kind, "label": label})
	}
	// Running byte budget across sources: once earlier sources hold more text
	// than the corpus can carry, later sources are skipped without being
	// fetched, so a request cannot pile unbounded text into worker memory.
	totalBytes := 0
	overBudget := func() bool { return totalBytes >= brandScanSourceTextCapBytes }
	add := func(kind brandscan.SourceKind, label, text string) {
		sources = append(sources, brandscan.Source{Kind: kind, Label: label, Text: text})
		totalBytes += len(text)
	}

	if text := strings.TrimSpace(req.PasteText); text != "" {
		add(brandscan.SourcePaste, "pasted text", text)
	}

	urls := req.URLs
	if len(urls) > MaxBrandScanURLs {
		urls = urls[:MaxBrandScanURLs] // defense in depth; the endpoint rejects >5
	}
	for _, u := range urls {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		if overBudget() {
			skip(string(brandscan.SourceURL), u, brandScanBudgetSkipReason)
			continue
		}
		text, err := brandscan.FetchURL(ctx, u)
		if err != nil {
			// The detailed error can carry resolver output (which IPs a host
			// resolves to and why they were rejected); that stays in server
			// logs only — the skip reason lands in job.Error and the step
			// logs, both user-visible.
			slog.WarnContext(ctx, "brand-scan url source failed",
				"job_id", jobID, "url", u, "error", err)
			skip(string(brandscan.SourceURL), u,
				"the page could not be fetched (only public https pages are readable)")
			continue
		}
		add(brandscan.SourceURL, u, text)
	}

	for _, key := range req.UploadKeys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if overBudget() {
			skip(string(brandscan.SourceFile), key, brandScanBudgetSkipReason)
			continue
		}
		label, text, err := readBrandScanUpload(ctx, deps.BlobStore, key)
		if label == "" {
			label = key
		}
		if err != nil {
			skip(string(brandscan.SourceFile), label, err.Error())
			continue
		}
		add(brandscan.SourceFile, label, text)
	}

	if repoURL := strings.TrimSpace(req.RepoURL); repoURL != "" {
		if overBudget() {
			skip(string(brandscan.SourceRepo), repoURL, brandScanBudgetSkipReason)
			return sources, skipped
		}
		text, err := fetchBrandScanRepo(ctx, deps.formats(), repoURL)
		if err != nil {
			// Same policy as the url path: git stderr and resolver detail are
			// an internal-network oracle, so they never reach the user-facing
			// skip reason.
			slog.WarnContext(ctx, "brand-scan repository source failed",
				"job_id", jobID, "repo", repoURL, "error", err)
			skip(string(brandscan.SourceRepo), repoURL,
				"the repository could not be read (only public https repositories are readable)")
		} else if strings.TrimSpace(text) == "" {
			skip(string(brandscan.SourceRepo), repoURL, "no markdown documentation found in the repository")
		} else {
			add(brandscan.SourceRepo, repoURL, text)
		}
	}

	return sources, skipped
}

// readBrandScanUpload downloads a brand-scan upload envelope and extracts its
// text via the brandscan format readers. It returns the original filename as
// the source label (best effort — falls back to the key on decode failure).
func readBrandScanUpload(ctx context.Context, blobs corestorage.BlobStore, key string) (label, text string, err error) {
	if blobs == nil {
		return "", "", errors.New("blob storage not configured")
	}
	// Blob-store errors can leak backend detail (endpoints, auth failures) and
	// the key is caller-controlled, so failures surface as a generic reason;
	// the detail goes to server logs only. Extraction errors below pass
	// through — they are the extractor's own user-facing messages (unsupported
	// type, deferred pdf/pptx).
	rc, err := blobs.Download(ctx, key)
	if err != nil {
		slog.WarnContext(ctx, "brand-scan upload download failed", "key", key, "error", err)
		return "", "", errors.New("the uploaded file could not be read")
	}
	defer rc.Close()
	// The envelope wraps a file capped at brandscan.MaxFileBytes; the limit
	// leaves headroom for the JSON + base64 framing.
	raw, err := io.ReadAll(io.LimitReader(rc, 2*brandscan.MaxFileBytes))
	if err != nil {
		slog.WarnContext(ctx, "brand-scan upload read failed", "key", key, "error", err)
		return "", "", errors.New("the uploaded file could not be read")
	}
	var env BrandScanUploadEnvelope
	if err := json.Unmarshal(raw, &env); err != nil || env.Filename == "" {
		return "", "", errors.New("blob is not a brand-scan upload")
	}
	text, err = brandscan.ExtractFile(env.Data, env.Filename, env.ContentType) //nolint:contextcheck // in-memory reader over already-downloaded bytes; no cancellation surface
	if err != nil {
		return env.Filename, "", err
	}
	return env.Filename, text, nil
}

// fetchBrandScanRepo clones the repository (into a scan-scoped temp dir,
// removed afterwards) via the git connector and concatenates the source text
// of its markdown documentation. The connector's default glob set excludes
// markdown, so the patterns are set explicitly.
//
// The repo URL comes straight from an API request, so it gets the same
// destination policy as the urls[] path (brandscan.FetchURL): https only —
// which also excludes the scp-like SSH form — and a host that resolves to no
// loopback/private/link-local/CGNAT address. The git subprocess re-resolves
// DNS itself, so the vet is a pre-check rather than a rebinding-proof pin;
// as compensating controls the clone is shallow and redirect following is
// disabled, so the vetted host stays the only transfer destination.
func fetchBrandScanRepo(ctx context.Context, formatReg *registry.FormatRegistry, repoURL string) (string, error) {
	u, err := url.Parse(repoURL)
	if err != nil {
		return "", fmt.Errorf("invalid repository URL: %w", err)
	}
	if !strings.EqualFold(u.Scheme, "https") || u.Hostname() == "" {
		return "", errors.New("brand scans accept only public https:// repository URLs")
	}
	if err := brandscan.VetPublicHost(ctx, u.Hostname()); err != nil {
		return "", err
	}

	dir, err := os.MkdirTemp("", "brandscan-git-")
	if err != nil {
		return "", fmt.Errorf("create scan checkout dir: %w", err)
	}
	defer os.RemoveAll(dir)

	conn, err := connector.NewGitConnector(formatReg, map[string]string{
		"repo":       repoURL,
		"local_path": dir,
		// README/docs harvesting: root-level and nested markdown.
		"patterns": "*.md,**/*.md,*.markdown,**/*.markdown",
		// One-shot harvest of a user-supplied repository: keep the fetch
		// bounded and the vetted host the only destination.
		"shallow":               "true",
		"http_follow_redirects": "false",
	})
	if err != nil {
		return "", err
	}
	defer conn.Close()

	// List (which clones) and then fetch a bounded selection: at most
	// brandScanRepoMaxFiles files, each at most brandScanRepoMaxFileBytes, so
	// a huge checkout cannot be parsed wholesale into worker memory.
	all, err := conn.List(ctx)
	if err != nil {
		return "", err
	}
	paths := make([]string, 0, min(len(all), brandScanRepoMaxFiles))
	for _, item := range all {
		if len(paths) >= brandScanRepoMaxFiles {
			break
		}
		info, statErr := os.Stat(filepath.Join(dir, item.Path))
		if statErr != nil || info.Size() > brandScanRepoMaxFileBytes {
			continue
		}
		paths = append(paths, item.Path)
	}
	if len(paths) == 0 {
		return "", nil
	}
	items, err := conn.Fetch(ctx, platconn.FetchOptions{Paths: paths})
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	for _, item := range items {
		if sb.Len() >= brandScanSourceTextCapBytes {
			break
		}
		for _, block := range item.Blocks {
			if sb.Len() >= brandScanSourceTextCapBytes {
				break
			}
			if block == nil || !block.Translatable {
				continue
			}
			text := strings.TrimSpace(block.SourceText())
			if text == "" {
				continue
			}
			if sb.Len() > 0 {
				sb.WriteString("\n\n")
			}
			sb.WriteString(text)
		}
	}
	return sb.String(), nil
}

// recordBrandScanUsage records one scan phase's token usage in ai_usage — the
// internal abuse cap, which must see all AI traffic (platform and BYO alike;
// brand scans are always platform). Best-effort, like the translation worker's
// per-chunk recording.
func recordBrandScanUsage(ctx context.Context, deps *BrandScanWorkerDeps, job *BrandScanJob, usageModel string, usage aiprovider.TokenUsage, totalTokens int) {
	if deps.QuotaStore == nil {
		return
	}
	_ = deps.QuotaStore.RecordUsage(ctx, AIUsageRecord{
		WorkspaceSlug: job.WorkspaceSlug,
		WorkspaceID:   job.WorkspaceID,
		JobID:         job.ID,
		Model:         usageModel,
		Operation:     "brand_scan",
		PromptTokens:  usage.InputTokens,
		OutputTokens:  usage.OutputTokens,
		TotalTokens:   totalTokens,
	})
}

// extractBrandScanTerms drives the existing term-extract tool over the corpus
// (chunked into prompt-sized blocks) and returns the deduplicated candidate
// terms plus the provider token usage of the pass.
func extractBrandScanTerms(ctx context.Context, prov aiprovider.LLMProvider, corpusText, domain string) ([]tools.TermEntry, aiprovider.TokenUsage, error) {
	termTool := tools.NewAITerminologyTool(prov, tools.AITerminologyConfig{
		Locale: model.LocaleEnglish,
		Domain: domain,
	})

	chunks := chunkRunes(corpusText, brandScanTermChunkRunes, brandScanMaxTermChunks)
	parts := make([]*model.Part, 0, len(chunks))
	for i, chunk := range chunks {
		block := &model.Block{
			ID:           fmt.Sprintf("brandscan-corpus-%d", i),
			Translatable: true,
			Source:       []model.Run{{Text: &model.TextRun{Text: chunk}}},
		}
		parts = append(parts, &model.Part{Type: model.PartBlock, Resource: block})
	}

	outParts, err := runToolOnParts(ctx, termTool, parts)
	usage := termTool.TotalUsage()
	if err != nil {
		return nil, usage, err
	}

	var terms []tools.TermEntry
	for _, pt := range outParts {
		block, ok := pt.Resource.(*model.Block)
		if !ok || block == nil {
			continue
		}
		raw := block.Properties["terminology"]
		if raw == "" {
			continue
		}
		var chunkTerms []tools.TermEntry
		if err := json.Unmarshal([]byte(raw), &chunkTerms); err != nil {
			continue
		}
		terms = append(terms, chunkTerms...)
	}
	return dedupeBrandScanTerms(terms), usage, nil
}

// mergeBrandScanTerms folds the drafted vocabulary's preferred terms into the
// candidate glossary (they are product/brand vocabulary the corpus itself
// evidenced) and deduplicates case-insensitively, keeping first occurrence.
func mergeBrandScanTerms(terms []tools.TermEntry, draft *brand.VoiceProfile, domain string) []tools.TermEntry {
	if draft != nil {
		for _, rule := range draft.Vocabulary.PreferredTerms {
			terms = append(terms, tools.TermEntry{
				Term:       rule.Term,
				Definition: rule.Note,
				Domain:     domain,
			})
		}
	}
	return dedupeBrandScanTerms(terms)
}

// dedupeBrandScanTerms removes empty and case-insensitively duplicated terms,
// keeping the first (highest-signal) occurrence.
func dedupeBrandScanTerms(terms []tools.TermEntry) []tools.TermEntry {
	seen := make(map[string]bool, len(terms))
	out := make([]tools.TermEntry, 0, len(terms))
	for _, t := range terms {
		key := strings.ToLower(strings.TrimSpace(t.Term))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, t)
	}
	return out
}

// chunkRunes splits s into rune-boundary chunks of at most size runes,
// keeping at most maxChunks (the remainder is dropped — the leading corpus
// text carries the highest-precedence sources).
func chunkRunes(s string, size, maxChunks int) []string {
	if size <= 0 {
		return []string{s}
	}
	var chunks []string
	for s != "" && len(chunks) < maxChunks {
		count := 0
		end := len(s)
		for i := range s {
			if count == size {
				end = i
				break
			}
			count++
		}
		chunks = append(chunks, s[:end])
		s = s[end:]
	}
	return chunks
}

// setBrandScanProgress records a milestone (epoch-guarded, so a stale worker
// cannot regress the fresh owner's progress) and mirrors it to the step logs.
func setBrandScanProgress(ctx context.Context, deps *BrandScanWorkerDeps, jobID string, epoch int64, progress int, message string) {
	if err := deps.Store.UpdateBrandScanJobProgress(ctx, jobID, epoch, progress, message); err != nil {
		slog.WarnContext(ctx, "update brand-scan progress failed", "job_id", jobID, "error", err)
	}
	emitBrandScanLog(deps, jobID, "info", message, map[string]string{"progress": strconv.Itoa(progress)})
}

func setBrandScanLogDone(deps *BrandScanWorkerDeps, jobID string, termCount, tokens int) {
	emitBrandScanLog(deps, jobID, "info",
		fmt.Sprintf("Brand scan completed — draft profile with %d candidate terms, %d tokens", termCount, tokens),
		map[string]string{"terms": strconv.Itoa(termCount), "tokens": strconv.Itoa(tokens)})
}

// emitBrandScanLog emits a step log keyed by the job id (brand scans have no
// automation step; the job id is the correlation key).
func emitBrandScanLog(deps *BrandScanWorkerDeps, jobID, level, message string, data map[string]string) {
	if deps.LogFunc != nil {
		deps.LogFunc(jobID, level, message, data)
	}
}

// usageRecordingProvider wraps an LLMProvider and accumulates the token usage
// of every structured/plain chat call — the usage-capture mechanism for
// corpus-level calls (InferVoiceProfile) that do not return usage themselves.
type usageRecordingProvider struct {
	aiprovider.LLMProvider
	mu    sync.Mutex
	usage aiprovider.TokenUsage
}

func (p *usageRecordingProvider) record(resp *aiprovider.ChatResponse, err error) {
	if err != nil || resp == nil {
		return
	}
	p.mu.Lock()
	p.usage = p.usage.Add(resp.Usage)
	p.mu.Unlock()
}

func (p *usageRecordingProvider) Chat(ctx context.Context, messages []aiprovider.Message) (*aiprovider.ChatResponse, error) {
	resp, err := p.LLMProvider.Chat(ctx, messages)
	p.record(resp, err)
	return resp, err
}

func (p *usageRecordingProvider) ChatStructured(ctx context.Context, messages []aiprovider.Message, schema aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
	resp, err := p.LLMProvider.ChatStructured(ctx, messages, schema)
	p.record(resp, err)
	return resp, err
}

// TotalUsage returns the accumulated token usage.
func (p *usageRecordingProvider) TotalUsage() aiprovider.TokenUsage {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.usage
}
