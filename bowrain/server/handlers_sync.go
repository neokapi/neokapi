package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"

	bowsync "github.com/neokapi/neokapi/bowrain/sync"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/analytics"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/jobs"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	pb "github.com/neokapi/neokapi/core/proto/sync/v1"
	"github.com/neokapi/neokapi/core/ref"
	"github.com/neokapi/neokapi/core/storage"
	"github.com/neokapi/neokapi/core/storage/compression"
	"github.com/neokapi/neokapi/core/venue"
	apiclient "github.com/neokapi/neokapi/host/venue/client"
)

// The sync handlers scope to a stream via refParam (editor.go): every sync
// route — workspace-scoped and flat alike — names the parameter :ref
// (AD-011). An earlier generation of these handlers read :stream, a name no
// sync route ever declared, so the lookup came back empty on every request
// and each handler quietly fell back to "main" — a branch-scoped push landed
// on the default stream regardless of the URL it arrived on.

// HandleSyncPushInit handles the first step of a push: Merkle tree diff negotiation.
// POST /sync/push/init
func (s *Server) HandleSyncPushInit(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageFiles); err != nil {
		return err
	}

	var req struct {
		ProjectID    string            `json:"project_id"`
		Stream       string            `json:"stream"`
		ContentTypes []string          `json:"content_types"`
		ItemHashes   map[string]string `json:"item_hashes"`
		RootHash     string            `json:"root_hash"`
		ContextHash  string            `json:"context_hash"`
		Collections  []string          `json:"collections"`
		// ContentModelEpoch is the generation of content model the producer
		// reads into, and AllowModelDowngrade is `push --force` carrying past
		// the refusal — see core/venue.ContentModelEpoch.
		ContentModelEpoch   int  `json:"content_model_epoch"`
		AllowModelDowngrade bool `json:"allow_model_downgrade"`
	}
	if err := c.Bind(&req); err != nil {
		return apiErr(c, http.StatusBadRequest, err.Error())
	}
	// Always authorize and operate on the path-scoped project. The permission
	// middleware resolved access against c.Param("id"); a client-supplied
	// project_id is ignored to prevent cross-project access (IDOR).
	req.ProjectID = c.Param("id")
	// The path is authorized; a body-supplied stream never overrides it.
	req.Stream = refParam(c)

	if s.ContentStore == nil {
		return apiErr(c, http.StatusServiceUnavailable, "content store not configured")
	}

	diffEngine := bowsync.NewDiffEngine(s.ContentStore, s.SyncCache)

	// The model generation is checked before anything else is computed: a push
	// that would flatten what a richer kapi wrote is refused here, so the
	// refusal costs no upload and no diff.
	if !req.AllowModelDowngrade {
		if err := diffEngine.CheckContentModelEpoch(c.Request().Context(),
			req.ProjectID, req.Stream, req.ContentModelEpoch); err != nil {
			if conflict, ok := errors.AsType[*bowsync.ContentModelConflict](err); ok {
				return apiErr(c, http.StatusConflict, conflict.Error())
			}
			return serverErr(c, err)
		}
	}

	// The context content type negotiates first, because its answer qualifies
	// the content fast path below: a push whose blocks are all unchanged but
	// whose recipe declares a new collection still has work to do.
	ctxDiff, err := diffEngine.CompareContext(c.Request().Context(), req.ProjectID, req.Stream, req.ContextHash, req.Collections)
	if err != nil {
		return serverErr(c, err)
	}

	// The freshness ref rides on the answer the push has already paid for, so
	// the commit that follows knows what to assert without a second round trip.
	// Best-effort: a ref that cannot be computed costs the push its
	// compare-and-swap, never its content.
	currentRef, refErr := bowsync.CurrentRef(c.Request().Context(),
		s.streamRefSource(), req.ProjectID, req.Stream)
	if refErr != nil {
		currentRef = ref.Ref{}
	}

	// How this venue takes chunks, decided once and stated here so the producer
	// knows before it chunks: the 2 MiB bound that shaped chunking is this
	// server's request limit, and nothing imposes it on a write that never
	// reaches this server.
	transport := s.chunkTransport(c.Request().Context())

	// Fast path: root hash comparison. Only "unchanged" when the declared
	// context matches too — otherwise the push proceeds carrying no chunks and
	// a manifest that is nothing but the context.
	if req.RootHash != "" && !ctxDiff.Changed {
		unchanged, err := diffEngine.CheckRootHash(c.Request().Context(), req.ProjectID, req.Stream, req.RootHash)
		if err == nil && unchanged {
			return c.JSON(http.StatusOK, map[string]any{
				"upload_id":              "",
				"status":                 "unchanged",
				"context_changed":        false,
				"undeclared_collections": ctxDiff.Undeclared,
				"transport":              transport,
				"ref":                    currentRef,
			})
		}
	}

	// Full diff: compare item hashes.
	itemDiff, err := diffEngine.CompareItems(c.Request().Context(), req.ProjectID, req.Stream, req.ItemHashes)
	if err != nil {
		return serverErr(c, err)
	}

	uploadID := id.New()

	return c.JSON(http.StatusOK, map[string]any{
		"upload_id":              uploadID,
		"status":                 "diff_computed",
		"changed_items":          itemDiff.ChangedItems,
		"new_items":              itemDiff.NewItems,
		"deleted_items":          itemDiff.DeletedItems,
		"unchanged_item_count":   itemDiff.UnchangedCount,
		"context_changed":        ctxDiff.Changed,
		"undeclared_collections": ctxDiff.Undeclared,
		"transport":              transport,
		"ref":                    currentRef,
	})
}

