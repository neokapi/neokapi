// Package connector provides the bowrain Source connector implementation
// (push/pull/status against a Bowrain server). It is the heavy in-process
// integration layer that turns local files into synced server state.
package source

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/editor"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	coreproj "github.com/neokapi/neokapi/core/project"
	pb "github.com/neokapi/neokapi/core/proto/sync/v1"
	"github.com/neokapi/neokapi/core/ref"
	"github.com/neokapi/neokapi/core/ref/refcache"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/venue"
	bowrainconn "github.com/neokapi/neokapi/core/venue/connector"
	"github.com/neokapi/neokapi/host"
	apiclient "github.com/neokapi/neokapi/host/venue/client"
	"github.com/neokapi/neokapi/host/venue/config"
	bproject "github.com/neokapi/neokapi/host/venue/project"
	"github.com/neokapi/neokapi/host/venue/schema"
)

// Project is re-exported so callers can keep typing connector.Project.
// In practice everyone constructs their own *bproject.Project and passes
// it through.
type Project = bproject.Project

// Recipe is re-exported for symmetry.
type Recipe = bproject.Recipe

// SyncCache is re-exported so this package can talk about the cache
// owned by host/venue/project.
type SyncCache = bproject.SyncCache

// FileCache is re-exported.
type FileCache = bproject.FileCache

// CachedProjectMeta is re-exported.
type CachedProjectMeta = bproject.CachedProjectMeta

// LoadSyncCache is re-exported.
var LoadSyncCache = bproject.LoadSyncCache

// ResolveStream is re-exported so connector code can stay
// self-contained.
var ResolveStream = bproject.ResolveStream

// formatNameOf returns the format ID from a FormatSpec, or "" when unset.
func formatNameOf(spec *coreproj.FormatSpec) string {
	if spec == nil {
		return ""
	}
	return spec.Name
}

// streamFor returns the recipe's configured stream name (bowrain.stream),
// or "" when the recipe has no bowrain block.
func streamFor(recipe *Recipe) string {
	if recipe == nil || recipe.Server == nil {
		return ""
	}
	return recipe.Server.Stream
}

// BowrainSourceConnector implements bowrainconn.SourceConnector for local bowrain-cli projects.
// It communicates with a Bowrain server via REST API.
type BowrainSourceConnector struct {
	// app is the host whose project stores this connector reads and writes
	// through. Decisions live in the working-set schema of the project's one
	// store, and the App is what memoizes a single handle for it — so a
	// connector must be handed the App the surrounding process already uses,
	// never open a store of its own. Two pools on one file cannot serialize
	// their writers in process.
	app *host.App

	project   *Project
	client    *apiclient.BowrainClient
	formatReg *registry.FormatRegistry
	cache     *SyncCache
	stream    string // resolved stream name
	maxBatch  int    // Max blocks per push request

	// refs is the project's freshness layer: per stream, the position this
	// project has consumed and the governance identities its last contact with
	// the server established. Every "is this worth sending?" and every
	// compare-and-swap assertion reads it, and nothing else records those
	// facts — the sync cache holds block hashes and claim tokens, and it used
	// to hold three uncoordinated fragments of this one.
	refs *refcache.Cache

	// pushContext is the context content type this push carries: the
	// collections the recipe declares, with their coordinates and resolved
	// governance. Set by SetPushContext before Push; nil means the caller is
	// pushing content without making any claim about the declared context.
	pushContext *apiclient.PushContext
}

// itemBlock associates a block with its source item name.
type itemBlock struct {
	itemName string
	block    *model.Block
}

// NewSourceConnector creates a SourceConnector for the given project.
//
// app is the host the caller already runs under — a command's App, the daemon's
// one App per process. It is how the connector reaches the project's store for
// decision staging, and taking it as a parameter rather than opening one is the
// whole point: the App memoizes one handle per project root, and a connector
// that opened its own would be a second connection pool on `.kapi/work/store.db`.
func NewSourceConnector(app *host.App, project *Project, formatReg *registry.FormatRegistry) (*BowrainSourceConnector, error) {
	recipe := project.Recipe
	if !recipe.HasServer() {
		return nil, fmt.Errorf("no server configuration in the kapi recipe (add a `%s:` block)", schema.VenueKey)
	}

	serverURL := config.NormalizeServerURL(recipe.Server.ServerURL())
	projectID := recipe.Server.ProjectID()
	workspace := recipe.Server.Workspace()

	if serverURL == "" {
		return nil, fmt.Errorf("server URL not configured in the recipe's `%s:` block", schema.VenueKey)
	}
	if projectID == "" {
		return nil, fmt.Errorf("server project ID not configured in the recipe's `%s:` block", schema.VenueKey)
	}

	cache := LoadSyncCache(project.Layout)

	// The cache describes ONE server's view of this project: the block hashes
	// the server confirmed. Pointed at a different server or project —
	// switching between production and a local stack, or redirecting with
	// BOWRAIN_PROJECT_URL — none of that describes the new destination, and
	// reusing it makes the first push conclude nothing has changed and send
	// nothing at all.
	//
	// The cache already recorded which server it belonged to; nothing ever
	// compared it. Comparing it turns a silently empty push into a full one.
	if cache.ServerURL != "" &&
		(config.NormalizeServerURL(cache.ServerURL) != serverURL || cache.ProjectID != projectID) {
		cache = bproject.NewEmptySyncCache()
		cache.ServerURL = serverURL
		cache.ProjectID = projectID
	}

	// The refs are destination-keyed in their own right: Load discards a file
	// belonging to another server or project rather than reconciling it.
	refs := refcache.Load(project.Layout, serverURL, projectID)

	var client *apiclient.BowrainClient
	switch {
	case cache.ClaimToken != "":
		client = apiclient.NewClaimTokenClient(serverURL, projectID, cache.ClaimToken)
	case workspace != "":
		authInfo, err := config.LoadAuth()
		if err != nil {
			return nil, errors.New("workspace sync requires authentication: run 'kapi auth login'")
		}
		if authServer := config.NormalizeServerURL(authInfo.ServerURL); authServer != "" && authServer != serverURL {
			return nil, fmt.Errorf("auth token is for %s but project points to %s", authServer, serverURL)
		}
		client = apiclient.NewWorkspaceBowrainClient(serverURL, workspace, projectID, authInfo.AccessToken)
		if authInfo.RefreshToken != "" {
			client.SetRefreshToken(authInfo.RefreshToken, func(newAccess, newRefresh string) {
				authInfo.AccessToken = newAccess
				authInfo.RefreshToken = newRefresh
				_ = config.SaveAuth(*authInfo)
			})
		}
	default:
		// No workspace or claim token in config — try env-based auth (BOWRAIN_AUTH_TOKEN).
		// This supports CI scenarios where auth is provided via environment variable.
		authInfo, err := config.LoadAuth()
		if err != nil {
			return nil, errors.New("server config requires either workspace or claim_token, or set BOWRAIN_AUTH_TOKEN")
		}
		// Detect claim tokens (clm_ prefix) and route them correctly.
		if strings.HasPrefix(authInfo.AccessToken, "clm_") {
			client = apiclient.NewClaimTokenClient(serverURL, projectID, authInfo.AccessToken)
		} else {
			// Use flat project routes with bearer auth (works for API tokens and JWTs).
			client = apiclient.NewProjectBearerClient(serverURL, projectID, authInfo.AccessToken)
		}
	}

	// Resolve the active stream and set it on the client.
	stream := ResolveStream("", streamFor(recipe))
	client.SetStream(stream)

	return &BowrainSourceConnector{
		app:       app,
		project:   project,
		client:    client,
		formatReg: formatReg,
		cache:     cache,
		refs:      refs,
		stream:    stream,
		maxBatch:  1000,
	}, nil
}

// NewLocalConnector creates a BowrainSourceConnector for local-only operations
// (listing files, scanning blocks) without requiring a server connection. It
// takes the same App for the same reason: local-only is about the server, not
// about the project's store.
func NewLocalConnector(app *host.App, project *Project, formatReg *registry.FormatRegistry) *BowrainSourceConnector {
	cache := LoadSyncCache(project.Layout)
	return &BowrainSourceConnector{
		app:       app,
		project:   project,
		formatReg: formatReg,
		cache:     cache,
		refs:      &refcache.Cache{},
	}
}

// FileInfo describes a single tracked file with optional stats.
type FileInfo struct {
	Path       string // Relative path from project root
	Format     string // Detected format name
	BlockCount int    // Number of translatable blocks (-1 = not scanned)
	WordCount  int    // Total source word count (-1 = not scanned)
	DirtyCount int    // Blocks changed vs cache (-1 = not checked)
}

// ListFiles scans all tracked files and returns per-file stats.
// It uses scanLocalBlocks for block extraction and compares against the
// sync cache for dirty detection. Results are sorted by path.
func (c *BowrainSourceConnector) ListFiles(ctx context.Context, paths []string) ([]FileInfo, error) {
	_, blockMap, err := c.scanLocalBlocks(ctx, paths)
	if err != nil {
		return nil, err
	}

	var files []FileInfo
	for relPath, blocks := range blockMap {
		words := 0
		for _, b := range blocks {
			words += b.WordCount()
		}

		dirty := 0
		for _, b := range blocks {
			identity := model.ComputeIdentity(b)
			cached, found := c.lookupCachedHashForItem(relPath, convergence.BlockKey(b))
			if !found || cached != identity.RecordHash() {
				dirty++
			}
		}

		absPath := filepath.Join(c.project.Root, relPath)
		formatName := c.detectFormat(absPath)

		files = append(files, FileInfo{
			Path:       relPath,
			Format:     formatName,
			BlockCount: len(blocks),
			WordCount:  words,
			DirtyCount: dirty,
		})
	}

	slices.SortFunc(files, func(a, b FileInfo) int {
		return cmp.Compare(a.Path, b.Path)
	})
	return files, nil
}

// ID returns the connector identifier.
func (c *BowrainSourceConnector) ID() string {
	return "bowrain-source"
}

// Name returns a human-readable name.
func (c *BowrainSourceConnector) Name() string {
	return "Bowrain Local Source"
}

// Category returns the connector category.
func (c *BowrainSourceConnector) Category() bowrainconn.Category {
	return bowrainconn.CategoryFile
}

