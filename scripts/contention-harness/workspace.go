package main

// The workspace topology measures the store as it ships after the split: a
// projection per checkout inside the tree, and one context store per project in
// the user's workspace, shared by every checkout and every process.
//
// The question it answers is the one the split creates. The context store is
// now written by everything at once — an agent's MCP server recording an
// observation, a review loop recording a decision, the desktop reading to
// redraw — where before each checkout had a file of its own. The shape measured
// here is deliberately the worst one the design admits:
//
//	16 agent processes   each in a checkout of its own, writing small
//	                     transactions into the SHARED context store: a decision
//	                     into the unit working set every round, and wording
//	                     promoted into the content memory every tenth
//	 1 desktop process   polling the same context store for change, read only
//	 1 CLI process       holding a long purge-and-refill transaction over ITS
//	                     OWN projection, plus a context write between passes
//
// The CLI process is what makes the run worth doing. Under the embedded layout
// its extraction transaction held the only file, so every one of those agent
// writes queued behind it; the claim the split makes is that it now holds a
// file none of them touch. The run either shows that or it does not.
//
// Pass bar: zero failed writes, and p99 latency under 50 ms for the small
// writes. The CLI's purge-and-refill is excluded from the latency half: it is a
// transaction over thousands of rows and is in the run as the antagonist.
//
// The two agent rates are a model of what an agent does, and the gap between
// them is the point. A decision is one upsert and costs about half a
// millisecond. Teaching the content memory maintains the FTS5 tables row by row,
// costs about 12 ms on a fresh store and 30 ms on a dogfood-sized one, and
// happens when a decision blesses wording worth reusing rather than on every
// decision. Run it at the decision rate (-memory-every=1) and the store
// saturates at dogfood scale: sixteen agents offer 16 promotions a second
// against a service rate near 33, and the tail goes with it.
//
//	go run -tags fts5 ./scripts/contention-harness -mode=workspace

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/core/workspace"
	"github.com/neokapi/neokapi/memory"
)

// topoWorkspace is the shipped two-pool store: a projection per checkout, one
// context store per project in a workspace.
const topoWorkspace topology = "workspace"

// The roles the workspace topology re-execs itself into.
const (
	roleAgent   = "agent"
	roleDesktop = "desktop"
	roleCLI     = "cli"
)

// Stream names for the workspace run. They are the rows of its report.
const (
	nWSDecision = "WS agent    decision put"
	nWSMemory   = "WS agent    memory add"
	nWSRegister = "WS agent    register in workspace"
	nWSPoll     = "WS desktop  poll for change"
	nWSExtract  = "WS cli      projection purge+refill"
	nWSCLIWrite = "WS cli      context write"
)

// workspaceContentionKey is the project every process in the run opens. One key
// is the whole point: sixteen checkouts, one context store.
const workspaceContentionKey = workspace.ProjectKey("prj_contention")

// gatedStreams are the streams the latency half of the pass bar is read from:
// the small writes an observation, a decision and a registration make. The
// count half covers every write in the run.
//
// Two streams are reported rather than gated, for opposite reasons. The
// desktop's poll is a read. The writes that teach the content memory (the
// agents' promotions and the CLI's between-pass write) are not small: a
// content-memory Add maintains the FTS5 tables row by row, and on a
// dogfood-sized store it costs 32 ms at the median with a SINGLE writer and
// nobody else on the file. Gating it would measure that cost, not contention,
// and the run already reports it beside the numbers it is compared against.
//
// The CLI's purge-and-refill is excluded from the latency half too: it is a
// transaction over thousands of rows, it is supposed to take as long as it
// takes, and it is in the run as the antagonist the small writes must not
// queue behind.
var gatedStreams = []string{nWSRegister, nWSDecision}

// countedStreams is every write in the run. All of them must succeed.
var countedStreams = []string{nWSRegister, nWSDecision, nWSMemory, nWSCLIWrite, nWSExtract}