// HandleSyncPushCommit finalizes a push by validating the manifest and enqueuing the worker job.
// POST /sync/push/commit
func (s *Server) HandleSyncPushCommit(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageFiles); err != nil {
		return err
	}

	var manifest struct {
		UploadID      string          `json:"upload_id"`
		ProjectID     string          `json:"project_id"`
		Stream        string          `json:"stream"`
		Chunks        []chunkRef      `json:"chunks"`
		Items         json.RawMessage `json:"items"`
		ActorID       string          `json:"actor_id"`
		WorkspaceSlug string          `json:"workspace_slug"`
		ConnectorID   string          `json:"connector_id"`
		// Contexts is the context content type, passed through to the worker
		// verbatim like Items: this handler validates the transport, the worker
		// reconciles the content.
		Contexts json.RawMessage `json:"contexts"`
		// Decisions is the decisions content type, passed through the same way:
		// the worker upserts it into the unit_decisions ledger after the chunks.
		Decisions json.RawMessage `json:"decisions"`
		// ExpectedRef is the compare-and-swap assertion: the governance
		// components this push last observed. Checked here so a client waiting
		// on `kapi push` is told at once, and again in the worker, which is
		// where the write actually lands.
		ExpectedRef ref.Ref `json:"expected_ref"`
		// BlockPropertyKeys declares what this producer's readers record, so
		// the worker knows which stored properties this push is authoritative
		// about. Passed through verbatim like Items — see
		// core/venue.BlockPropertyKeys.
		BlockPropertyKeys []string `json:"block_property_keys"`
		// Scope is the set of paths this push is authoritative over, and Tree
		// is what the producer read within it — per item, the block keys it
		// holds and the content hash of each. Together they are how a push says
		// what is GONE: a deletion sends no block, so the payload cannot say
		// it, and absence is only an answer inside a declared scope. Passed
		// through verbatim like Items; see core/venue.Tree and core/venue.Scope.
		Scope venue.Scope `json:"scope"`
		Tree  venue.Tree  `json:"tree"`
		// ContentModelEpoch is the generation this push wrote, recorded on the
		// stream now that it has been accepted.
		ContentModelEpoch int `json:"content_model_epoch"`
	}
	if err := c.Bind(&manifest); err != nil {
		return apiErr(c, http.StatusBadRequest, err.Error())
	}
	// Force the project and actor to the authorized path/identity. The
	// permission middleware resolved access against c.Param("id") and the
	// authenticated user; a client-supplied project_id / actor_id / workspace
	// is not trusted (prevents writing into another tenant's project, IDOR).
	manifest.ProjectID = c.Param("id")
	// The path is authorized; a manifest-supplied stream never overrides it.
	manifest.Stream = refParam(c)
	manifest.ActorID, _ = c.Get("user_id").(string)
	// Resolve the workspace slug from the path-scoped project rather than
	// trusting the client. Best-effort: the worker tolerates an empty slug.
	manifest.WorkspaceSlug = ""
	if s.ContentStore != nil {
		if proj, err := s.ContentStore.GetProject(c.Request().Context(), manifest.ProjectID); err == nil && proj != nil && proj.WorkspaceID != "" {
			if s.AuthStore != nil {
				if ws, err := s.AuthStore.GetWorkspace(c.Request().Context(), proj.WorkspaceID); err == nil && ws != nil {
					manifest.WorkspaceSlug = ws.Slug
				}
			}
		}
	}

	// The compare-and-swap, before anything is stored or queued.
	//
	// Only the components this manifest WRITES are asserted. A push carrying
	// blocks and nothing else asserts nothing, so content traffic cannot be
	// refused because governance moved beside it — which is the whole reason the
	// ref has components rather than one number.
	if err := s.assertGovernance(c.Request().Context(), manifest.ProjectID, manifest.Stream,
		manifest.ExpectedRef,
		writtenComponents(carriesRecords(manifest.Contexts), carriesRecords(manifest.Decisions))...); err != nil {
		if resp, ok := governanceConflict(c, err); ok {
			return resp
		}
		return serverErr(c, err)
	}

	// Enforce upload budget. Cheap and manifest-local, so it runs before the
	// per-chunk storage probes below — an oversized manifest is rejected without
	// paying for a stat per chunk.
	maxPushBytes := s.Config.MaxPushBytes
	if maxPushBytes <= 0 {
		maxPushBytes = 256 * 1024 * 1024 // default 256MB
	}
	var totalBytes int64
	for _, chunk := range manifest.Chunks {
		totalBytes += chunk.ByteSize
	}
	if totalBytes > maxPushBytes {
		return apiErr(c, http.StatusRequestEntityTooLarge, fmt.Sprintf("upload budget exceeded: %d bytes > %d bytes max", totalBytes, maxPushBytes))
	}

	// Every chunk the manifest names must already be a blob this server stored.
	//
	// This check used to be skipped whenever the blob store implemented
	// ChunkedBlobStore, on the theory that such a store keeps chunks in an
	// upload session rather than in content-addressed storage. That is not how
	// either backend behaves: HandleSyncProxyChunkUpload stores every proxied
	// chunk through BlobStore.Upload, which is content-addressed by
	// construction, and the ChunkedBlobStore staging methods have no caller.
	// The exemption therefore only ever applied to the self-hosted local store
	// — the one deployment where it removed the sole check standing between a
	// client-supplied hash and the worker that resolves it.
	for _, chunk := range manifest.Chunks {
		exists, err := s.BlobStore.Exists(c.Request().Context(), chunk.Hash)
		if err != nil || !exists {
			return apiErr(c, http.StatusBadRequest, fmt.Sprintf("chunk %d (hash %s) not found in storage", chunk.Index, chunk.Hash))
		}
	}

	pushID := id.New()

	// Serialize manifest for the worker.
	manifestJSON, _ := json.Marshal(manifest)

	// Store manifest as a blob for the worker to read.
	ref, err := s.BlobStore.Upload(c.Request().Context(), manifestJSON, storage.UploadOptions{
		ContentType: "application/json",
		Filename:    fmt.Sprintf("manifest-%s.json", pushID),
	})
	if err != nil {
		return apiErr(c, http.StatusInternalServerError, "failed to store manifest")
	}

	// Enqueue the push job.
	if s.JobStore != nil && s.JobQueue != nil {
		job := &jobs.TranslationJob{
			ID:            pushID,
			WorkspaceSlug: manifest.WorkspaceSlug,
			ProjectID:     manifest.ProjectID,
			ItemName:      "__sync_push__",
			Stream:        manifest.Stream,
			TargetLocale:  manifest.Stream, // display only; the worker reads the manifest
			Model:         ref.Key,         // manifest blob key
			PushID:        pushID,
			Status:        jobs.StatusQueued,
			// The manifest already names the actor for the push-completed
			// event; the row needs it too, so a push that fails reaches the
			// person whose `kapi push` is waiting on it.
			CreatedBy: manifest.ActorID,
		}
		if err := s.JobStore.CreateJob(c.Request().Context(), job); err != nil {
			return apiErr(c, http.StatusInternalServerError, "failed to create push job")
		}
		if err := s.JobQueue.Enqueue(c.Request().Context(), pushID); err != nil {
			_ = s.JobStore.DeleteJob(c.Request().Context(), pushID)
			return apiErr(c, http.StatusInternalServerError, "failed to enqueue push job")
		}
	}

	// Raise the stream's model generation to the one this push carried, now
	// that it is accepted. On commit rather than at init: a push that
	// negotiated and then failed must not close a stream to producers whose
	// content it never delivered. Best-effort — the next push records it.
	if manifest.ContentModelEpoch > 0 {
		_ = bowsync.NewDiffEngine(s.ContentStore, s.SyncCache).RecordContentModelEpoch(
			c.Request().Context(), manifest.ProjectID, manifest.Stream, manifest.ContentModelEpoch)
	}

	resp := map[string]any{
		"push_id": pushID,
		"status":  "queued",
	}
	// What this request can already say the platform will not accept.
	if precheck := s.precheckPushVerdicts(c, manifest.Decisions); !precheck.Empty() {
		resp["governance"] = precheck
	}
	return c.JSON(http.StatusAccepted, resp)
}

