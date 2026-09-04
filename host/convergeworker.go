package host

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// convergeJobsDefault is how many locales a convergence pass runs concurrently
// when the run does not say otherwise (`up --jobs`, defaults.jobs). Locales
// are independent within a pass (disjoint target rows and files, shared
// read-only source and content memory), so the fan-out is safe; 4 keeps the effective
// LLM concurrency (jobs × parallel-blocks) inside typical provider limits.
const convergeJobsDefault = 4

// convergeWorker builds the per-locale worker App a convergence pass fans out
// on. Workers share everything immutable-during-a-run with the parent —
// registries, config, credentials, the project context/bindings, the document
// cache (SQLite, WAL), the plugin runtime — and own only the per-run mutable
// state that made App single-locale: TargetLang, the projectFlowTools slot
// (each worker builds its own tool instances, so no LLM provider or content-memory handle
// is shared mid-flight), and the progress tap.
//
// This is an explicit field-by-field copy, NOT a struct copy: App carries a
// sync.Once (pluginRuntimeOnce), and the plugin runtime must be pre-seeded
// from the parent so workers never race to build daemon pools of their own.
// When App grows a new field, decide whether workers share or own it and say
// so in convergeWorkerFields — TestConvergeWorker_CoversEveryAppField fails
// until you do, so a new field can't silently go missing from every worker.
func (a *App) convergeWorker(locale string, tap *convergeTap) *App {
	return &App{
		FormatReg:  a.FormatReg,
		ToolReg:    a.ToolReg,
		SchemaReg:  a.SchemaReg,
		PluginHost: a.PluginHost,
		Config:     a.Config,

		Verbose:   a.Verbose,
		Quiet:     true, // workers are silent: progress speaks the event protocol; the parent owns the terminal
		AssumeYes: a.AssumeYes,
		CfgFile:   a.CfgFile,
		PluginDir: a.PluginDir,
		Lang:      a.Lang,
		Explain:   a.Explain,

		FormatFlag: a.FormatFlag,
		Encoding:   a.InputEncoding(),
		SourceLang: a.SourceLocale(),
		TargetLang: locale,
		ConvTiming: a.ConvTiming,

		MemoryBackend: a.MemoryBackend,
		TermsBackend:  a.TermsBackend,
		BlocksBackend: a.BlocksBackend,
		Credentials:   a.Credentials,

		RegistryResolver: a.RegistryResolver,
		FallbackRunE:     a.FallbackRunE,
		ExtraFlows:       a.ExtraFlows,

		ProjectContext:      a.ProjectContext,
		MCPSurface:          a.MCPSurface,
		execTrustGranted:    a.execTrustGranted,
		ProjectBindings:     a.ProjectBindings,
		convergeWriteFiles:  a.convergeWriteFiles,
		convergeDraftDir:    a.convergeDraftDir,
		convergeDraftRoot:   a.convergeDraftRoot,
		docCache:            a.docCache,
		translator:          a.translator,
		AISetupIOOverride:   a.AISetupIOOverride,
		AISetupPrompter:     a.AISetupPrompter,
		isTTY:               a.isTTY,
		convergeProgressTap: tap,

		// Pre-seed the parent's runtime (building it on the parent if needed —
		// concurrent callers serialize on the parent's Once). The worker's own
		// fresh Once sees the seed and never builds a second runtime.
		pluginRuntime: a.ensurePluginRuntime(),
		// Same arrangement for the project stores, and the same reason: the
		// locales of one pass read and write one `.kapi/work/store.db`, so they must
		// share the pool rather than open one each.
		projectStores: a.ensureProjectStores(),
		// And for governance: one instant for the whole pass, one report of a
		// profile that expired under it.
		governance: a.ensureGovernance(),
		// The freshness watch is one process's reading history, and a fan-out
		// is one reader. Sharing it means a convergence that spans a governance
		// move reports it once, to whichever worker read across it — rather
		// than once per locale, or not at all because each worker's first read
		// established a baseline of its own.
		freshness: a.freshnessWatch(),
	}
}

// workerFieldPolicy says what a converge worker does with one App field.
type workerFieldPolicy int

const (
	// fieldShared: the worker reads the parent's value (immutable during a run).
	fieldShared workerFieldPolicy = iota
	// fieldOwned: the worker gets its own value — the per-run mutable state that
	// made App single-locale, plus the fresh sync.Once.
	fieldOwned
)