// Status reports the sync state.
func (c *BowrainSourceConnector) Status(ctx context.Context) (*bowrainconn.SyncStatus, error) {
	// Count local changes by scanning files and comparing to cache.
	hashMap, blockMap, err := c.scanLocalBlocks(ctx, nil)
	if err != nil {
		return &bowrainconn.SyncStatus{
			ConnectorID: c.ID(),
			Errors:      []string{err.Error()},
		}, nil
	}

	totalBlocks := 0
	pendingPush := 0
	for itemName, fileHashes := range hashMap {
		for blockID, hash := range fileHashes {
			totalBlocks++
			cachedHash, inCache := c.lookupCachedHashForItem(itemName, blockID)
			if !inCache || cachedHash != hash {
				pendingPush++
			}
		}
	}

	totalWords := 0
	for _, blocks := range blockMap {
		for _, b := range blocks {
			totalWords += b.WordCount()
		}
	}

	// Check remote changes by querying the server.
	pendingPull := 0
	if consumed := c.refs.Ref(c.stream).Content; consumed > 0 {
		resp, err := c.client.Pull(ctx, consumed, nil, 1)
		if err == nil && len(resp.Blocks) > 0 {
			pendingPull = len(resp.Blocks)
			if resp.HasMore {
				pendingPull = -1 // Unknown, but there are more
			}
		}
	}

	return &bowrainconn.SyncStatus{
		ConnectorID: c.ID(),
		LastSync:    c.cache.LastSync,
		ItemCount:   totalBlocks,
		FileCount:   len(hashMap),
		WordCount:   totalWords,
		PendingPush: pendingPush,
		PendingPull: pendingPull,
	}, nil
}

// BlockDelta describes how a single block has changed locally relative to the
// last-synced server state recorded in the sync cache.
type BlockDelta struct {
	// BlockID is the stable block identity within its file.
	BlockID string
	// Name is the keyed block name (JSON/YAML key, .strings key, …) or empty
	// for nameless prose blocks.
	Name string
	// Preview is a short excerpt of the block source text, for display.
	Preview string
	// Change is one of "added", "changed", or "removed".
	Change string
}

// FileDelta groups the local-vs-remote block changes for a single tracked file.
type FileDelta struct {
	Path    string       // path relative to project root
	Format  string       // detected format name
	Added   int          // blocks present locally but not in the sync cache
	Changed int          // blocks whose content hash differs from the cache
	Removed int          // blocks in the cache but no longer present locally
	Blocks  []BlockDelta // per-block detail, sorted by block ID
}

// Diff describes the overall local-vs-remote delta for a project.
type Diff struct {
	Files       []FileDelta // per-file changes, sorted by path (only files with changes)
	Added       int         // total blocks added locally
	Changed     int         // total blocks changed locally
	Removed     int         // total blocks removed locally
	PendingPull int         // remote changes available to pull (-1 = unknown, more available)
}

// HasChanges reports whether the diff carries any local or remote changes.
func (d *Diff) HasChanges() bool {
	return d.Added > 0 || d.Changed > 0 || d.Removed > 0 || d.PendingPull != 0
}

// Diff computes the per-file/per-block delta between the local files and the
// last-synced server state recorded in the sync cache. It reuses the same scan
// + cache-comparison machinery as Status, but reports detail (which blocks
// changed and how) rather than just counts.
//
// Local-vs-remote semantics, anchored on the sync cache:
//   - "added"   — block present locally, absent from the cache.
//   - "changed" — block present in both, content hash differs.
//   - "removed" — block present in the cache, absent locally.
//
// It also queries the server for the count of pending remote changes, mirroring
// Status, so the caller can surface "remote changes available to pull".
func (c *BowrainSourceConnector) Diff(ctx context.Context, paths []string) (*Diff, error) {
	hashMap, blockMap, err := c.scanLocalBlocks(ctx, paths)
	if err != nil {
		return nil, err
	}

	// Index blocks by item → blockID for name/preview lookups.
	metaByItem := map[string]map[string]blockMeta{}
	for itemName, blocks := range blockMap {
		m := map[string]blockMeta{}
		for _, b := range blocks {
			m[b.ID] = blockMeta{name: b.Name, preview: previewText(b.SourceText())}
		}
		metaByItem[itemName] = m
	}

	// Union of items seen locally and items present in the cache, so we can
	// detect removals (cache has the block, local scan does not).
	itemNames := map[string]struct{}{}
	for itemName := range hashMap {
		itemNames[itemName] = struct{}{}
	}
	for itemName := range c.cache.Files {
		itemNames[itemName] = struct{}{}
	}

	diff := &Diff{}
	for itemName := range itemNames {
		fileHashes := hashMap[itemName] // may be nil if the file is gone
		var cachedBlocks map[string]string
		if fc, ok := c.cache.Files[itemName]; ok {
			cachedBlocks = fc.Blocks
		}

		fd := FileDelta{
			Path:   itemName,
			Format: c.detectFormat(filepath.Join(c.project.Root, itemName)),
		}

		// Added / changed: walk local blocks against the cache.
		for blockID, hash := range fileHashes {
			cachedHash, inCache := cachedBlocks[blockID]
			if !inCache {
				fd.Added++
				fd.Blocks = append(fd.Blocks, blockDelta(metaByItem, itemName, blockID, "added"))
			} else if cachedHash != hash {
				fd.Changed++
				fd.Blocks = append(fd.Blocks, blockDelta(metaByItem, itemName, blockID, "changed"))
			}
		}

		// Removed: cached blocks no longer present locally.
		for blockID := range cachedBlocks {
			if _, ok := fileHashes[blockID]; !ok {
				fd.Removed++
				fd.Blocks = append(fd.Blocks, blockDelta(metaByItem, itemName, blockID, "removed"))
			}
		}

		if fd.Added == 0 && fd.Changed == 0 && fd.Removed == 0 {
			continue
		}

		slices.SortFunc(fd.Blocks, func(a, b BlockDelta) int {
			return cmp.Compare(a.BlockID, b.BlockID)
		})
		diff.Files = append(diff.Files, fd)
		diff.Added += fd.Added
		diff.Changed += fd.Changed
		diff.Removed += fd.Removed
	}

	slices.SortFunc(diff.Files, func(a, b FileDelta) int {
		return cmp.Compare(a.Path, b.Path)
	})

	// Query the server for pending remote changes, mirroring Status.
	if consumed := c.refs.Ref(c.stream).Content; c.client != nil && consumed > 0 {
		resp, err := c.client.Pull(ctx, consumed, nil, 1)
		if err == nil && len(resp.Blocks) > 0 {
			diff.PendingPull = len(resp.Blocks)
			if resp.HasMore {
				diff.PendingPull = -1
			}
		}
	}

	return diff, nil
}

// blockMeta carries the display name and source preview for a scanned block.
type blockMeta struct {
	name    string
	preview string
}

// blockDelta builds a BlockDelta for a block, pulling name/preview from the
// scanned metadata when available (removed blocks have no local metadata).
func blockDelta(metaByItem map[string]map[string]blockMeta, itemName, blockID, change string) BlockDelta {
	d := BlockDelta{BlockID: blockID, Change: change}
	if m, ok := metaByItem[itemName]; ok {
		if bm, ok := m[blockID]; ok {
			d.Name = bm.name
			d.Preview = bm.preview
		}
	}
	return d
}

// previewText trims and truncates block source text for compact display.
func previewText(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	const max = 60
	if len(s) > max {
		// Trim on a rune boundary.
		r := []rune(s)
		if len(r) > max {
			return string(r[:max]) + "…"
		}
	}
	return s
}

// Configure is a no-op for the Bowrain CLI source connector (configured via the kapi recipe).
func (c *BowrainSourceConnector) Configure(config map[string]string) error {
	return nil
}

// Close saves the sync cache and the freshness refs.
//
// Both, always: they record two halves of one contact with the server, and
// writing one without the other leaves a project claiming content the server
// holds at a position the server never issued.
func (c *BowrainSourceConnector) Close() error {
	if err := c.cache.Save(c.project.Layout); err != nil {
		return err
	}
	return c.refs.Save(c.project.Layout)
}

// SetConceptBaseline records the governed-terminology baseline a concept pull
// just wrote into the project's bound terms onto the connector's in-memory
// sync cache. The baseline is persisted by the single final Close() alongside
// the block-sync state (stream cursors, file hashes) the connector accumulated
// during the same pull — so a concept pull folded into a block pull cannot be
// silently overwritten by the connector's own deferred Close(). A later concept
// push reloads this baseline to diff local terms edits against it.
func (c *BowrainSourceConnector) SetConceptBaseline(b *bproject.ConceptBaseline) {
	c.cache.ConceptBaseline = b
}

// ObserveTermsRef records the terms component a terminology pull observed on the
// server, on the connector's in-memory refs — persisted by the same single
// Close() as the baseline it belongs with, for the same reason: two writers to
// one file mean the last one wins and the other's work is gone.
//
// An empty component is not recorded. A server too old to publish a ref says
// nothing about terminology, and storing that silence as "no terminology" would
// turn an old server into a permanent divergence report.
func (c *BowrainSourceConnector) ObserveTermsRef(component string) {
	if component == "" {
		return
	}
	c.refs.SetIdentity(c.stream, ref.ComponentTerms, component)
}

// Governance answers the status report's second axis: the governance
// identities this project last observed, and the ones the venue publishes now.
//
// The venue's half is best-effort and returned as a pointer for that reason. An
// unreachable server, an unclaimed project, a server too old for the endpoint —
// each leaves it nil, which is a report ("not checked") and not a comparison
// against nothing. Reading a missing answer as an empty ref would say every
// component had moved, on a laptop that simply has no network.
func (c *BowrainSourceConnector) Governance(ctx context.Context) (observed ref.Ref, venue *ref.Ref, observedAt time.Time) {
	observed = c.refs.Ref(c.stream)
	observedAt = c.refs.ObservedAt
	if c.client == nil {
		return observed, nil, observedAt
	}
	current, err := c.client.Ref(ctx)
	if err != nil {
		return observed, nil, observedAt
	}
	return observed, &current, observedAt
}