// precheckPushVerdicts answers, in the request, the half of the review gate a
// request can answer: whether the pusher holds review permission for each
// language whose verdicts this push carries.
//
// The worker checks it again, and checks the workspace separation-of-duties
// policy besides, because the worker is where the write actually lands. This
// one exists so a waiting `kapi push` is told at once rather than after a
// queue, the same "checked here, and again in the worker" the expected-ref
// assertion uses.
//
// A missing permission is a refusal of those verdicts, reported in the
// response. It is not a 403 for the push: the content is not in question, and
// refusing to store it would lose work over a permission the pusher can be
// granted afterwards.
func (s *Server) precheckPushVerdicts(c echo.Context, raw json.RawMessage) venue.PushGovernance {
	if len(raw) == 0 {
		return venue.PushGovernance{}
	}
	var decisions []venue.UnitDecision
	if err := json.Unmarshal(raw, &decisions); err != nil {
		return venue.PushGovernance{} // the worker fails a payload it cannot read
	}
	counts := map[[3]string]int{}
	for _, d := range decisions {
		if !d.CarriesVerdict() {
			continue
		}
		locale := decisionLocale(d)
		if locale != "" && allowsLanguage(c, platauth.PermReview, locale) {
			continue
		}
		if locale == "" {
			locale = d.Variant
		}
		counts[[3]string{locale, d.VerdictKind(), venue.RefusedNoReviewPermission}]++
	}
	if len(counts) == 0 {
		return venue.PushGovernance{}
	}
	refusals := make([]venue.DecisionRefusal, 0, len(counts))
	for key, count := range counts {
		refusals = append(refusals, venue.DecisionRefusal{
			Locale: key[0], Kind: key[1], Reason: key[2], Count: count,
		})
	}
	slices.SortFunc(refusals, func(a, b venue.DecisionRefusal) int {
		if a.Locale != b.Locale {
			return strings.Compare(a.Locale, b.Locale)
		}
		return strings.Compare(a.Kind, b.Kind)
	})
	return venue.PushGovernance{Refusals: refusals}
}