// passBarP99Ms is the latency a small write must stay under at the 99th
// percentile.
const passBarP99Ms = 50.0

// workspaceRoot and checkoutRoot place one run's directories under the harness
// data directory.
func workspaceRoot(dir string) string { return filepath.Join(dir, string(topoWorkspace), "workspace") }

func checkoutRoot(dir, name string) string {
	return filepath.Join(dir, string(topoWorkspace), "checkouts", name)
}

func agentCheckoutName(i int) string { return fmt.Sprintf("agent-%02d", i) }

// openWorkspaceStore opens one process's view of the shared project: the
// workspace's context store, and this checkout's own projection.
func openWorkspaceStore(ctx context.Context, dir, checkout string) (*workspace.Workspace, *projectdb.DB, error) {
	ws, err := workspace.OpenLocal(ctx, workspaceRoot(dir))
	if err != nil {
		return nil, nil, fmt.Errorf("open workspace: %w", err)
	}
	cdb, err := ws.Context(ctx, workspaceContentionKey)
	if err != nil {
		_ = ws.Close()
		return nil, nil, fmt.Errorf("open context store: %w", err)
	}
	root := checkoutRoot(dir, checkout)
	if err := os.MkdirAll(filepath.Join(root, project.StateDirName), 0o755); err != nil {
		_ = ws.Close()
		return nil, nil, fmt.Errorf("create checkout %s: %w", checkout, err)
	}
	db, err := projectdb.Open(ctx, project.LayoutAt(root), projectdb.WithWorkspace(projectdb.Stores{
		Context: cdb, Graph: ws.Registry(),
	}))
	if err != nil {
		_ = ws.Close()
		return nil, nil, fmt.Errorf("open project store in %s: %w", checkout, err)
	}
	return ws, db, nil
}

// seedWorkspace builds the shared context store and every checkout's
// projection, strictly sequentially, so the measured run never pays for a first
// open and never races eighteen processes into one set of migrations.
func seedWorkspace(ctx context.Context, cfg config) error {
	ws, db, err := openWorkspaceStore(ctx, cfg.dir, agentCheckoutName(0))
	if err != nil {
		return fmt.Errorf("seed: %w", err)
	}
	defer func() { _ = ws.Close() }()
	defer func() { _ = db.Close() }()

	if _, err := ws.Register(ctx, workspaceContentionKey, "Contention", checkoutRoot(cfg.dir, agentCheckoutName(0))); err != nil {
		return fmt.Errorf("seed: register project: %w", err)
	}

	mem := db.Memory()
	const entryChunk = 2000
	for start := 0; start < cfg.entries; start += entryChunk {
		end := min(start+entryChunk, cfg.entries)
		batch := make([]memory.Entry, 0, end-start)
		for i := start; i < end; i++ {
			batch = append(batch, makeEntry(i))
		}
		if err := mem.BulkAddWithStream(ctx, batch, ""); err != nil {
			return fmt.Errorf("seed: bulk add entries: %w", err)
		}
	}
	if err := mem.RebuildFuzzyIndex(ctx); err != nil {
		return fmt.Errorf("seed: rebuild fuzzy index: %w", err)
	}
	if err := mem.RebuildSearchIndex(ctx); err != nil {
		return fmt.Errorf("seed: rebuild search index: %w", err)
	}
	for i := range cfg.concepts {
		if err := db.Terms().AddConcept(ctx, makeConcept(i)); err != nil {
			return fmt.Errorf("seed: add concept %d: %w", i, err)
		}
	}
	work := db.Work()
	for i := range cfg.units {
		if err := work.Put(ctx, makeUnitState(i)); err != nil {
			return fmt.Errorf("seed: put unit %d: %w", i, err)
		}
	}
	if err := work.Commit(ctx); err != nil {
		return fmt.Errorf("seed: commit working set: %w", err)
	}

	// The CLI's checkout carries the blocks its extraction purges and refills.
	// The agents' checkouts stay empty: an agent writes context, and giving each
	// of them a corpus would measure sixteen projections nobody contends on.
	cliWS, cliDB, err := openWorkspaceStore(ctx, cfg.dir, roleCLI)
	if err != nil {
		return fmt.Errorf("seed: %w", err)
	}
	defer func() { _ = cliWS.Close() }()
	defer func() { _ = cliDB.Close() }()

	const blockChunk = 5000
	for start := 0; start < cfg.blocks; start += blockChunk {
		sess, err := cliDB.Blocks().Begin(ctx)
		if err != nil {
			return fmt.Errorf("seed: begin block session: %w", err)
		}
		end := min(start+blockChunk, cfg.blocks)
		for i := start; i < end; i++ {
			if err := sess.PutBlock(collectionOf(i), makeBlock(i)); err != nil {
				_ = sess.Rollback()
				return fmt.Errorf("seed: put block %d: %w", i, err)
			}
		}
		if err := sess.Commit(); err != nil {
			return fmt.Errorf("seed: commit blocks: %w", err)
		}
	}

	// Every agent's projection, opened once so the measured run finds its
	// migrations done.
	for i := range cfg.agents {
		aws, adb, err := openWorkspaceStore(ctx, cfg.dir, agentCheckoutName(i))
		if err != nil {
			return fmt.Errorf("seed: %w", err)
		}
		_ = adb.Close()
		_ = aws.Close()
	}
	return nil
}