// SetPushContext records the context content type the next Push carries: the
// recipe's declared collections, their coordinates, and the governance resolved
// for each. It is set from the command layer rather than built here because
// resolving a collection's voice means loading a profile file or a starter
// pack, which is the host's resolution ladder, not the connector's.
//
// Leaving it unset pushes content only. That is not the same as pushing an
// empty context: a push that declares nothing would read as a recipe whose
// collections were all removed.
func (c *BowrainSourceConnector) SetPushContext(p *apiclient.PushContext) {
	c.pushContext = p
}

// PushContextChanged reports whether the declared context differs from the one
// the last push carried, as recorded in the sync cache. It is the local half of
// the context fast path: an unedited recipe is not worth a round trip, but a
// recipe that gained a collection or changed a voice must reach the server even
// when not one block of content moved.
func (c *BowrainSourceConnector) PushContextChanged() bool {
	if c.pushContext == nil {
		return false
	}
	return c.refs.Ref(c.stream).Context != c.pushContext.Hash
}

// Push sends source content from local files to Bowrain.
func (c *BowrainSourceConnector) Push(ctx context.Context, opts bowrainconn.PushOptions) (*bowrainconn.PushResult, error) {
	// Scan local files and extract blocks and media grouped by item.
	hashMap, blockMap, mediaHashMap, mediaMap, err := c.scanLocalBlocksAndMedia(ctx, opts.Paths)
	if err != nil {
		return nil, fmt.Errorf("scan local files: %w", err)
	}
	_, _ = mediaHashMap, mediaMap // used below after block push

	// What this push is authoritative over.
	scope := c.pushScope(opts.Paths)

	// Fetch the venue's tree, once. It answers two different questions, and
	// both used to be answered from local state instead.
	//
	// The cache is this producer's record of what it last sent, and it is not
	// committed: a fresh clone has none, and another machine may have pushed
	// since. Trusting it made a reset venue and a cold CI runner two different
	// kinds of wrong — one pushed nothing and reported success, the other
	// pushed everything. One fetch answers for both, and the ref it comes with
	// is what the commit asserts, so a tree that went stale between fetch and
	// push is refused rather than acted on.
	//
	// It is fetched even under --force. Force means "send everything regardless
	// of the diff", never "re-key everything": without priors, resolution below
	// would mint a fresh unit for every block and the declared tree would ask
	// the venue to drop every translation it holds.
	var serverTree *apiclient.TreeResponse
	if c.client != nil {
		if fetched, terr := c.client.Tree(ctx, scope); terr == nil {
			serverTree = fetched
		} else {
			slog.Warn("could not fetch the venue's tree; falling back to the local cache, so this push sends what this machine last recorded rather than what the venue lacks",
				"stream", c.stream, "error", terr)
		}
	}

	// Resolve each block's durable identity against what the venue holds.
	//
	// A reader names blocks by structure, and structure shifts: delete the
	// first paragraph of a section and every name below it is re-addressed, so
	// the venue would prune the last one's row — with its translation and its
	// approval — and land its text on its neighbour's. Resolution files content
	// under a key that is matched rather than named, so that stops happening.
	//
	// It runs BEFORE the declared tree is built, because the tree's keys are
	// what the venue prunes against, and after the fetch, because the priors
	// are the venue's. With no priors there is nothing to match against, and
	// resolution is skipped entirely rather than minting: an unresolved block
	// keys on its name, exactly as it did before any of this existed.
	if serverTree != nil {
		fetched := serverTree.Tree()
		host.ResolveIdentity(blockMap, host.Priors{
			Documents: fetched.Units(),
			Units:     fetched.Priors(),
		})
	}

	// What this scan read, keyed by the identity just resolved. Together with
	// the scope this is how a push says a string, or a whole file, is GONE — a
	// deletion sends no block, so a payload of changes cannot express one.
	localTree := venue.TreeFromBlocks(blockMap)

	var changed []itemBlock
	var held map[string]struct{}
	if serverTree != nil && !opts.Force {
		held = serverTree.Records()
	}
	for itemName, blocks := range blockMap {
		fileHashes := hashMap[itemName]
		for _, b := range blocks {
			key := convergence.BlockKey(b)
			switch {
			case opts.Force:
				changed = append(changed, itemBlock{itemName: itemName, block: b})
			case held != nil:
				// Content-addressed and global: a block that moved between
				// files is content the venue already holds, and asking per item
				// would upload it again under the new name.
				if _, have := held[model.ComputeIdentity(b).RecordHash()]; !have {
					changed = append(changed, itemBlock{itemName: itemName, block: b})
				}
			default:
				if cachedHash, inCache := c.lookupCachedHashForItem(itemName, key); !inCache || cachedHash != fileHashes[key] {
					changed = append(changed, itemBlock{itemName: itemName, block: b})
				}
			}
		}
	}

	pushWords := 0
	for _, ib := range changed {
		pushWords += ib.block.WordCount()
	}

	// The committed decision record travels with the content it judges. A
	// malformed record fails the push rather than being skipped: state is
	// authoritative, and a push that silently dropped it would be the exact
	// failure shape this protocol keeps re-learning to confess. Hashed against
	// the sync cache so a decision committed since the last push forces the
	// push past every "nothing changed" fast path — decisions only travel
	// when they changed, and a changed record cannot be silently skipped.
	decisions, derr := c.committedDecisions(ctx)
	if derr != nil {
		return nil, derr
	}
	decisionsHash := venue.DecisionsComponent(decisions)
	decisionsChanged := decisionsHash != c.refs.Ref(c.stream).Decisions
	if !decisionsChanged {
		decisions = nil // unchanged: the server already holds this record
	}

	if opts.DryRun {
		return &bowrainconn.PushResult{
			BlocksPushed: len(changed),
			FilesScanned: len(hashMap),
			WordCount:    pushWords,
		}, nil
	}

	// A recipe change moves no content and still has to reach the venue: a
	// collection added, a coordinate moved, a voice rebound, a decision
	// committed. Falling out on "no blocks changed" is what used to leave the
	// declared structure stranded on the developer's machine.
	//
	// A deletion is the one source change that sends nothing at all — the
	// string is simply absent from the scan — so the tree, not the payload, is
	// what carries it. A venue holding an item or a block this scan did not
	// produce, inside the scope this push declares, is exactly that case, and
	// comparing the two trees is how it is seen. A push whose tree fetch failed
	// cannot make that comparison and does not pretend to: it stays additive,
	// as it was before there were trees.
	if len(changed) == 0 && !venueHoldsMoreThan(serverTree, localTree, scope) &&
		!c.PushContextChanged() && !decisionsChanged {
		return &bowrainconn.PushResult{FilesScanned: len(hashMap)}, nil
	}

	// Generate per-item editor metadata (BlockIndex + PreviewHTML) for changed items.
	itemMeta := c.buildItemMeta(ctx, changed)

	// Group changed blocks by item for the push API.
	blocksByItem := map[string][]*model.Block{}
	for _, ib := range changed {
		blocksByItem[ib.itemName] = append(blocksByItem[ib.itemName], ib.block)
	}

	// Push via init → diff → chunk → commit flow, carrying the declared
	// context so the collections land in the same transaction as the items.
	// The compare-and-swap asserts the ref this project last observed, not one
	// fetched now: the payload was built by diffing against the observed ref,
	// so that is the state it is correct against.
	// Declared over EVERY block this scan read, not the changed ones: the
	// declaration says what this producer's readers record, and a push carrying
	// one edited string still knows what a note is. Scoping it to the payload
	// would make a one-block push claim ignorance of the rest of the file, and
	// the server would preserve values the source had genuinely dropped.
	pushOpts := []apiclient.PushOption{
		apiclient.AssertRef(c.refs.Ref(c.stream)),
		apiclient.DeclareBlockProperties(venue.BlockPropertyKeys(allScannedBlocks(blockMap))),
		apiclient.DeclareTree(scope, localTree),
	}
	// --force already means "send everything, ignore the cache". It also means
	// the one thing the server refuses on its own: writing content from an
	// older model than the stream holds. Deliberate is the whole difference
	// between a downgrade and an accident.
	if opts.Force {
		pushOpts = append(pushOpts, apiclient.AllowModelDowngrade())
	}
	resp, err := c.client.Push(ctx, blocksByItem, itemMeta, c.pushContext, decisions, pushOpts...)
	if err != nil {
		return nil, fmt.Errorf("push: %w", err)
	}
	totalStored := len(changed)
	var lastCursor int64
	pushID := ""
	var undeclared []string
	blocksUploaded := 0
	chunkCount := 0
	var served *ref.Ref
	if resp != nil {
		lastCursor = resp.NewCursor
		pushID = resp.PushID
		undeclared = resp.UndeclaredCollections
		blocksUploaded = resp.BlocksUploaded
		chunkCount = resp.ChunkCount
		served = resp.ServerRef
	}

	// Fetch and cache server metadata (best-effort).
	c.fetchAndCacheMetadata(ctx)

	// Push assets (Bowrain AD-007): upload changed media to blob storage.
	assets := &assetPushResult{}
	if c.client != nil && len(mediaMap) > 0 {
		assets = c.pushAssets(ctx, mediaHashMap, mediaMap, opts.Force)
	}

	// Confirm the server actually applied the push before recording that it
	// holds this content. The commit answers 202 and hands the work to a
	// worker; a failed ingest with the cache already written is content the
	// next push will conclude is present and never send again.
	ingest, governance, ierr := c.confirmIngest(ctx, pushID)
	if ierr != nil {
		return nil, ierr
	}
	// The venue's pre-check answers before the worker does, and says the
	// same thing about the permission half. Keep whichever answer arrived.
	if governance == nil && resp != nil {
		governance = resp.Governance
	}

	// The project's record follows the venue: a verdict it did not accept is
	// retired here, or every push from now on would send it again, be
	// refused again, and report it again.
	retired, rerr := c.retireRefusedVerdicts(ctx, governance)
	if rerr != nil {
		return nil, rerr
	}

	// Update cache with per-file hashes.
	for itemName, fileHashes := range hashMap {
		fc, ok := c.cache.Files[itemName]
		if !ok {
			fc = &FileCache{Blocks: map[string]string{}}
			c.cache.Files[itemName] = fc
		}
		maps.Copy(fc.Blocks, fileHashes)
	}
	// Update cache with per-file asset hashes — except for the assets that
	// failed to upload. Recording those would tell the next push they are
	// already on the server, and the retry it would otherwise perform is the
	// only thing standing between a transient blob-store error and an image
	// that is permanently absent from the translated page.
	for itemName, assetHashes := range mediaHashMap {
		fc, ok := c.cache.Files[itemName]
		if !ok {
			fc = &FileCache{Blocks: map[string]string{}, Assets: map[string]string{}}
			c.cache.Files[itemName] = fc
		}
		if fc.Assets == nil {
			fc.Assets = map[string]string{}
		}
		failedForItem := assets.FailedIDs[itemName]
		for assetID, blobKey := range assetHashes {
			if failedForItem[assetID] {
				continue
			}
			fc.Assets[assetID] = blobKey
		}
	}
	c.cache.LastSync = time.Now().UTC()
	c.cache.ServerURL = c.project.Recipe.Server.ServerURL()
	c.cache.ProjectID = c.project.Recipe.Server.ProjectID()

	c.refs.Consume(c.stream, lastCursor)
	c.recordGovernanceAfterPush(ctx, pushID, ingest, served)
	c.refs.Touch(time.Now())

	if err := c.cache.Save(c.project.Layout); err != nil {
		return nil, fmt.Errorf("save sync cache: %w", err)
	}
	if err := c.refs.Save(c.project.Layout); err != nil {
		return nil, fmt.Errorf("save freshness refs: %w", err)
	}

	return &bowrainconn.PushResult{
		BlocksPushed:          totalStored,
		BlocksUploaded:        blocksUploaded,
		ChunkCount:            chunkCount,
		AssetsPushed:          assets.Pushed,
		AssetsFailed:          assets.Failed,
		AssetErrors:           assets.Errors,
		FilesScanned:          len(hashMap),
		WordCount:             pushWords,
		PushID:                pushID,
		UndeclaredCollections: undeclared,
		Ingest:                ingest,
		Governance:            governance,
		VerdictsRetired:       retired,
	}, nil
}