// decisionLocale reads the language out of a decision's variant, or empty when
// the variant is not one this venue can read.
func decisionLocale(d venue.UnitDecision) string {
	var key model.VariantKey
	if err := key.UnmarshalText([]byte(d.Variant)); err != nil {
		return ""
	}
	return string(key.Locale)
}

// pushGovernanceFor reads back what a push's review gate refused, as the worker
// recorded it on the job row. Best-effort: a report that cannot be read leaves
// the status response as it was rather than failing it.
func (s *Server) pushGovernanceFor(ctx context.Context, pushID string) *venue.PushGovernance {
	raw, err := s.JobStore.PushGovernance(ctx, pushID)
	if err != nil || raw == "" {
		return nil
	}
	var report venue.PushGovernance
	if json.Unmarshal([]byte(raw), &report) != nil || report.Empty() {
		return nil
	}
	return &report
}

// HandleSyncProxyChunkUpload handles chunk uploads for the proxy transport mode
// (local dev / self-hosted without Azure Blob SAS URLs).
// PUT /sync/push/chunks/:uploadId/:chunkIndex
func (s *Server) HandleSyncProxyChunkUpload(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageFiles); err != nil {
		return err
	}

	data, err := readBody(c, 2*1024*1024) // 2MB max per chunk
	if err != nil {
		return apiErr(c, http.StatusRequestEntityTooLarge, "chunk too large")
	}

	// Store chunk as a content-addressed blob. The worker later downloads each
	// chunk by its hash (from the commit manifest), so we need the chunk to be
	// accessible via BlobStore.Download(hash). Content-addressed Upload gives
	// us a stable key that matches the SHA-256 the client computes.
	//
	// Which is why :uploadId and :chunkIndex are not read here. They used to be
	// parsed into an UploadOptions.Filename — but the key is the content hash,
	// and neither blob store reads Filename at all, so the parse fed nothing.
	// An unparseable :chunkIndex silently became 0 and changed no outcome; a
	// 400 for one would reject a request that is, as far as this endpoint is
	// concerned, perfectly well formed.
	if _, err := s.BlobStore.Upload(c.Request().Context(), data, storage.UploadOptions{
		ContentType: "application/octet-stream",
	}); err != nil {
		return serverErr(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{"ok": true})
}