// convergeWorkerFields classifies every App field for the worker clone. It
// exists so the clone can be checked for completeness: App is copied
// field-by-field (it holds a sync.Once, so a struct copy is out), and a field
// added to App but forgotten here would silently read as its zero value inside
// every converge worker — a project context or credential store that is simply
// absent once a run fans out. TestConvergeWorker_CoversEveryAppField reconciles
// this map against App's actual fields and against what the clone produces.
var convergeWorkerFields = map[string]workerFieldPolicy{
	"FormatReg":         fieldShared,
	"ToolReg":           fieldShared,
	"SchemaReg":         fieldShared,
	"PluginHost":        fieldShared,
	"Config":            fieldShared,
	"Verbose":           fieldShared,
	"Quiet":             fieldOwned, // workers are silent; the parent owns the terminal
	"AssumeYes":         fieldShared,
	"CfgFile":           fieldShared,
	"PluginDir":         fieldShared,
	"Lang":              fieldShared,
	"Explain":           fieldShared,
	"FormatFlag":        fieldShared,
	"Encoding":          fieldShared,
	"SourceLang":        fieldShared,
	"TargetLang":        fieldOwned, // the whole point: one worker, one locale
	"ConvTiming":        fieldShared,
	"MemoryBackend":     fieldShared,
	"TermsBackend":      fieldShared,
	"BlocksBackend":     fieldShared,
	"Credentials":       fieldShared,
	"AISetupIOOverride": fieldShared,
	"AISetupPrompter":   fieldShared, // presentation, and a worker never prompts
	// Terminal detection for App's own prompts. A worker never prompts (the
	// parent owns the terminal), so this is inert in a worker — carried rather
	// than reset so the clone stays a faithful copy, like MCPSurface below.
	"isTTY":            fieldShared,
	"RegistryResolver": fieldShared,
	"FallbackRunE":     fieldShared,
	"ExtraFlows":       fieldShared,
	"ProjectContext":   fieldShared,
	// The execution-trust decision belongs to the run, not to the locale: the
	// user was asked once about one project's recipe, and every worker fanning
	// out over that project inherits the answer. Owning it instead would leave
	// each worker unable to build the step the user approved (AD-038).
	"execTrustGranted": fieldShared,
	// The MCP tool surface is set from `kapi mcp` flags and read only while that
	// server is running. A converge worker never serves MCP, so sharing the
	// parent's value is correct and inert — it is carried rather than reset so
	// the clone stays a faithful copy.
	"MCPSurface":         fieldShared,
	"ProjectBindings":    fieldShared,
	"convergeWriteFiles": fieldShared,
	// One draft tree for the whole run: the locales of a pass draft side by
	// side and finishConverge delivers from the store, so a per-worker tree
	// would be a per-locale answer to a question the run asks once.
	"convergeDraftDir":    fieldShared,
	"convergeDraftRoot":   fieldShared,
	"docCache":            fieldShared,
	"translator":          fieldShared,
	"pluginRuntime":       fieldShared, // pre-seeded, never rebuilt per worker
	"projectStores":       fieldShared, // pre-seeded: one store per project, not per locale
	"governance":          fieldShared, // one instant and one transition report per run
	"governanceOnce":      fieldOwned,  // a fresh Once that sees the pre-seeded state
	"freshness":           fieldShared, // one process, one reading history
	"freshnessOnce":       fieldOwned,  // a fresh Once that sees the pre-seeded watch
	"projectFlowTools":    fieldOwned,  // each worker builds its own tool instances
	"flowFindings":        fieldOwned,  // armed per reported run; a silent worker reports none
	"flowFindingsSink":    fieldOwned,  // scoped to one RunFromProject; a worker gates nothing
	"convergeProgressTap": fieldOwned,  // one tap per locale
	"pluginRuntimeOnce":   fieldOwned,  // a fresh Once that sees the pre-seeded runtime
	"projectStoresOnce":   fieldOwned,  // a fresh Once that sees the pre-seeded holder
	// The parent owns the --explain collector and renders the transcript once.
	// The LLM recorder is process-wide, so a worker's calls are still captured
	// into the parent's collector; a worker holding its own would flush a
	// partial transcript of just its locale.
	"explain": fieldOwned,
}