// recordGovernanceAfterPush records where the governance components stand once
// a push has landed — read back from the server, never computed here.
//
// A client-computed value would be wrong in an ordinary case. The server keeps a
// collection the recipe no longer declares (report, never delete), and it keeps
// it recipe-owned, so the fold it publishes and the fold this client makes over
// what the recipe declares differ permanently. Caching this side's fold and then
// asserting it would refuse every later governance push with "governance moved"
// — over a collection nobody moved, on a project that could never recover,
// because the recipe is the authority and pulling would not remove it.
//
// When the ingest is not confirmed applied, or the ref cannot be read, the
// components this push carried are CLEARED rather than guessed. An empty
// component asserts nothing and reads as "worth sending again", so the next push
// re-sends an idempotent write and re-reads the answer. One redundant reconcile,
// never a false conflict.
//
// A push that committed NOTHING is the third case and takes neither branch: the
// negotiation's answer still describes the server, because this push did not
// move it. It is also where a project pushing before it has ever pulled learns
// the governance it never wrote.
func (c *BowrainSourceConnector) recordGovernanceAfterPush(ctx context.Context, pushID, ingest string, negotiated *ref.Ref) {
	if pushID == "" || pushID == apiclient.PushUnchanged {
		if negotiated != nil {
			c.refs.Observe(c.stream, *negotiated)
		}
		return
	}
	if c.client != nil && ingest == bowrainconn.IngestApplied {
		if current, err := c.client.Ref(ctx); err == nil {
			c.refs.Record(c.stream, current)
			return
		}
	}
	for _, component := range []ref.Component{ref.ComponentContext, ref.ComponentDecisions} {
		c.refs.SetIdentity(c.stream, component, "")
	}
}

// ingestConfirmWindow bounds how long a push waits for the server to say what
// it did with the upload. Kept short: the wait is a confirmation, not a
// dependency — an ingest still queued when it elapses is reported as queued,
// not treated as a failure.
const ingestConfirmWindow = 6 * time.Second

// confirmIngest asks the server what became of a committed push.
//
// The commit endpoint returns 202 Accepted with a push id and enqueues a worker
// job; every other content type in this push (blocks, context, decisions) is
// applied there, not in the request. So the 202 says "received", and only the
// job says "stored". A client that reported the 202 as success — as this one
// did — told the user their content had landed at the exact moment it might
// still be rejected for a bad chunk hash, an unsupported content type, or a
// conflicting block, and then wrote the sync cache saying the server holds it.
// The next push diffs against that cache, finds nothing to send, and the
// content is stranded until someone runs --force.
//
// The aggregate status is safe to read this way because the ingest job is the
// FIRST job created for a push id: convergence fans out translation jobs under
// the same id only after the push-completed event, which the ingest job emits
// on success. So `failed > 0 && completed == 0` is the ingest itself failing,
// and any completed job means the ingest landed.
//
// Being unable to ask is not a failure. A server with no job system, one too
// old for the endpoint, or a status call that errors leaves the push reported
// as unknown rather than refused — the alternative is refusing pushes that
// worked.
func (c *BowrainSourceConnector) confirmIngest(ctx context.Context, pushID string) (string, *venue.PushGovernance, error) {
	// The unchanged sentinel is the init fast path's: nothing was committed, so
	// there is no job and nothing to confirm.
	if c.client == nil || pushID == "" || pushID == apiclient.PushUnchanged {
		return "", nil, nil
	}

	deadline := time.Now().Add(ingestConfirmWindow)
	interval := 150 * time.Millisecond
	for {
		st, err := c.client.PushStatus(ctx, pushID)
		if err != nil {
			return bowrainconn.IngestUnknown, nil, nil //nolint:nilerr // an unaskable server is unknown, not failed
		}
		switch {
		case st.Failed > 0 && st.Completed == 0:
			return "", nil, fmt.Errorf(
				"push %s was accepted but the server failed to apply it (%d job(s) failed); "+
					"the local sync record was left untouched so the next push sends this content again",
				pushID, st.Failed)
		case st.Completed > 0:
			return bowrainconn.IngestApplied, st.Governance, nil
		case st.Total == 0:
			// No job exists for this push id. Either the deployment runs no
			// worker or the job has not been recorded yet; keep waiting, and
			// report it as unconfirmed if it never appears.
		}
		if time.Now().After(deadline) {
			if st.Total == 0 {
				return bowrainconn.IngestUnknown, st.Governance, nil
			}
			return bowrainconn.IngestQueued, st.Governance, nil
		}
		select {
		case <-ctx.Done():
			return bowrainconn.IngestUnknown, st.Governance, nil
		case <-time.After(interval):
		}
		if interval < time.Second {
			interval *= 2
		}
	}
}

// pushAssets uploads changed media assets to the server.
// For each asset: checks cache → requests upload URL → uploads blob → registers metadata.
//
// It returns what landed, what did not, and why. Each asset is best-effort —
// one refused blob must not abandon the blocks already stored — but a refusal
// must leave a trace. Counting successes only makes a push whose every image
// failed report the same "assets: 0" as a push with no images, and the reader
// of the translated page finds the pictures missing with nothing in the
// transcript to explain it.
func (c *BowrainSourceConnector) pushAssets(
	ctx context.Context,
	mediaHashMap map[string]map[string]string,
	mediaMap map[string][]*model.Media,
	force bool,
) *assetPushResult {
	res := &assetPushResult{FailedIDs: map[string]map[string]bool{}}
	// note records one asset failure, keeping a bounded sample of the reasons
	// and remembering which asset it was so the cache does not record it as
	// synced — a hash written for a blob that never arrived is the same silent
	// loss one level down: the next push finds it unchanged and never retries.
	note := func(itemName, assetID string, err error) {
		res.Failed++
		if res.FailedIDs[itemName] == nil {
			res.FailedIDs[itemName] = map[string]bool{}
		}
		res.FailedIDs[itemName][assetID] = true
		if len(res.Errors) < maxAssetErrorsReported {
			res.Errors = append(res.Errors, fmt.Sprintf("%s: %s: %v", itemName, assetID, err))
		}
	}
	for itemName, mediaList := range mediaMap {
		cachedAssets := map[string]string{}
		if fc, ok := c.cache.Files[itemName]; ok && fc.Assets != nil {
			cachedAssets = fc.Assets
		}

		for _, m := range mediaList {
			// Skip unchanged assets.
			if !force {
				if cachedKey, ok := cachedAssets[m.ID]; ok && cachedKey == m.BlobKey {
					continue
				}
			}

			// Request upload URL (dedup check).
			urlResp, err := c.client.GetAssetUploadURL(ctx, m.BlobKey, m.MimeType, m.Size)
			if err != nil {
				note(itemName, m.ID, fmt.Errorf("request upload URL: %w", err))
				continue // best-effort, per asset
			}

			// If blob doesn't exist yet and we have a SAS URL, upload directly.
			// For local backend (no SAS URL), the server proxies the upload
			// via the PushAsset metadata call — the blob was already uploaded
			// inline if the server supports it, or the upload-url response
			// indicates the blob exists (dedup).
			if !urlResp.Exists && urlResp.UploadURL != "" {
				// Direct upload to blob storage via SAS URL.
				if err := uploadToSASURL(ctx, urlResp.UploadURL, m.Data, m.MimeType); err != nil {
					note(itemName, m.ID, fmt.Errorf("upload blob: %w", err))
					continue // best-effort, per asset
				}
			}

			// Register asset metadata on the server.
			_, err = c.client.PushAsset(ctx, apiclient.AssetInput{
				BlobKey:    m.BlobKey,
				ItemName:   itemName,
				SourceID:   m.ID,
				MimeType:   m.MimeType,
				Filename:   m.Filename,
				SizeBytes:  m.Size,
				AltText:    m.AltText,
				Properties: m.Properties,
			})
			if err != nil {
				note(itemName, m.ID, fmt.Errorf("register asset: %w", err))
				continue // best-effort, per asset
			}
			res.Pushed++
		}
	}
	return res
}

// assetPushResult is what an asset push amounted to: how many blobs landed, how
// many were refused, a bounded sample of the reasons, and which assets failed
// per item so their hashes are kept out of the sync cache.
type assetPushResult struct {
	Pushed    int
	Failed    int
	Errors    []string
	FailedIDs map[string]map[string]bool // itemName → assetID → failed
}