type chunkRef struct {
	Index       int    `json:"index"`
	ContentType string `json:"content_type"`
	Hash        string `json:"hash"`
	RecordCount int    `json:"record_count"`
	ByteSize    int64  `json:"byte_size"`
}

// readBody reads up to maxBytes from the request body.
func readBody(c echo.Context, maxBytes int64) ([]byte, error) {
	return io.ReadAll(io.LimitReader(c.Request().Body, maxBytes))
}

// HandleSyncPull returns full blocks, terms, and media for a project since the
// given cursor. The response is a RichPullResponse (Bowrain AD-009 Phase 7) with structured
// SyncBlock records instead of raw change log entries. When the client sends
// Accept-Encoding: zstd, the response is zstd-compressed.
func (s *Server) HandleSyncPull(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	if s.Services == nil {
		return apiErr(c, http.StatusServiceUnavailable, "store not configured")
	}

	ctx := c.Request().Context()
	projectID := c.Param("id")
	cursor, _ := strconv.ParseInt(c.QueryParam("cursor"), 10, 64)
	limit, _ := pageParams(c, 1000, store.DefaultBlockLimit)

	stream := refParam(c)

	var locales []string
	if raw := c.QueryParam("locales"); raw != "" {
		for l := range strings.SplitSeq(raw, ",") {
			if t := strings.TrimSpace(l); t != "" {
				locales = append(locales, t)
			}
		}
	}

	// Get change log entries to determine what changed.
	cs, err := s.Services.Project.GetChanges(ctx, projectID, stream, cursor, locales, limit)
	if err != nil {
		return serverErr(c, err)
	}

	resp := apiclient.RichPullResponse{
		Cursor:   cs.NewCursor,
		HasMore:  cs.HasMore,
		Contexts: s.pullContextEntries(ctx, projectID, stream),
	}

	// The freshness ref rides on the LAST page only. A client keeps the last
	// page's ref and discards every earlier one, so computing it per page would
	// be work whose answer is thrown away — and a pull is a client's cheapest
	// contact with the server precisely because it stays cheap.
	if !resp.HasMore {
		if current, rerr := bowsync.CurrentRef(ctx, s.streamRefSource(), projectID, stream); rerr == nil {
			resp.Ref = &current
		}
	}

	// The decision ledger travels like the declared context: small, always
	// current, on every page rather than cursor-driven. A store without the
	// ledger capability simply sends none.
	if ds, ok := s.ContentStore.(store.DecisionStore); ok {
		if decisions, derr := ds.ListUnitDecisions(ctx, projectID, stream); derr == nil {
			resp.Decisions = decisions
		}
	}

	if len(cs.Changes) > 0 {
		// Collect unique block IDs from the change log.
		blockIDSet := make(map[string]struct{})
		itemSet := make(map[string]struct{})
		for _, ch := range cs.Changes {
			if ch.ChangeType != "source_removed" {
				blockIDSet[ch.BlockID] = struct{}{}
			}
		}

		if len(blockIDSet) > 0 {
			blockIDs := make([]string, 0, len(blockIDSet))
			for id := range blockIDSet {
				blockIDs = append(blockIDs, id)
			}

			// Fetch full blocks from the store.
			query := store.BlockQuery{
				ProjectID: projectID,
				Stream:    stream,
				IDs:       blockIDs,
				Limit:     len(blockIDs),
			}
			stored, err := s.Services.Project.GetBlocks(ctx, query)
			if err != nil {
				return serverErr(c, err)
			}

			resp.Blocks = make([]apiclient.SyncBlock, 0, len(stored))
			for _, sb := range stored {
				resp.Blocks = append(resp.Blocks, apiclient.StoredBlockToSyncBlock(sb))
				itemSet[sb.ItemName] = struct{}{}
			}
		}

		// Fetch media assets for affected items.
		if s.ContentStore != nil && len(itemSet) > 0 {
			for itemName := range itemSet {
				assets, err := s.ContentStore.ListAssets(ctx, projectID, stream, itemName)
				if err != nil {
					continue // best-effort: skip media on error
				}
				for _, a := range assets {
					resp.Media = append(resp.Media, apiclient.AssetToSyncMedia(a))
				}
			}
		}
	}

	// Fire-and-forget analytics: only pulls that actually returned changed
	// content emit, so sync polling with no changes stays silent. Counts are
	// bucketed; never content.
	if s.PostHogClient != nil && len(resp.Blocks) > 0 {
		userID, _ := c.Get("user_id").(string)
		props := analytics.Props("", projectID)
		if wsID, ok := c.Get("workspace_id").(string); ok && wsID != "" {
			props[analytics.PropWorkspaceID] = wsID
		}
		props["block_count_bucket"] = analytics.CountBucket(len(resp.Blocks))
		props["has_more"] = resp.HasMore
		s.trackEvent(userID, analytics.EventContentPulled, props)
	}

	return writePullResponse(c, resp)
}