// runWorkspaceTopology drives the whole run from the parent, which starts every
// workload as a real process and does no store work itself.
//
// Every writer is out of process on purpose. The in-process write gate can only
// order writers holding the same handle, so measuring it with goroutines would
// flatter the design by removing exactly the contention the workspace
// introduces: eighteen operating-system processes on one set of files.
func runWorkspaceTopology(ctx context.Context, cfg config) ([]streamStats, error) {
	children := make([]*childHandle, 0, cfg.agents+2)
	start := func(role string, checkout string) error {
		c, err := startWorkspaceChild(ctx, role, checkout, cfg)
		if err != nil {
			return err
		}
		children = append(children, c)
		return nil
	}
	for i := range cfg.agents {
		if err := start(roleAgent, agentCheckoutName(i)); err != nil {
			return nil, err
		}
	}
	if err := start(roleDesktop, agentCheckoutName(0)); err != nil {
		return nil, err
	}
	if err := start(roleCLI, roleCLI); err != nil {
		return nil, err
	}

	// Child reports carry the same stream names, so merging them is a sum per
	// name rather than eighteen near-identical rows.
	merged := newRecorder()
	for _, c := range children {
		stats, err := c.wait()
		if err != nil {
			return nil, err
		}
		for _, s := range stats {
			mergeStream(merged.stream(s.Name), s)
		}
	}
	return merged.all(), nil
}

// mergeStream folds one process's report into the run's: counts add, the
// maximum is the worst any process saw, and the latency samples are pooled so
// the run's quantiles are the quantiles of every observation rather than an
// average of eighteen small ones.
func mergeStream(dst *stream, src streamStats) {
	dst.mu.Lock()
	defer dst.mu.Unlock()
	dst.ops += src.Ops
	dst.busy += src.Busy
	dst.other += src.Other
	dst.cancelled += src.Cancelled
	if d := time.Duration(src.Max * float64(time.Millisecond)); d > dst.max {
		dst.max = d
	}
	for _, v := range src.Samples {
		dst.samples = append(dst.samples, time.Duration(v*float64(time.Millisecond)))
		dst.seen++
	}
	dst.total += time.Duration(src.Mean*float64(time.Millisecond)) * time.Duration(max(src.Ops, 1))
}