// maxAssetErrorsReported bounds how many asset failures are named. The count is
// exact; the reasons are a sample, because a project whose blob store is down
// fails every asset and a wall of identical errors reports nothing the first
// three do not.
const maxAssetErrorsReported = 3

// uploadToSASURL uploads data directly to a pre-signed Azure SAS URL.
func uploadToSASURL(ctx context.Context, sasURL string, data []byte, contentType string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, sasURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("x-ms-blob-type", "BlockBlob")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("SAS upload failed: HTTP %d", resp.StatusCode)
	}
	return nil
}

// Pull retrieves translated content from Bowrain.
func (c *BowrainSourceConnector) Pull(ctx context.Context, opts bowrainconn.PullOptions) (*bowrainconn.PullResult, error) {
	// Refresh server metadata so we have up-to-date target locales.
	c.fetchAndCacheMetadata(ctx)

	// Recipe first, content second. A coordinate is minted by the recipe, so
	// content arriving at a point the recipe cannot yet account for is the same
	// divergence the venue refuses on the other side.
	for _, r := range c.applyPendingRecipeChanges(ctx) {
		switch {
		case r.Err != nil:
			slog.WarnContext(ctx, "a recipe change from the venue could not be applied",
				"path", r.Path, "error", r.Err)
		case r.Conflict != "":
			slog.WarnContext(ctx, "the recipe already says something else here; left as it is",
				"path", r.Path, "recipe", r.Conflict)
		case r.Applied:
			slog.InfoContext(ctx, "recipe updated from the venue; review it with git diff", "path", r.Path)
		}
	}

	// Resolve target locales: CLI args > config > server cache.
	pullLocales := opts.Locales
	if len(pullLocales) == 0 {
		pullLocales = c.project.Recipe.Defaults.TargetLanguages
	}
	if len(pullLocales) == 0 {
		pullLocales = c.ServerTargetLocales()
	}
	locales := make([]string, len(pullLocales))
	for i, l := range pullLocales {
		locales[i] = string(l)
	}

	cursor := c.refs.Ref(c.stream).Content
	if opts.Force {
		// A forced pull restarts the stream, and the recorded position must
		// restart with it: the position is monotonic, so leaving it standing
		// would let a server that was rebuilt below it answer every later pull
		// with nothing at all.
		cursor = 0
		c.refs.Reset(c.stream)
	}

	// Pull blocks directly from the server using the rich pull response.
	// Each response includes full SyncBlock records with structured segments.
	totalPulled := 0

	// Collect all blocks across paginated responses.
	var allBlocks []apiclient.SyncBlock
	var contexts []*pb.SyncContextEntry
	var decisions []venue.UnitDecision
	var served *ref.Ref

	// The assets each item holds, as the venue already listed them.
	//
	// A pull response has always carried these — the server lists them per
	// affected item while assembling the page — and the loop below used to drop
	// them on the floor. The write-out then asked for the same list again, once
	// per item and once per locale, over the network. Keeping what was already
	// sent is the whole of the saving: no wire shape changes, and nothing new is
	// computed on either side.
	mediaByItem := map[string][]apiclient.SyncMedia{}

	for {
		resp, err := c.client.Pull(ctx, cursor, locales, 1000)
		if err != nil {
			return nil, fmt.Errorf("pull: %w", err)
		}

		totalPulled += len(resp.Blocks)
		allBlocks = append(allBlocks, resp.Blocks...)
		// The declared context is not cursor-driven — every page carries the
		// current one — so the last page's is the one to keep. The decision
		// ledger and the freshness ref travel the same way.
		if len(resp.Contexts) > 0 {
			contexts = resp.Contexts
		}
		if len(resp.Decisions) > 0 {
			decisions = resp.Decisions
		}
		if resp.Ref != nil {
			served = resp.Ref
		}
		for _, m := range resp.Media {
			mediaByItem[m.ItemName] = append(mediaByItem[m.ItemName], m)
		}
		cursor = resp.Cursor

		if !resp.HasMore {
			break
		}
	}

	if opts.DryRun {
		return &bowrainconn.PullResult{
			BlocksPulled: totalPulled,
			LocalesCount: len(locales),
		}, nil
	}

	// Reconcile the server's decision ledger into the working store — staged,
	// not committed: `kapi commit` remains the only door into the git-tracked
	// record, so a pull can never publish on anyone's behalf. `kapi status`
	// reports what arrived and names the command that publishes it.
	decisionsStaged, decisionsSkipped, err := c.stagePulledDecisions(ctx, decisions)
	if err != nil {
		return nil, err
	}

	// Record what the server holds and report what diverges. Nothing here
	// changes the governance a local run resolves — see connector/context.go.
	contextResult := c.applyPulledContext(ctx, contexts)

	// Group pulled blocks by item name.
	filesWritten := 0
	itemsRetired := 0

	// Track per-file write failures. A failed write must NOT let the stream
	// cursor advance past the unwritten change: the server's GetChanges is
	// forward-only (seq > cursor), so a skipped translation would never be
	// re-delivered and would be permanently lost locally. We therefore collect
	// write errors and, if any occurred, abort without advancing/saving the
	// cursor so the next pull re-delivers everything (the partial successes
	// already written stay on disk and are simply overwritten next time).
	var writeErrs []error

	if len(allBlocks) > 0 && len(locales) > 0 {
		blocksByItem := map[string][]apiclient.SyncBlock{}
		for _, b := range allBlocks {
			blocksByItem[b.ItemName] = append(blocksByItem[b.ItemName], b)
		}

		for itemName, blocks := range blocksByItem {
			// Check if any blocks have targets for our locales.
			hasTargets := false
			for _, b := range blocks {
				if len(b.Targets) > 0 {
					hasTargets = true
					break
				}
			}
			if !hasTargets {
				continue
			}

			// An item's assets and their variants do not vary by target
			// language — the variant list carries every locale and the filter
			// below picks one — so both are read once, here, rather than once
			// per locale inside the loop.
			var itemAssets []apiclient.SyncMedia
			var itemVariants map[string][]apiclient.AssetVariantResponse
			if c.project.Recipe.AssetsEnabled() {
				itemAssets = mediaByItem[itemName]
				var listErrs []error
				itemVariants, listErrs = c.listMediaVariants(ctx, itemAssets)
				for _, err := range listErrs {
					writeErrs = append(writeErrs, fmt.Errorf("%s: %w", itemName, err))
				}
			}

			// Write a translated file for each target locale.
			for _, loc := range locales {
				// Build target map for this locale from structured segments.
				// Keyed by a stable match key (not the server block ID, which is
				// not preserved across push/pull) so writeTranslatedFile can match
				// the freshly re-parsed source blocks.
				targetMap := map[string]string{} // matchKey → translated text
				for _, b := range blocks {
					if segs, ok := b.Targets[loc]; ok {
						// Extract plain text from segments (flatten
						// TextRuns only — inline codes and structured
						// runs contribute nothing at export time).
						var textSb strings.Builder
						for _, seg := range segs {
							for _, r := range seg.Runs {
								if r.Text != nil {
									textSb.WriteString(r.Text.Text)
								}
							}
						}
						if text := textSb.String(); text != "" {
							targetMap[targetMatchKey(b.Name, b.SourceText)] = text
						}
					}
				}
				if len(targetMap) == 0 {
					continue
				}

				// Determine output path.
				outPath := c.resolveTargetPath(itemName, loc)
				absOut := c.project.ResolvePath(outPath)

				// Read source, inject targets, write output.
				absSource := c.project.ResolvePath(itemName)
				if _, statErr := os.Stat(absSource); errors.Is(statErr, fs.ErrNotExist) {
					// The source was deleted or renamed after it was pushed;
					// additive-only push means the server never forgets it.
					// There is nothing to reconstruct a target from — on this
					// pull or any retry — so holding the cursor would wedge
					// every later translation behind a file that is never
					// coming back. Skip it loudly and move on.
					itemsRetired++
					fmt.Fprintf(os.Stderr, "pull: skipping %s (%s): its source file is gone from this checkout, retired locally and still on the server\n", itemName, loc)
					continue
				}
				formatName := c.detectFormat(absSource)
				if formatName == "" {
					// The server has translations for an item this checkout
					// cannot read. Skipping was silent, and the cursor advanced
					// past them anyway — the server's stream is forward-only, so
					// those translations were never offered again and were gone.
					//
					// It is not a rare shape: a machine without the plugin that
					// provides the format (a colleague without okapi-bridge, a CI
					// image that ships fewer plugins) reads every Office or IDML
					// item as format-less and pulls "500 blocks, wrote 0 files"
					// with a clean exit. Joining the write failures makes it what
					// it is — a failure that holds the cursor, so the pull retries
					// once the format can be read.
					writeErrs = append(writeErrs, fmt.Errorf(
						"%s (%s): no format is registered for this file. The recipe names none and no plugin claims its extension, so its translations cannot be written",
						itemName, loc))
					continue
				}

				// Fetch locale-variant media for this item (Bowrain AD-007).
				var mediaRepl []MediaReplacement
				cleanupMedia := func() {}
				if len(itemAssets) > 0 {
					var mediaErrs []error
					mediaRepl, cleanupMedia, mediaErrs = c.fetchMediaReplacements(ctx, itemAssets, itemVariants, loc)
					for _, err := range mediaErrs {
						writeErrs = append(writeErrs, fmt.Errorf("%s (%s): %w", itemName, loc, err))
					}
				}

				werr := c.writeTranslatedFile(ctx, absSource, absOut, formatName, loc, targetMap, mediaRepl...)
				cleanupMedia()
				if werr != nil {
					writeErrs = append(writeErrs, fmt.Errorf("write %s (%s): %w", outPath, loc, werr))
					continue
				}
				filesWritten++
			}
		}
	}

	// If any target file failed to write, do NOT advance or save the cursor:
	// leaving it where it was means the next pull re-delivers these changes
	// (the server cursor is forward-only). Partial successes already written
	// to disk are preserved. We surface the failures so the CLI reports the
	// pull as failed rather than silently succeeding past lost translations.
	if len(writeErrs) > 0 {
		return nil, fmt.Errorf("pull: %d target file(s) failed to write; cursor not advanced so the next pull retries: %w",
			len(writeErrs), errors.Join(writeErrs...))
	}

	// Everything the pull brought down is on disk and in the store, so the
	// project's ref may move: the position to what it consumed, and the
	// governance identities to what the server published on the way. Recorded
	// together and only here, after the writes above succeeded — the server's
	// change feed is forward-only, so a ref advanced past an unwritten change
	// is a change nobody ever sees again.
	c.refs.Consume(c.stream, cursor)
	if served != nil {
		c.refs.Observe(c.stream, *served)
	}
	c.refs.Touch(time.Now())

	c.cache.LastSync = time.Now().UTC()
	if err := c.cache.Save(c.project.Layout); err != nil {
		return nil, fmt.Errorf("save sync cache: %w", err)
	}
	if err := c.refs.Save(c.project.Layout); err != nil {
		return nil, fmt.Errorf("save freshness refs: %w", err)
	}

	return &bowrainconn.PullResult{
		BlocksPulled:         totalPulled,
		LocalesCount:         len(locales),
		FilesWritten:         filesWritten,
		ItemsRetired:         itemsRetired,
		DecisionsStaged:      decisionsStaged,
		DecisionsSkipped:     decisionsSkipped,
		CollectionsObserved:  contextResult.Observed,
		GovernanceDiverged:   contextResult.Diverged,
		GovernanceDivergence: contextResult.DivergedDetail,
	}, nil
}