// convergeTap is the trailing read-only step a converge worker appends to its
// flow (runProjectStepsOver): it observes every block leaving the pipeline and
// counts the units that carry a committed target for the worker's locale —
// split by Origin.Kind into content memory-recycled and AI/MT-produced — feeding the
// unit_progress events of the convergence run. Counters are atomic because
// ParallelBlockTool may drive Annotate from several goroutines.
type convergeTap struct {
	tool.BaseTool
	locale model.LocaleID

	done      atomic.Int64
	viaMemory atomic.Int64
	viaAI     atomic.Int64

	// reuse holds the pass's translate tools, which count the blocks they
	// served from the block store instead of sending to a provider. The tap
	// cannot see that on the block: a reused draft carries the provenance of
	// the engine that first made it, correctly, so what the pass paid for is
	// the tool's own fact and has to be read from the tool. One chain is built
	// per binding group and the groups run in turn, so the sources accumulate.
	reuseMu sync.Mutex
	reuse   []reusedTargetCounter
}

// reusedTargetCounter is a producer that can say how many blocks it served from
// the block store rather than translating (core/ai/tools, core/mt/tools).
type reusedTargetCounter interface{ ReusedTargets() int }

// countReuse registers the tools whose reuse this pass should report.
func (t *convergeTap) countReuse(tools []tool.Tool) {
	t.reuseMu.Lock()
	defer t.reuseMu.Unlock()
	for _, tl := range tools {
		if c, ok := tl.(reusedTargetCounter); ok {
			t.reuse = append(t.reuse, c)
		}
	}
}

// reusedDrafts sums what the pass's producers served from the store.
func (t *convergeTap) reusedDrafts() int {
	t.reuseMu.Lock()
	defer t.reuseMu.Unlock()
	total := 0
	for _, c := range t.reuse {
		total += c.ReusedTargets()
	}
	return total
}

// newConvergeTap builds the tap for one locale's pass run.
func newConvergeTap(locale string) *convergeTap {
	t := &convergeTap{locale: model.LocaleID(locale)}
	t.ToolName = "converge-progress"
	t.ToolDescription = "counts converged units for live run progress (internal)"
	t.Annotate = func(v tool.BlockView) error {
		if !v.Translatable() {
			return nil
		}
		tgt := v.Target(t.locale)
		if tgt == nil || len(tgt.Runs) == 0 {
			return nil
		}
		t.done.Add(1)
		switch tgt.Origin.Kind {
		case model.OriginMemory:
			t.viaMemory.Add(1)
		case model.OriginAI, model.OriginMT:
			t.viaAI.Add(1)
		}
		return nil
	}
	return t
}

// snapshot returns the tap's current counts. A block served from the store
// carries its producer's provenance, so it arrives inside viaAI; it is moved to
// ViaDraft, because what the reader is being told is what the pass cost.
func (t *convergeTap) snapshot() convergence.PassProduction {
	p := convergence.PassProduction{
		Done:      int(t.done.Load()),
		ViaMemory: int(t.viaMemory.Load()),
		ViaAI:     int(t.viaAI.Load()),
		ViaDraft:  t.reusedDrafts(),
	}
	if p.ViaDraft > p.ViaAI {
		p.ViaDraft = p.ViaAI
	}
	p.ViaAI -= p.ViaDraft
	return p
}

// convergeProgressInterval throttles unit_progress: one event per locale at
// most this often (plus a final flush), keeping NDJSON/SSE streams and UI
// bridges light no matter how fast blocks flow.
const convergeProgressInterval = 700 * time.Millisecond

// watchTapProgress emits throttled unit_progress events from a tap until the
// returned stop function is called; stop flushes a final event when counts
// moved since the last emission.
func watchTapProgress(tap *convergeTap, pass int, emit func(convergence.Event)) (stop func()) {
	if emit == nil {
		return func() {}
	}
	var last int
	var mu sync.Mutex
	flush := func() {
		p := tap.snapshot()
		mu.Lock()
		defer mu.Unlock()
		if p.Done == last {
			return
		}
		last = p.Done
		emit(convergence.Event{
			Type:      convergence.EventUnitProgress,
			Pass:      pass,
			Locale:    string(tap.locale),
			Done:      p.Done,
			ViaMemory: p.ViaMemory,
			ViaAI:     p.ViaAI,
			ViaDraft:  p.ViaDraft,
		})
	}
	stopCh := make(chan struct{})
	doneCh := make(chan struct{})
	go func() {
		defer close(doneCh)
		ticker := time.NewTicker(convergeProgressInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				flush()
			case <-stopCh:
				flush()
				return
			}
		}
	}()
	return func() {
		close(stopCh)
		<-doneCh
	}
}