// startWorkspaceChild re-execs this binary in one of the workspace roles.
func startWorkspaceChild(ctx context.Context, role, checkout string, cfg config) (*childHandle, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("locate self for %s child: %w", role, err)
	}
	cmd := exec.CommandContext(ctx, exe,
		"-child-role="+role,
		"-mode="+string(topoWorkspace),
		"-dir="+cfg.dir,
		"-checkout="+checkout,
		"-duration="+cfg.duration.String(),
		fmt.Sprintf("-agent-hz=%d", cfg.agentHz),
		fmt.Sprintf("-memory-every=%d", cfg.memoryEvery),
		fmt.Sprintf("-status-hz=%d", cfg.statusHz),
		fmt.Sprintf("-blocks=%d", cfg.blocks),
		fmt.Sprintf("-entries=%d", cfg.entries),
		fmt.Sprintf("-units=%d", cfg.units),
		fmt.Sprintf("-extract-blocks=%d", cfg.extractN),
		"-extract-every="+cfg.extractTick.String(),
	)
	var out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s child: %w", role, err)
	}
	return &childHandle{role: role, cmd: cmd, out: &out}, nil
}

// runAgentChild is one agent's whole life: register in the workspace, then
// write the small transactions an observation and a decision make, at a steady
// rate, into the store sixteen siblings are writing at the same time.
func runAgentChild(ctx context.Context, checkout string, cfg config) error {
	rec := newRecorder()
	for _, n := range []string{nWSRegister, nWSDecision, nWSMemory} {
		rec.stream(n)
	}
	ws, db, err := openWorkspaceStore(ctx, cfg.dir, checkout)
	if err != nil {
		return fmt.Errorf("agent %s: %w", checkout, err)
	}
	defer func() { _ = ws.Close() }()
	defer func() { _ = db.Close() }()

	runCtx, cancel := context.WithTimeout(ctx, cfg.duration)
	defer cancel()

	_ = rec.observe(nWSRegister, func() error {
		_, rerr := ws.Register(runCtx, workspaceContentionKey, "Contention", checkoutRoot(cfg.dir, checkout))
		return rerr
	})

	work, mem := db.Work(), db.Memory()
	seed := int(fnv(checkout))

	// Independent processes do not arrive in lockstep, and a run that models
	// them with sixteen tickers started at the same instant measures a
	// thundering herd instead: every agent's write lands in the same
	// millisecond, so each one queues behind fifteen siblings and the tail is
	// the burst rather than the store. The period is jittered per write, and the
	// first tick is offset by this agent's own fraction of it.
	period := time.Second / time.Duration(max(cfg.agentHz, 1))
	rnd := rand.New(rand.NewPCG(uint64(seed), 0x5eed))
	wait := func(d time.Duration) bool {
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-runCtx.Done():
			return false
		case <-t.C:
			return true
		}
	}
	if !wait(time.Duration(rnd.Int64N(int64(period)))) {
		return emitReport(rec.all())
	}

	for i := 0; ; i++ {
		if i > 0 && !wait(period/2+time.Duration(rnd.Int64N(int64(period)))) {
			return emitReport(rec.all())
		}
		u := makeUnitState((seed + i) % max(cfg.units, 1))
		u.Status = model.TargetStatusReviewed
		_ = rec.observe(nWSDecision, func() error { return work.Put(runCtx, u) })

		// Teaching the content memory is a PROMOTION, not an observation: it
		// happens when a decision blesses wording worth reusing, which is a
		// fraction of the decisions. Modelling it at the decision rate measures
		// a converge run rather than an agent, and the two have very different
		// costs — a memory Add maintains the FTS5 tables row by row and grows
		// with the corpus, where a unit-state write is one upsert.
		if cfg.memoryEvery > 0 && i%cfg.memoryEvery == 0 {
			e := makeEntry((seed + i) % max(cfg.entries, 1))
			e.Variants[model.LocaleID("nb")] = []model.Run{
				model.TextR(fmt.Sprintf("promotion %d from %s", i, checkout)),
			}
			_ = rec.observe(nWSMemory, func() error { return mem.Add(runCtx, e) })
		}
	}
}