// scanLocalBlocks walks local source files, reads them with format readers,
// and extracts blocks grouped by item (file path relative to project root).
// Returns itemName→(blockID→hash) and itemName→blocks.
func (c *BowrainSourceConnector) scanLocalBlocks(ctx context.Context, paths []string) (map[string]map[string]string, map[string][]*model.Block, error) {
	hashMap, blockMap, _, _, err := c.scanLocalBlocksAndMedia(ctx, paths)
	return hashMap, blockMap, err
}

// scanLocalBlocksAndMedia extracts both blocks and media from local files.
// Returns block hashes, blocks, media hashes (sourceID→blobKey), and media grouped by item.
func (c *BowrainSourceConnector) scanLocalBlocksAndMedia(ctx context.Context, paths []string) (
	map[string]map[string]string, map[string][]*model.Block,
	map[string]map[string]string, map[string][]*model.Media, error,
) {
	hashMap := map[string]map[string]string{}
	blockMap := map[string][]*model.Block{}
	mediaHashMap := map[string]map[string]string{}
	mediaMap := map[string][]*model.Media{}

	recipe := c.project.Recipe
	assetsEnabled := recipe.AssetsEnabled()

	// The declared redaction policy applies HERE, where content enters kapi —
	// before anything hashes, stores or transmits it. Push used to read the
	// policy nowhere at all, so a project that declared its content sensitive
	// still sent originals to the server; only `kapi extract` honoured it.
	//
	// Redacting at ingest also means the content hashes below are hashes of the
	// redacted text, which is what the server should be diffing: the vault
	// stays local, and the server never holds a value it could restore.
	redactSpec := host.ProjectRedaction(&recipe.KapiProject)
	vaultPath := c.project.Layout.RedactionVaultPath()
	srcLocale := recipe.Defaults.SourceLanguage

	// If no specific paths, use content entries to discover files. A file two
	// items match is read once, for the first of them.
	if len(paths) == 0 {
		seen := map[string]bool{}
		for _, it := range recipe.IterateContent() {
			lang := string(it.Item.ResolvedSourceLanguage(it.Collection, recipe.Defaults))
			pattern := coreproj.ResolvePathPattern(it.Item.Path, lang)
			relPaths, err := coreproj.ExpandGlob(c.project.Root, pattern, recipe.Defaults.Exclude...)
			if err != nil {
				continue
			}
			for _, rp := range relPaths {
				if seen[rp] {
					continue
				}
				seen[rp] = true
				paths = append(paths, filepath.Join(c.project.Root, rp))
			}
		}
	}

	if len(paths) == 0 {
		return hashMap, blockMap, mediaHashMap, mediaMap, nil
	}

	for _, p := range paths {
		absPath := c.project.ResolvePath(p)

		// Skip if file doesn't exist.
		info, err := os.Stat(absPath)
		if err != nil || info.IsDir() {
			continue
		}

		// Determine format from config mappings or registry detection.
		formatName := c.detectFormat(absPath)
		if formatName == "" {
			continue
		}

		relPath, _ := c.project.RelativePath(absPath)

		// Extract blocks and optionally media.
		if assetsEnabled {
			blocks, media, err := c.readBlocksAndMedia(ctx, absPath, formatName)
			if err != nil {
				continue
			}
			if err := host.RedactAtIngest(ctx, blocks, redactSpec, c.project.Root, vaultPath, srcLocale); err != nil {
				return nil, nil, nil, nil, fmt.Errorf("redact %s: %w", relPath, err)
			}

			fileHashes := map[string]string{}
			for _, b := range blocks {
				identity := model.ComputeIdentity(b)
				fileHashes[convergence.BlockKey(b)] = identity.RecordHash()
			}
			hashMap[relPath] = fileHashes
			blockMap[relPath] = blocks

			if len(media) > 0 {
				assetHashes := map[string]string{}
				for _, m := range media {
					assetHashes[m.ID] = m.BlobKey
				}
				mediaHashMap[relPath] = assetHashes
				mediaMap[relPath] = media
			}
		} else {
			blocks, err := c.readBlocks(ctx, absPath, formatName)
			if err != nil {
				continue
			}
			if err := host.RedactAtIngest(ctx, blocks, redactSpec, c.project.Root, vaultPath, srcLocale); err != nil {
				return nil, nil, nil, nil, fmt.Errorf("redact %s: %w", relPath, err)
			}

			fileHashes := map[string]string{}
			for _, b := range blocks {
				identity := model.ComputeIdentity(b)
				fileHashes[convergence.BlockKey(b)] = identity.RecordHash()
			}
			hashMap[relPath] = fileHashes
			blockMap[relPath] = blocks
		}
	}

	return hashMap, blockMap, mediaHashMap, mediaMap, nil
}

// detectFormat determines the format for a file: the format the claiming
// content item declares (itemFor, the recipe's first-match order), or the
// registry when it declares none.
func (c *BowrainSourceConnector) detectFormat(absPath string) string {
	if item := c.itemFor(absPath); item != nil {
		if formatName := coreproj.ResolveFormat(formatNameOf(item.Format)); formatName != "" {
			return formatName
		}
	}

	// Fall back to the project's content-aware detector — the same
	// ProjectContext.DetectFormat the framework hosts use. A shared extension
	// is disambiguated by file content and per-format priority, and a compound
	// suffix like .kbf.json resolves to the KBF reader rather than the generic
	// JSON reader its .json tail would otherwise select.
	return c.projectContext().DetectFormat(c.formatReg, absPath)
}

// projectContext builds a framework ProjectContext over the connector's recipe,
// which embeds the framework KapiProject, so content-aware format detection
// resolves exactly as it does for kapi's own hosts.
func (c *BowrainSourceConnector) projectContext() *coreproj.ProjectContext {
	recipePath := filepath.Join(c.project.Root, coreproj.RecipeFileName)
	return coreproj.NewProjectContext(&c.project.Recipe.KapiProject, recipePath)
}

// itemFor returns the recipe content item whose glob claims absPath, in the
// recipe's own first-match order — the same walk detectFormat uses. nil when
// nothing claims it.
func (c *BowrainSourceConnector) itemFor(absPath string) *coreproj.ContentItem {
	relPath, err := c.project.RelativePath(absPath)
	if err != nil {
		return nil
	}
	relPath = filepath.ToSlash(relPath)
	recipe := c.project.Recipe
	for _, it := range recipe.IterateContent() {
		lang := string(it.Item.ResolvedSourceLanguage(it.Collection, recipe.Defaults))
		pattern := coreproj.ResolvePathPattern(it.Item.Path, lang)
		if matched, merr := doublestar.Match(pattern, relPath); merr == nil && matched {
			item := it.Item.ContentItem
			return &item
		}
	}
	return nil
}

// configureReaderFor applies the recipe's format configuration for the item that
// claims absPath — the project's format defaults overlaid by the item's own
// format.config.
//
// A push that skipped this extracts every leaf of a document the recipe
// described as partly structural: the scene ids and kinds of a narration script,
// the identifiers and slugs of a CMS export. The server has no second chance to
// tell content from identity, so it translates what it is given and the venue
// writes the result back over the source's structure.
func (c *BowrainSourceConnector) configureReaderFor(reader format.DataFormatReader, formatName, absPath string) error {
	return c.projectContext().ConfigureReaderFor(reader, formatName, c.itemFor(absPath))
}

// configureWriterFor is configureReaderFor's other half: the recipe's encoding
// and serialization keys for the item that claims absPath.
func (c *BowrainSourceConnector) configureWriterFor(writer format.DataFormatWriter, formatName, absPath string) error {
	return c.projectContext().ConfigureWriterFor(writer, formatName, c.itemFor(absPath))
}

// buildItemMeta generates editor metadata (BlockIndex + PreviewHTML) for each
// unique item that has changed blocks. It re-parses the source files using
// editor.ParseItem to build the full Part stream needed for metadata generation.
//
// It also names each item's collection — the recipe collection whose glob
// claims the item's path — which is what links a stored item to the context
// content type's collection row. The matcher is the recipe's own
// (core/project.CollectionForPath): the same globs, in the same first-match
// order, that target resolution uses, so an item cannot land in one collection
// for governance and another for output.
func (c *BowrainSourceConnector) buildItemMeta(ctx context.Context, changed []itemBlock) []apiclient.ItemMeta {
	// Collect unique item names that have changes.
	seen := map[string]bool{}
	var itemNames []string
	for _, ib := range changed {
		if ib.itemName != "" && !seen[ib.itemName] {
			seen[ib.itemName] = true
			itemNames = append(itemNames, ib.itemName)
		}
	}

	sourceLocale := string(c.project.Recipe.Defaults.SourceLanguage)
	var meta []apiclient.ItemMeta

	for _, itemName := range itemNames {
		absPath := c.project.ResolvePath(filepath.Join(c.project.Root, itemName))
		formatName := c.detectFormat(absPath)
		if formatName == "" {
			continue
		}

		reader, err := c.formatReg.NewReader(registry.FormatID(formatName))
		if err != nil {
			continue
		}

		f, err := os.Open(absPath)
		if err != nil {
			continue
		}

		doc := &model.RawDocument{
			URI:      absPath,
			FormatID: formatName,
			Reader:   f,
		}

		result, err := editor.ParseItem(ctx, reader, doc, sourceLocale, formatName, itemName)
		if err != nil {
			continue
		}

		meta = append(meta, apiclient.ItemMeta{
			Name:        itemName,
			Format:      formatName,
			BlockIndex:  result.BlockIndexJSON,
			PreviewHTML: result.PreviewHTML,
			Collection:  c.project.Recipe.CollectionForPath(filepath.ToSlash(itemName)),
		})
	}

	return meta
}

