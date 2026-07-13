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
// read-only source and TM), so the fan-out is safe; 4 keeps the effective
// LLM concurrency (jobs × parallel-blocks) inside typical provider limits.
const convergeJobsDefault = 4

// convergeWorker builds the per-locale worker App a convergence pass fans out
// on. Workers share everything immutable-during-a-run with the parent —
// registries, config, credentials, the project context/bindings, the document
// cache (SQLite, WAL), the plugin runtime — and own only the per-run mutable
// state that made App single-locale: TargetLang, the projectFlowTools slot
// (each worker builds its own tool instances, so no LLM provider or TM handle
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
		Encoding:   a.Encoding,
		SourceLang: a.SourceLang,
		TargetLang: locale,

		TMBackend:   a.TMBackend,
		TBBackend:   a.TBBackend,
		Credentials: a.Credentials,

		RegistryResolver: a.RegistryResolver,
		FallbackRunE:     a.FallbackRunE,
		ExtraFlows:       a.ExtraFlows,

		ProjectContext:      a.ProjectContext,
		ProjectBindings:     a.ProjectBindings,
		convergeWriteFiles:  a.convergeWriteFiles,
		docCache:            a.docCache,
		translator:          a.translator,
		AISetupIOOverride:   a.AISetupIOOverride,
		convergeProgressTap: tap,

		// Pre-seed the parent's runtime (building it on the parent if needed —
		// concurrent callers serialize on the parent's Once). The worker's own
		// fresh Once sees the seed and never builds a second runtime.
		pluginRuntime: a.ensurePluginRuntime(),
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
	"FormatReg":           fieldShared,
	"ToolReg":             fieldShared,
	"SchemaReg":           fieldShared,
	"PluginHost":          fieldShared,
	"Config":              fieldShared,
	"Verbose":             fieldShared,
	"Quiet":               fieldOwned, // workers are silent; the parent owns the terminal
	"AssumeYes":           fieldShared,
	"CfgFile":             fieldShared,
	"PluginDir":           fieldShared,
	"Lang":                fieldShared,
	"Explain":             fieldShared,
	"FormatFlag":          fieldShared,
	"Encoding":            fieldShared,
	"SourceLang":          fieldShared,
	"TargetLang":          fieldOwned, // the whole point: one worker, one locale
	"TMBackend":           fieldShared,
	"TBBackend":           fieldShared,
	"Credentials":         fieldShared,
	"AISetupIOOverride":   fieldShared,
	"RegistryResolver":    fieldShared,
	"FallbackRunE":        fieldShared,
	"ExtraFlows":          fieldShared,
	"ProjectContext":      fieldShared,
	"ProjectBindings":     fieldShared,
	"convergeWriteFiles":  fieldShared,
	"docCache":            fieldShared,
	"translator":          fieldShared,
	"pluginRuntime":       fieldShared, // pre-seeded, never rebuilt per worker
	"projectFlowTools":    fieldOwned,  // each worker builds its own tool instances
	"convergeProgressTap": fieldOwned,  // one tap per locale
	"pluginRuntimeOnce":   fieldOwned,  // a fresh Once that sees the pre-seeded runtime
	// The parent owns the --explain collector and renders the transcript once.
	// The LLM recorder is process-wide, so a worker's calls are still captured
	// into the parent's collector; a worker holding its own would flush a
	// partial transcript of just its locale.
	"explain": fieldOwned,
}

// convergeTap is the trailing read-only step a converge worker appends to its
// flow (runProjectStepsOver): it observes every block leaving the pipeline and
// counts the units that carry a committed target for the worker's locale —
// split by Origin.Kind into TM-recycled and AI/MT-produced — feeding the
// unit_progress events of the convergence run. Counters are atomic because
// ParallelBlockTool may drive Annotate from several goroutines.
type convergeTap struct {
	tool.BaseTool
	locale model.LocaleID

	done  atomic.Int64
	viaTM atomic.Int64
	viaAI atomic.Int64
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
		case model.OriginTM:
			t.viaTM.Add(1)
		case model.OriginAI, model.OriginMT:
			t.viaAI.Add(1)
		}
		return nil
	}
	return t
}

// snapshot returns the tap's current counts.
func (t *convergeTap) snapshot() (done, viaTM, viaAI int) {
	return int(t.done.Load()), int(t.viaTM.Load()), int(t.viaAI.Load())
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
		done, viaTM, viaAI := tap.snapshot()
		mu.Lock()
		defer mu.Unlock()
		if done == last {
			return
		}
		last = done
		emit(convergence.Event{
			Type:   convergence.EventUnitProgress,
			Pass:   pass,
			Locale: string(tap.locale),
			Done:   done,
			ViaTM:  viaTM,
			ViaAI:  viaAI,
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