// runDesktopChild polls the shared context store for change, the way a desktop
// window redraws itself. It never writes: under WAL a reader neither blocks a
// writer nor waits for one, and this row exists to show that staying true with
// eighteen processes on the file.
func runDesktopChild(ctx context.Context, checkout string, cfg config) error {
	rec := newRecorder()
	rec.stream(nWSPoll)

	ws, db, err := openWorkspaceStore(ctx, cfg.dir, checkout)
	if err != nil {
		return fmt.Errorf("desktop: %w", err)
	}
	defer func() { _ = ws.Close() }()
	defer func() { _ = db.Close() }()

	runCtx, cancel := context.WithTimeout(ctx, cfg.duration)
	defer cancel()
	tick := time.NewTicker(time.Second / time.Duration(max(cfg.statusHz, 1)))
	defer tick.Stop()
	for {
		select {
		case <-runCtx.Done():
			return emitReport(rec.all())
		case <-tick.C:
		}
		// The two reads a window redraws from: how much this checkout has
		// recorded and not yet written to the record, and what the workspace
		// holds. Both are spelled as SQL rather than taken through core/state,
		// because RecordDiff imports first and this process is the run's one
		// reader.
		_ = rec.observe(nWSPoll, func() error {
			var unwritten int
			if err := db.Raw().QueryRowContext(runCtx,
				`SELECT COUNT(*) FROM unit_view WHERE exported = 0`).Scan(&unwritten); err != nil {
				return err
			}
			var projects int
			return ws.Registry().QueryRowContext(runCtx,
				`SELECT COUNT(*) FROM workspace_projects`).Scan(&projects)
		})
	}
}

// runCLIChild is the run the split is about: a long purge-and-refill
// transaction over this checkout's own projection, with a context write between
// passes.
//
// The projection transaction holds a file no agent touches. If the agents'
// latencies move while it is in flight, the two pools are not as independent as
// the design says they are.
func runCLIChild(ctx context.Context, cfg config) error {
	rec := newRecorder()
	for _, n := range []string{nWSExtract, nWSCLIWrite} {
		rec.stream(n)
	}
	ws, db, err := openWorkspaceStore(ctx, cfg.dir, roleCLI)
	if err != nil {
		return fmt.Errorf("cli: %w", err)
	}
	defer func() { _ = ws.Close() }()
	defer func() { _ = db.Close() }()

	runCtx, cancel := context.WithTimeout(ctx, cfg.duration)
	defer cancel()
	tick := time.NewTicker(cfg.extractTick)
	defer tick.Stop()
	for pass := 0; ; pass++ {
		select {
		case <-runCtx.Done():
			return emitReport(rec.all())
		case <-tick.C:
		}
		start := pass * cfg.extractN
		_ = rec.observe(nWSExtract, func() error {
			sess, serr := db.Blocks().Begin(runCtx)
			if serr != nil {
				return serr
			}
			for i := start; i < start+cfg.extractN; i++ {
				idx := i % max(cfg.blocks, 1)
				if err := sess.PutBlock(collectionOf(idx), makeBlock(idx)); err != nil {
					_ = sess.Rollback()
					return err
				}
			}
			return sess.Commit()
		})
		// A converge run teaches the content memory as it writes overlays, and
		// that write goes to the other pool. It is here to prove the sequence is
		// legal: the block session has committed, so the projection's permit is
		// free, and the context store's was never taken.
		e := makeEntry(pass % max(cfg.entries, 1))
		_ = rec.observe(nWSCLIWrite, func() error { return db.Memory().Add(runCtx, e) })
	}
}