// readBlocks reads a file and extracts blocks using the format reader.
func (c *BowrainSourceConnector) readBlocks(ctx context.Context, filePath, formatName string) ([]*model.Block, error) {
	blocks, _, err := c.readBlocksAndMedia(ctx, filePath, formatName)
	return blocks, err
}

// readBlocksAndMedia reads a file and extracts both blocks and media using the format reader.
//
// The media it returns outlive this call — a push scans every changed file
// before uploading any asset — so a deferred slice is materialized here rather
// than left pointing into a document that is about to close. That is the one
// consumer that genuinely wants the bytes; everything else on the read path
// keeps them in the container.
func (c *BowrainSourceConnector) readBlocksAndMedia(ctx context.Context, filePath, formatName string) ([]*model.Block, []*model.Media, error) {
	reader, err := c.formatReg.NewReader(registry.FormatID(formatName))
	if err != nil {
		return nil, nil, fmt.Errorf("create reader for %s: %w", formatName, err)
	}
	defer reader.Close()

	// What the recipe says this file is, before anything the push wants: the
	// extraction rules decide which of its leaves are content at all.
	if err := c.configureReaderFor(reader, formatName, filePath); err != nil {
		return nil, nil, fmt.Errorf("apply the recipe's format config for %s: %w", formatName, err)
	}

	// Enable media extraction if the format supports it.
	if cfg := reader.Config(); cfg != nil {
		_ = cfg.ApplyMap(map[string]any{"extractMedia": true})
	}

	f, err := os.Open(filePath)
	if err != nil {
		return nil, nil, fmt.Errorf("open file %s: %w", filePath, err)
	}
	defer f.Close()

	doc := &model.RawDocument{
		URI:      filePath,
		FormatID: formatName,
		Reader:   f,
	}
	// A package format reads its entries straight out of the file handle
	// instead of buffering the container.
	if info, serr := f.Stat(); serr == nil {
		doc.ReaderAt, doc.Size = f, info.Size()
	}

	if err := reader.Open(ctx, doc); err != nil {
		return nil, nil, fmt.Errorf("open document %s: %w", filePath, err)
	}

	var blocks []*model.Block
	var media []*model.Media
	ch := reader.Read(ctx)
	for pr := range ch {
		if pr.Error != nil {
			continue
		}
		switch pr.Part.Type {
		case model.PartBlock:
			if b, ok := pr.Part.Resource.(*model.Block); ok {
				blocks = append(blocks, b)
			}
		case model.PartMedia:
			if m, ok := pr.Part.Resource.(*model.Media); ok {
				if merr := m.Materialize(); merr != nil {
					continue // best-effort: skip an asset we cannot read
				}
				media = append(media, m)
			}
		}
	}

	return blocks, media, nil
}

// resolveTargetPath maps a pulled item to the output path its translation is
// written to, using the same matcher and target expansion the local engine
// uses (project.MatchGlob + project.ResolveTargetPath) — one template
// vocabulary, one behavior. A hand-rolled copy here once dropped the {dir}
// token and reconstructed the SOURCE path, so a pull overwrote four English
// masters with Norwegian; the in-place guard at the bottom makes that class
// impossible regardless of template.
func (c *BowrainSourceConnector) resolveTargetPath(itemName, locale string) string {
	recipe := c.project.Recipe
	srcLang := string(recipe.Defaults.SourceLanguage)

	out := ""
	for _, it := range recipe.IterateContent() {
		entryLang := string(it.Item.ResolvedSourceLanguage(it.Collection, recipe.Defaults))
		pattern := coreproj.ResolvePathPattern(it.Item.Path, entryLang)
		if !coreproj.MatchGlob(pattern, itemName) {
			continue
		}
		if it.Item.Target == "" {
			break // matched, but no target — the fallbacks below decide
		}
		out = coreproj.ResolveTargetPath(pattern, it.Item.Base, it.Item.Target, itemName, locale)
		break
	}

	if out == "" {
		// No matching item or no target template: replace the source locale
		// segment when the path carries one, else suffix the locale.
		if srcLang != "" && strings.Contains(itemName, srcLang) {
			out = strings.Replace(itemName, srcLang, locale, 1)
		} else {
			ext := filepath.Ext(itemName)
			out = strings.TrimSuffix(itemName, ext) + "." + locale + ext
		}
	}

	// A target-language write must never land on the source file. Whatever
	// produced the collision (a template without {lang}, a mapping bug), the
	// locale-suffixed sibling is always a safe destination.
	if filepath.Clean(out) == filepath.Clean(itemName) {
		ext := filepath.Ext(itemName)
		out = strings.TrimSuffix(itemName, ext) + "." + locale + ext
	}
	return out
}

// writeTranslatedFile reads a source file, injects target translations into blocks,
// and writes the translated output file using the appropriate format writer.
// MediaReplacement describes a locale-variant media file to substitute in the
// output. The variant asset travels as a *model.Media reference (a local file
// path the writer streams from), never as inline bytes on this struct.
type MediaReplacement struct {
	ZipPath string       // original ZIP entry path (e.g., "word/media/image1.png")
	Media   *model.Media // locale-variant asset reference
}

// MediaReplacementSetter is implemented by writers that support locale-variant media substitution.
type MediaReplacementSetter interface {
	SetMediaReplacement(zipPath string, media *model.Media)
}

// targetMatchKey computes a stable key for matching a pulled translation to a
// locally re-parsed source block. Server-assigned block IDs are not preserved
// across push/pull, so matching on b.ID fails (translations would never land).
// Keyed catalogs (JSON, YAML, .strings, …) carry a stable block name; nameless
// prose blocks fall back to source-text identity.
func targetMatchKey(name, sourceText string) string {
	if name != "" {
		return "n:" + name
	}
	return "s:" + sourceText
}

func (c *BowrainSourceConnector) writeTranslatedFile(ctx context.Context, sourcePath, outputPath, formatName, locale string, targets map[string]string, mediaReplacements ...MediaReplacement) error {
	// Read source.
	reader, err := c.formatReg.NewReader(registry.FormatID(formatName))
	if err != nil {
		return fmt.Errorf("create reader for %s: %w", formatName, err)
	}

	// The writer is chosen from the OUTPUT path, not the source format: a
	// recipe target whose extension differs from its source is a cross-format
	// projection, and writing source-format bytes to that path would corrupt
	// the target (a JSON document under a .mo name, say).
	// registry.WriterFormatFor is the same rule the host flow runner applies.
	writerFormat := c.formatReg.WriterFormatFor(registry.FormatID(formatName), outputPath)
	writer, err := c.formatReg.NewWriter(writerFormat)
	if err != nil {
		return fmt.Errorf("create writer for %s: %w", writerFormat, err)
	}

	// The recipe's configuration, on both halves. The read here re-derives the
	// blocks a pulled translation is matched against, so a reader configured any
	// other way offers the pull leaves the recipe never declared translatable
	// and the write puts a translation on the document's structure.
	if err := c.configureReaderFor(reader, formatName, sourcePath); err != nil {
		return fmt.Errorf("apply the recipe's format config for %s: %w", formatName, err)
	}
	if err := c.configureWriterFor(writer, string(writerFormat), sourcePath); err != nil {
		return fmt.Errorf("apply the recipe's writer config for %s: %w", writerFormat, err)
	}

	// A skeleton store lets a SAME-format writer reconstruct non-translatable
	// content (frontmatter, structural markup, etc.) byte-exactly from the
	// source. NewWiredSkeleton wires it only for an eligible same-format pair —
	// a cross-format writer reconstructs from the content model instead, and
	// must never be fed a foreign skeleton.
	skelStore, err := format.NewWiredSkeleton(reader, writer)
	if err != nil {
		return fmt.Errorf("create skeleton store: %w", err)
	}
	if skelStore != nil {
		defer skelStore.Close()
	}

	f, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open source file %s: %w", sourcePath, err)
	}

	doc := &model.RawDocument{
		URI:      sourcePath,
		FormatID: formatName,
		Reader:   f,
	}
	if err := reader.Open(ctx, doc); err != nil {
		f.Close()
		return fmt.Errorf("open document %s: %w", sourcePath, err)
	}

	// Collect parts, injecting targets.
	var parts []*model.Part
	ch := reader.Read(ctx)
	for pr := range ch {
		if pr.Error != nil {
			continue
		}
		p := pr.Part
		if p.Type == model.PartBlock {
			if b, ok := p.Resource.(*model.Block); ok {
				if t, exists := targets[targetMatchKey(b.Name, b.SourceText())]; exists {
					b.SetTargetText(model.LocaleID(locale), t)
				}
			}
		}
		parts = append(parts, p)
	}

	// For formats that need the original file content for reconstruction
	// (openxml, idml, odf, epub — ZIP-based formats that rebuild from original).
	if sps, ok := writer.(format.SourcePathSetter); ok {
		sps.SetSourcePath(sourcePath)
	} else if ocs, ok := writer.(format.OriginalContentSetter); ok {
		srcBytes, err := os.ReadFile(sourcePath)
		if err != nil {
			return fmt.Errorf("read source for writer: %w", err)
		}
		ocs.SetOriginalContent(srcBytes)
	}

	// Apply locale-variant media replacements (Bowrain AD-007).
	if mrs, ok := writer.(MediaReplacementSetter); ok {
		for _, mr := range mediaReplacements {
			mrs.SetMediaReplacement(mr.ZipPath, mr.Media)
		}
	}

	// Ensure output directory exists.
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	if err := writer.SetOutput(outputPath); err != nil {
		return fmt.Errorf("set output path: %w", err)
	}
	writer.SetLocale(model.LocaleID(locale))

	outCh := make(chan *model.Part, len(parts))
	for _, p := range parts {
		outCh <- p
	}
	close(outCh)

	if err := writer.Write(ctx, outCh); err != nil {
		return fmt.Errorf("write translated file: %w", err)
	}

	return writer.Close()
}