// pullContextEntries renders the project's collections as the pull's context
// content type: which collections exist, the point each occupies, who governs
// it, and — decisively — which side owns it.
//
// Every collection is carried, workspace-owned and recipe-owned alike.
// Ownership is what the client keys its own decisions on, so withholding the
// rows it may not act on would leave it unable to tell "not mine to touch" from
// "not there at all".
//
// The voice profile travels as its NAME, resolved from the bound id: a name is
// what a recipe and a brand hub agree on, while an id is one instance's
// bookkeeping. Best-effort throughout — a collection listing that fails, or a
// profile id that no longer resolves, costs the pull its context rather than
// its content.
func (s *Server) pullContextEntries(ctx context.Context, projectID, stream string) []*pb.SyncContextEntry {
	if s.ContentStore == nil {
		return nil
	}
	collections, err := s.ContentStore.ListCollections(ctx, projectID, stream)
	if err != nil || len(collections) == 0 {
		return nil
	}

	profileNames := map[string]string{}
	entries := make([]*pb.SyncContextEntry, 0, len(collections))
	for _, col := range collections {
		if col == nil {
			continue
		}
		e := &pb.SyncContextEntry{
			Name:        col.Name,
			Coordinates: col.Context,
			Channel:     col.ConnectorConfig[coreprofile.PropertyChannel],
			Owner:       venue.NormalizeContextOwner(col.Owner),
			ContentHash: col.ContextHash,
		}
		if col.PreviewKind != "" || col.PreviewURL != "" {
			e.Preview = &pb.SyncPreviewSource{Kind: col.PreviewKind, Url: col.PreviewURL}
		}
		if pid := col.ConnectorConfig[coreprofile.PropertyProfileID]; pid != "" {
			name, cached := profileNames[pid]
			if !cached {
				if s.VoiceStore != nil {
					if p, perr := s.VoiceStore.GetProfile(ctx, pid); perr == nil && p != nil {
						name = p.Name
					}
				}
				profileNames[pid] = name
			}
			e.VoiceProfile = name
		}
		entries = append(entries, e)
	}
	slices.SortFunc(entries, func(a, b *pb.SyncContextEntry) int {
		return strings.Compare(a.Name, b.Name)
	})
	return entries
}

// carriesRecords reports whether a manifest's raw payload holds at least one
// record.
//
// The length of the raw message will not do: a JSON `null` is four bytes and an
// empty array is two, so measuring the bytes makes a push that writes no
// governance look like one that does — and then refuses it because governance
// moved somewhere it never touched.
func carriesRecords(raw json.RawMessage) bool {
	var records []json.RawMessage
	if err := json.Unmarshal(raw, &records); err != nil {
		return false
	}
	return len(records) > 0
}