// reportWorkspaceBar prints the pass bar's verdict beside the numbers it read.
func reportWorkspaceBar(stats []streamStats) bool {
	byName := map[string]streamStats{}
	for _, s := range stats {
		byName[s.Name] = s
	}
	var failed int64
	for _, name := range countedStreams {
		failed += byName[name].Busy + byName[name].Other
	}
	worst := 0.0
	worstName := ""
	for _, name := range gatedStreams {
		if s := byName[name]; s.P99 > worst {
			worst, worstName = s.P99, name
		}
	}
	pass := failed == 0 && worst < passBarP99Ms
	verdict := "PASS"
	if !pass {
		verdict = "FAIL"
	}
	fmt.Printf("\npass bar: zero failed writes and small-write p99 under %.0f ms → %s\n", passBarP99Ms, verdict)
	fmt.Printf("  failed writes, every stream: %d\n", failed)
	if worstName != "" {
		fmt.Printf("  worst small-write p99: %.2f ms (%s)\n", worst, worstName)
	}
	fmt.Printf("  reported, not gated: teaching the content memory costs what FTS5 maintenance costs "+
		"(%.2f ms p50, %.2f ms p99 here)\n", byName[nWSMemory].P50, byName[nWSMemory].P99)
	fmt.Printf("  the CLI's projection transaction, which the small writes must not queue behind: "+
		"%.2f ms p99 over %d passes\n", byName[nWSExtract].P99, byName[nWSExtract].Ops)
	return pass
}

// workspaceDBSizes reports the files one run left behind: the workspace's two,
// and a couple of the checkouts' projections.
func workspaceDBSizes(cfg config) map[string]int64 {
	out := map[string]int64{}
	add := func(label, path string) {
		if fi, err := os.Stat(path); err == nil {
			out[label] = fi.Size()
		}
	}
	root := workspaceRoot(cfg.dir)
	add(workspace.RegistryFileName, filepath.Join(root, workspace.RegistryFileName))
	add("context/"+string(workspaceContentionKey)+".db",
		filepath.Join(root, workspace.ProjectsDirName, workspace.FileNameFor(workspaceContentionKey)+".db"))
	add("cli projection", project.LayoutAt(checkoutRoot(cfg.dir, roleCLI)).StorePath())
	add("agent-00 projection", project.LayoutAt(checkoutRoot(cfg.dir, agentCheckoutName(0))).StorePath())
	return out
}

// fnv is a small hash, used to give each agent a different slice of the corpus
// so sixteen processes do not all rewrite the same row.
func fnv(s string) uint32 {
	const prime = 16777619
	h := uint32(2166136261)
	for i := range len(s) {
		h ^= uint32(s[i])
		h *= prime
	}
	return h
}

// runWorkspaceChild dispatches a workspace child to its role.
func runWorkspaceChild(ctx context.Context, role, checkout string, cfg config) error {
	switch role {
	case roleAgent:
		return runAgentChild(ctx, checkout, cfg)
	case roleDesktop:
		return runDesktopChild(ctx, checkout, cfg)
	case roleCLI:
		return runCLIChild(ctx, cfg)
	default:
		return fmt.Errorf("unknown workspace role %q", role)
	}
}

// measureWorkspace seeds one run and drives it.
func measureWorkspace(ctx context.Context, cfg config) (result, error) {
	cfg.mode = topoWorkspace
	root := filepath.Join(cfg.dir, string(topoWorkspace))
	if err := os.RemoveAll(root); err != nil {
		return result{}, fmt.Errorf("clear %s: %w", root, err)
	}

	fmt.Printf("\nseeding %s …\n", topoWorkspace)
	seedStart := time.Now()
	if err := seedWorkspace(ctx, cfg); err != nil {
		return result{}, err
	}
	seedTook := time.Since(seedStart)
	fmt.Printf("seeded in %.1fs; measuring for %s with %d agent processes, "+
		"1 desktop reader and 1 CLI run …\n", seedTook.Seconds(), cfg.duration, cfg.agents)

	stats, err := runWorkspaceTopology(ctx, cfg)
	if err != nil {
		return result{}, err
	}
	r := result{mode: topoWorkspace, seed: seedTook, stats: stats, dbBytes: workspaceDBSizes(cfg)}
	for _, s := range stats {
		r.totalErr += s.Busy + s.Other
	}
	return r, nil
}

// errWorkspaceBar reports a run that missed the bar, so the command exits
// non-zero and a pipeline notices.
var errWorkspaceBar = errors.New("the workspace run missed its pass bar")