// listMediaVariants reads the variants of an item's assets, once.
//
// A variant list is locale-INDEPENDENT: the endpoint answers with every variant
// an asset has and the caller filters. Asking inside the per-locale loop
// therefore fetched the same list once per target language and threw all but
// one row away each time — three locales, three identical round trips per
// asset. Hoisting it here is the whole saving, and it needs no new endpoint.
//
// A download URL is minted by this call, and expires. Listing here rather than
// with the pull keeps it close to its use: an expired URL is a variant silently
// missing from a written file, so the gap between minting and downloading is
// the thing worth keeping short.
func (c *BowrainSourceConnector) listMediaVariants(ctx context.Context, assets []apiclient.SyncMedia) (map[string][]apiclient.AssetVariantResponse, []error) {
	if c.client == nil || len(assets) == 0 {
		return nil, nil
	}
	out := make(map[string][]apiclient.AssetVariantResponse, len(assets))
	var errs []error
	for _, asset := range assets {
		variants, err := c.client.ListAssetVariants(ctx, asset.ID)
		if err != nil {
			errs = append(errs, fmt.Errorf("list variants of asset %s (%s): %w", asset.Filename, asset.ID, err))
			continue
		}
		out[asset.ID] = variants
	}
	return out, errs
}

// fetchMediaReplacements materializes the approved variants for one locale to
// temp files and returns MediaReplacement entries that reference them by path
// (so the writer streams the asset rather than buffering it). The returned
// cleanup removes the temp files and must be called once the writer is done
// with them.
//
// A variant that cannot be fetched is REPORTED, not skipped. Every one of these
// used to be a bare `continue`: the file was written without its media and the
// pull exited zero, which is the failure shape a written file cannot show you.
// The errors join the write failures, so the cursor is held and the next pull
// tries again — the same contract an unreadable format already has.
func (c *BowrainSourceConnector) fetchMediaReplacements(
	ctx context.Context,
	assets []apiclient.SyncMedia,
	variantsByAsset map[string][]apiclient.AssetVariantResponse,
	locale string,
) ([]MediaReplacement, func(), []error) {
	noop := func() {}
	if c.client == nil || len(assets) == 0 {
		return nil, noop, nil
	}
	var errs []error

	var tmpDir string
	cleanup := func() {
		if tmpDir != "" {
			_ = os.RemoveAll(tmpDir)
		}
	}

	var replacements []MediaReplacement
	for _, asset := range assets {
		variants := variantsByAsset[asset.ID]

		for _, v := range variants {
			if v.Locale != locale || v.Status != "approved" {
				continue
			}
			if v.DownloadURL == "" {
				// The venue holds the bytes and will not say where. On a
				// deployment whose blob store cannot presign this was every
				// variant, every time, and the file was written without its
				// media and reported as a success.
				errs = append(errs, fmt.Errorf(
					"variant %s of %s (%s): the venue returned no download location",
					v.Locale, asset.Filename, asset.ID))
				continue
			}

			// Download the variant binary and materialize it to a temp file so
			// the writer can stream it into the output instead of holding every
			// variant's bytes in memory.
			data, err := c.client.DownloadBlob(ctx, v.DownloadURL)
			if err != nil {
				errs = append(errs, fmt.Errorf(
					"download variant %s of %s: %w", v.Locale, asset.Filename, err))
				continue
			}
			if tmpDir == "" {
				tmpDir, err = os.MkdirTemp("", "bowrain-media-*")
				if err != nil {
					cleanup()
					return nil, noop, append(errs, fmt.Errorf("create a temp dir for media: %w", err))
				}
			}
			variantPath := filepath.Join(tmpDir, fmt.Sprintf("%s-%s", asset.ID, v.Locale))
			if err := os.WriteFile(variantPath, data, 0o644); err != nil {
				errs = append(errs, fmt.Errorf(
					"materialize variant %s of %s: %w", v.Locale, asset.Filename, err))
				continue
			}

			// Use the asset's zipPath property if available, otherwise
			// fall back to sourceID which contains the ZIP path.
			zipPath := strings.TrimPrefix(asset.SourceID, "media:")

			replacements = append(replacements, MediaReplacement{
				ZipPath: zipPath,
				Media:   &model.Media{URI: variantPath, Filename: filepath.Base(zipPath), Size: int64(len(data))},
			})
		}
	}

	return replacements, cleanup, errs
}

// fetchAndCacheMetadata fetches project metadata from the server and caches it.
// Errors are non-fatal — metadata is best-effort.
func (c *BowrainSourceConnector) fetchAndCacheMetadata(ctx context.Context) {
	if c.client == nil {
		return
	}

	meta, err := c.client.GetProjectMetadata(ctx)
	if err != nil {
		return
	}

	c.cache.ServerMeta = &CachedProjectMeta{
		TargetLanguages: meta.TargetLanguages,
		FetchedAt:       time.Now().UTC(),
	}
}

// ServerTargetLocales returns target locales from the server cache.
// Returns nil if no cached metadata is available.
func (c *BowrainSourceConnector) ServerTargetLocales() []model.LocaleID {
	if c.cache.ServerMeta == nil || len(c.cache.ServerMeta.TargetLanguages) == 0 {
		return nil
	}
	locales := make([]model.LocaleID, len(c.cache.ServerMeta.TargetLanguages))
	for i, l := range c.cache.ServerMeta.TargetLanguages {
		locales[i] = model.LocaleID(l)
	}
	return locales
}

// pushScope is what this push is authoritative over.
//
// It is stated as PATTERNS, never as the paths the scan resolved, and the
// difference is the whole point: a deleted file matches no glob, so a scope
// built from what the scan found could never contain the file whose absence it
// is supposed to explain. The recipe's globs say "I speak for everything that
// looks like this", which covers the file that used to.
//
// The recipe's exclusions ride along negated, because a directory swept into a
// glob and then excluded is not content the source deleted.
//
// A scoped `kapi push <path>` narrows it to those paths: a directory names
// everything beneath it, a file names itself. That is what makes the scoped
// push safe by construction rather than by nobody looking — the files outside
// it are out of scope by the same rule that governs the rest, and the venue
// enforces it.
func (c *BowrainSourceConnector) pushScope(paths []string) venue.Scope {
	recipe := c.project.Recipe

	var scope venue.Scope
	if len(paths) == 0 {
		for _, it := range recipe.IterateContent() {
			lang := string(it.Item.ResolvedSourceLanguage(it.Collection, recipe.Defaults))
			if pattern := coreproj.ResolvePathPattern(it.Item.Path, lang); pattern != "" {
				scope = append(scope, pattern)
			}
		}
	} else {
		for _, p := range paths {
			rel, err := filepath.Rel(c.project.Root, c.project.ResolvePath(p))
			if err != nil || strings.HasPrefix(rel, "..") {
				// A path outside the project is not something this push can
				// speak for. Declaring it would claim authority over content
				// this producer never reads.
				continue
			}
			scope = append(scope, filepath.ToSlash(rel))
		}
	}
	if len(scope) == 0 {
		return nil
	}
	for _, ex := range recipe.Defaults.Exclude {
		if ex != "" {
			scope = append(scope, "!"+ex)
		}
	}
	return scope
}

// lookupCachedHashForItem finds a block's cached hash for a specific item.
// venueHoldsMoreThan reports whether the venue holds an item, or a block within
// one, that this scan did not produce — inside the scope this push declares.
//
// It answers whether to push at all, never what to delete. The deletion itself
// is decided at the far side, from the declared tree and scope, which is why a
// fresh clone with no cache still prunes: there is nothing local to miss
// against here, and everything to declare there.
//
// It reads the venue's tree rather than the local cache for the same reason the
// diff does. The cache is what THIS machine last sent; the tree is what the
// venue actually holds, and a deletion made on another machine — or a push that
// half-landed — is only visible in the second.
func venueHoldsMoreThan(serverTree *apiclient.TreeResponse, localTree venue.Tree, scope venue.Scope) bool {
	if serverTree == nil {
		return false
	}
	for _, item := range serverTree.Items {
		if len(scope) > 0 && !scope.Covers(item.Path) {
			continue
		}
		local, scanned := localTree[item.Path]
		if !scanned {
			// The venue holds a file this scan did not read. Inside a declared
			// scope that is a deletion; with no scope declared it is silence.
			return len(scope) > 0
		}
		have := make(map[string]struct{}, len(local.Keys))
		for _, k := range local.Keys {
			have[k] = struct{}{}
		}
		for _, k := range item.Keys {
			if _, still := have[k]; !still {
				return true
			}
		}
	}
	return false
}

func (c *BowrainSourceConnector) lookupCachedHashForItem(itemName, blockID string) (string, bool) {
	fc, ok := c.cache.Files[itemName]
	if !ok {
		return "", false
	}
	hash, found := fc.Blocks[blockID]
	return hash, found
}

// Stream returns the resolved stream name for this connector.
func (c *BowrainSourceConnector) Stream() string {
	return c.stream
}

// SetStream overrides the resolved stream (e.g. from --stream flag).
func (c *BowrainSourceConnector) SetStream(stream string) {
	c.stream = schema.NormalizeStreamName(stream)
	if c.client != nil {
		c.client.SetStream(c.stream)
	}
}

// Client returns the underlying BowrainClient for direct API calls (e.g. PushStatus).
func (c *BowrainSourceConnector) Client() *apiclient.BowrainClient {
	return c.client
}

// Ensure BowrainSourceConnector implements SourceConnector at compile time.
var _ bowrainconn.SourceConnector = (*BowrainSourceConnector)(nil)

// allScannedBlocks flattens a scan's per-item blocks into one slice, for the
// facts a push declares about itself rather than about its payload.
func allScannedBlocks(blockMap map[string][]*model.Block) []*model.Block {
	var all []*model.Block
	for _, blocks := range blockMap {
		all = append(all, blocks...)
	}
	return all
}