// writtenComponents names the governance components a push commit writes, and
// so the only ones its compare-and-swap may assert.
func writtenComponents(contexts, decisions bool) []ref.Component {
	var out []ref.Component
	if contexts {
		out = append(out, ref.ComponentContext)
	}
	if decisions {
		out = append(out, ref.ComponentDecisions)
	}
	return out
}

// syncCompressorPool is a lazily-initialized zstd compression pool for pull responses.
var syncCompressorPool = compression.NewPool(nil)

// writePullResponse marshals the response as JSON and optionally compresses with zstd.
func writePullResponse(c echo.Context, resp apiclient.RichPullResponse) error {
	data, err := json.Marshal(resp)
	if err != nil {
		return apiErr(c, http.StatusInternalServerError, "marshal response")
	}

	// Compress with zstd if the client accepts it.
	if strings.Contains(c.Request().Header.Get("Accept-Encoding"), "zstd") {
		compressed, err := syncCompressorPool.Compress(data)
		if err != nil {
			return apiErr(c, http.StatusInternalServerError, "compress response")
		}
		c.Response().Header().Set("Content-Encoding", "zstd")
		c.Response().Header().Set("Content-Type", "application/json")
		return c.Blob(http.StatusOK, "application/json", compressed)
	}

	return c.JSONBlob(http.StatusOK, data)
}

// HandleSyncGetBlocks returns blocks with full structured content for a specific item.
// Returns []SyncBlock with segments, spans, annotations, and metadata.
func (s *Server) HandleSyncGetBlocks(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	if s.Services == nil {
		return apiErr(c, http.StatusServiceUnavailable, "store not configured")
	}

	projectID := c.Param("id")
	itemName := c.QueryParam("item_name")

	stream := refParam(c)

	limit := 1000
	if l := c.QueryParam("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if limit > store.DefaultBlockLimit {
		limit = store.DefaultBlockLimit
	}
	offset := 0
	if o := c.QueryParam("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	query := store.BlockQuery{
		ProjectID: projectID,
		Stream:    stream,
		ItemName:  itemName,
		Limit:     limit,
		Offset:    offset,
	}

	blocks, err := s.Services.Project.GetBlocks(c.Request().Context(), query)
	if err != nil {
		return serverErr(c, err)
	}

	result := make([]apiclient.SyncBlock, len(blocks))
	for i, sb := range blocks {
		result[i] = apiclient.StoredBlockToSyncBlock(sb)
	}

	return c.JSON(http.StatusOK, result)
}

// HandleSyncPushStatus returns the aggregated status of jobs triggered by a push.
// GET /api/v1/projects/:id/sync/status?push_id=xxx
func (s *Server) HandleSyncPushStatus(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	if s.JobStore == nil {
		return apiErr(c, http.StatusServiceUnavailable, "job system not configured")
	}

	pushID := c.QueryParam("push_id")
	if pushID == "" {
		return apiErr(c, http.StatusBadRequest, "push_id is required")
	}

	jobList, err := s.JobStore.ListJobsByPushID(c.Request().Context(), pushID)
	if err != nil {
		return serverErr(c, err)
	}

	total := len(jobList)
	completed := 0
	failed := 0
	inProgress := 0

	for _, j := range jobList {
		switch j.Status {
		case jobs.StatusCompleted:
			completed++
		case jobs.StatusFailed:
			failed++
		case jobs.StatusProcessing, jobs.StatusQueued:
			inProgress++
		}
	}

	status := "completed"
	if inProgress > 0 {
		status = "in_progress"
	} else if failed > 0 && completed == 0 {
		status = "failed"
	}

	resp := map[string]any{
		"push_id":     pushID,
		"status":      status,
		"total":       total,
		"completed":   completed,
		"failed":      failed,
		"in_progress": inProgress,
	}
	// What the push's review gate did not accept. The producer polls this
	// endpoint until the ingest lands, so it is where a refusal decided in
	// the worker reaches the person who ran `kapi push`.
	if report := s.pushGovernanceFor(c.Request().Context(), pushID); report != nil {
		resp["governance"] = report
	}
	return c.JSON(http.StatusOK, resp)
}
