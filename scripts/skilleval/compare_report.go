package main

import (
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

// The report: per host and arm, the rule-violation rate, the voice score,
// tokens, turns and wall time, each with a 95% interval, and the kapi arm's
// difference from each other arm. Proportions use the Wilson interval; means
// and differences use a percentile bootstrap that resamples attempts within
// each task, so a difference is never driven by one arm drawing easier tasks.

// compareRecord is everything saved about one attempt.
type compareRecord struct {
	Result   CompareResult
	Grade    *CompareGrade
	Verdicts map[string]CompareVerdict
}

func (r compareRecord) voice() (float64, bool) {
	sum, n := 0.0, 0
	for _, verdict := range r.Verdicts {
		if verdict.complete() {
			sum += verdict.score()
			n++
		}
	}
	if n == 0 {
		return 0, false
	}
	return sum / float64(n), true
}

func loadCompareRecords(dirs []string) ([]compareRecord, error) {
	records := []compareRecord{}
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			attemptDir := filepath.Join(dir, entry.Name())
			record := compareRecord{Verdicts: map[string]CompareVerdict{}}
			if readPairedJSON(filepath.Join(attemptDir, "result.json"), &record.Result) != nil {
				continue
			}
			var grade CompareGrade
			if readPairedJSON(filepath.Join(attemptDir, "grade.json"), &grade) == nil {
				record.Grade = &grade
			}
			for _, host := range []string{"claude", "codex"} {
				var verdict CompareVerdict
				if readPairedJSON(filepath.Join(attemptDir, "judge-"+host+".json"), &verdict) == nil {
					record.Verdicts[host] = verdict
				}
			}
			records = append(records, record)
		}
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Result.Attempt.ID < records[j].Result.Attempt.ID })
	return records, nil
}

// wilson is the 95% Wilson score interval for k successes in n.
func wilson(k, n int) (lo, hi float64) {
	if n == 0 {
		return math.NaN(), math.NaN()
	}
	const z = 1.959963984540054
	p := float64(k) / float64(n)
	denominator := 1 + z*z/float64(n)
	centre := (p + z*z/(2*float64(n))) / denominator
	half := z * math.Sqrt(p*(1-p)/float64(n)+z*z/(4*float64(n)*float64(n))) / denominator
	return math.Max(0, centre-half), math.Min(1, centre+half)
}

// stratum is one task's values for one group.
type stratum map[string][]float64

func (s stratum) add(task string, value float64) { s[task] = append(s[task], value) }

func (s stratum) mean() float64 {
	sum, n := 0.0, 0
	for _, values := range s {
		for _, v := range values {
			sum += v
			n++
		}
	}
	if n == 0 {
		return math.NaN()
	}
	return sum / float64(n)
}

func (s stratum) count() int {
	n := 0
	for _, values := range s {
		n += len(values)
	}
	return n
}

func (s stratum) all() []float64 {
	out := []float64{}
	for _, task := range s.tasks() {
		out = append(out, s[task]...)
	}
	return out
}

func (s stratum) tasks() []string {
	tasks := make([]string, 0, len(s))
	for task := range s {
		tasks = append(tasks, task)
	}
	slices.Sort(tasks)
	return tasks
}

// resample draws each task's values with replacement.
func (s stratum) resample(r *rand.Rand) stratum {
	out := stratum{}
	for _, task := range s.tasks() {
		values := s[task]
		drawn := make([]float64, len(values))
		for i := range drawn {
			drawn[i] = values[r.IntN(len(values))]
		}
		out[task] = drawn
	}
	return out
}

const compareBootstrap = 2000

// bootstrapMean is a mean with its 95% percentile-bootstrap interval.
func bootstrapMean(s stratum, seed uint64) (mean, lo, hi float64) {
	mean = s.mean()
	if s.count() < 2 {
		return mean, math.NaN(), math.NaN()
	}
	r := rand.New(rand.NewPCG(seed, 1))
	draws := make([]float64, compareBootstrap)
	for i := range draws {
		draws[i] = s.resample(r).mean()
	}
	lo, hi = percentileInterval(draws)
	return mean, lo, hi
}

// bootstrapDiff is mean(a) - mean(b) over the tasks both arms ran, with its
// 95% interval; each arm is resampled within each task.
func bootstrapDiff(a, b stratum, seed uint64) (diff, lo, hi float64) {
	shared := func(s stratum) stratum {
		out := stratum{}
		for task, values := range s {
			if len(a[task]) > 0 && len(b[task]) > 0 {
				out[task] = values
			}
		}
		return out
	}
	a, b = shared(a), shared(b)
	diff = a.mean() - b.mean()
	if a.count() < 2 || b.count() < 2 {
		return diff, math.NaN(), math.NaN()
	}
	r := rand.New(rand.NewPCG(seed, 2))
	draws := make([]float64, compareBootstrap)
	for i := range draws {
		draws[i] = a.resample(r).mean() - b.resample(r).mean()
	}
	lo, hi = percentileInterval(draws)
	return diff, lo, hi
}

func percentileInterval(draws []float64) (lo, hi float64) {
	clean := slices.DeleteFunc(slices.Clone(draws), math.IsNaN)
	if len(clean) == 0 {
		return math.NaN(), math.NaN()
	}
	slices.Sort(clean)
	at := func(q float64) float64 {
		return clean[int(math.Min(float64(len(clean)-1), math.Max(0, math.Round(q*float64(len(clean)-1)))))]
	}
	return at(0.025), at(0.975)
}

func median(values []float64) float64 {
	if len(values) == 0 {
		return math.NaN()
	}
	sorted := slices.Clone(values)
	slices.Sort(sorted)
	mid := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[mid]
	}
	return (sorted[mid-1] + sorted[mid]) / 2
}

// compareGroup is the attempts of one host (or every host) in one arm.
type compareGroup struct {
	Host, Arm  string
	Scheduled  int
	Completed  int
	Graded     int
	Clean      int
	TaskDone   int
	ByCategory map[string]int
	Violations stratum
	KapiCheck  stratum
	Voice      stratum
	Input      stratum
	Output     stratum
	Turns      stratum
	ToolCalls  stratum
	Seconds    stratum
	Cost       stratum
	Asked      int
	Checked    int
	Skill      int
	ReadRules  int
	Recorded   int
	Statuses   map[string]int
}

func newCompareGroup(host, arm string) *compareGroup {
	return &compareGroup{Host: host, Arm: arm, ByCategory: map[string]int{}, Statuses: map[string]int{},
		Violations: stratum{}, KapiCheck: stratum{}, Voice: stratum{}, Input: stratum{}, Output: stratum{},
		Turns: stratum{}, ToolCalls: stratum{}, Seconds: stratum{}, Cost: stratum{}}
}

func (g *compareGroup) add(r compareRecord) {
	g.Scheduled++
	g.Statuses[r.Result.Status]++
	if !r.Result.completed() {
		return
	}
	g.Completed++
	task := r.Result.Attempt.Task
	g.Input.add(task, float64(r.Result.Usage.totalInput()))
	g.Output.add(task, float64(r.Result.Usage.OutputTokens))
	if r.Result.Usage.Turns > 0 {
		g.Turns.add(task, float64(r.Result.Usage.Turns))
	}
	if r.Result.Usage.CostUSD > 0 {
		g.Cost.add(task, r.Result.Usage.CostUSD)
	}
	g.ToolCalls.add(task, float64(r.Result.ToolCalls))
	g.Seconds.add(task, float64(r.Result.DurationMS)/1000)
	if r.Result.Asked {
		g.Asked++
	}
	if r.Result.Checked {
		g.Checked++
	}
	if r.Result.SkillLoaded {
		g.Skill++
	}
	if r.Result.ReadRules {
		g.ReadRules++
	}
	g.Recorded += r.Result.Recorded
	if r.Grade == nil {
		return
	}
	g.Graded++
	g.Violations.add(task, float64(r.Grade.Violations))
	if r.Grade.Violations == 0 {
		g.Clean++
	}
	if r.Grade.TaskDone {
		g.TaskDone++
	}
	for _, category := range r.Grade.Broken {
		g.ByCategory[category]++
	}
	if r.Grade.KapiCheck.Ran {
		g.KapiCheck.add(task, float64(r.Grade.KapiCheck.Failing))
	}
	if score, ok := r.voice(); ok {
		g.Voice.add(task, score)
	}
}

func fmtCI(mean, lo, hi float64, format string) string {
	if math.IsNaN(mean) {
		return "n/a"
	}
	if math.IsNaN(lo) {
		return fmt.Sprintf(format, mean)
	}
	return fmt.Sprintf(format+" ["+format+", "+format+"]", mean, lo, hi)
}

func fmtRate(k, n int) string {
	if n == 0 {
		return "n/a"
	}
	lo, hi := wilson(k, n)
	return fmt.Sprintf("%d/%d = %.0f%% [%.0f, %.0f]", k, n, 100*float64(k)/float64(n), 100*lo, 100*hi)
}

func fmtTokens(v float64) string {
	switch {
	case math.IsNaN(v):
		return "n/a"
	case v >= 1e6:
		return fmt.Sprintf("%.2fM", v/1e6)
	case v >= 1e3:
		return fmt.Sprintf("%.0fk", v/1e3)
	}
	return fmt.Sprintf("%.0f", v)
}

// reportCompare writes the markdown report for the scope opts.Attempts names
// ("pilot", "run" or "all"; the default is run) to opts.Out or stdout.
func reportCompare(opts CompareOptions, m CompareManifest) error {
	scope := opts.Attempts
	if scope == "" {
		scope = comparePhaseRun
	}
	dirs := []string{}
	switch scope {
	case comparePhasePilot, comparePhaseRun:
		dirs = append(dirs, filepath.Join(opts.Dir, scope))
	case "all":
		dirs = comparePhaseDirs(opts)
	default:
		return fmt.Errorf("report scope must be pilot, run or all, not %q", scope)
	}
	records, err := loadCompareRecords(dirs)
	if err != nil {
		return err
	}
	text := renderCompareReport(m, scope, records)
	if opts.Out == "" {
		fmt.Print(text)
		return nil
	}
	return os.WriteFile(opts.Out, []byte(text), 0o600)
}

func renderCompareReport(m CompareManifest, scope string, records []compareRecord) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Agent comparison: %s (%s)\n\n", m.Study, scope)
	fmt.Fprintf(&b, "Generated %s from %d attempts. Arms: %s. Hosts: %s.\n\n", time.Now().UTC().Format(time.RFC3339), len(records),
		strings.Join(m.Arms, ", "), compareHostNames(m.Hosts))
	hosts := append(compareHostList(m.Hosts), "all")
	groups := map[string]*compareGroup{}
	key := func(host, arm string) string { return host + "/" + arm }
	for _, host := range hosts {
		for _, arm := range m.Arms {
			groups[key(host, arm)] = newCompareGroup(host, arm)
		}
	}
	for _, r := range records {
		groups[key(r.Result.Attempt.Host.Host, r.Result.Attempt.Arm)].add(r)
		groups[key("all", r.Result.Attempt.Arm)].add(r)
	}
	seed := m.Seed + 17

	b.WriteString("## Rule violations\n\nViolations per attempt counts the failed rule checks in what the attempt wrote (rename, rename scope, competitor, banned claim, house term, voice rule). Clean is the share of attempts with none. 95% intervals: bootstrap within task for means, Wilson for shares.\n\n")
	b.WriteString("| Host | Arm | Completed | Violations per attempt | Clean attempts | Task done | kapi check failing findings |\n|---|---|---|---|---|---|---|\n")
	for _, host := range hosts {
		for _, arm := range m.Arms {
			g := groups[key(host, arm)]
			mean, lo, hi := bootstrapMean(g.Violations, seed)
			kmean, klo, khi := bootstrapMean(g.KapiCheck, seed)
			fmt.Fprintf(&b, "| %s | %s | %d/%d | %s | %s | %s | %s |\n", host, arm, g.Completed, g.Scheduled,
				fmtCI(mean, lo, hi, "%.2f"), fmtRate(g.Clean, g.Graded), fmtRate(g.TaskDone, g.Graded), fmtCI(kmean, klo, khi, "%.2f"))
		}
	}
	b.WriteString("\n### Share of attempts breaking each kind of rule\n\n| Host | Arm | " + strings.Join(compareCategories, " | ") + " |\n|---|---|" + strings.Repeat("---|", len(compareCategories)) + "\n")
	for _, host := range hosts {
		for _, arm := range m.Arms {
			g := groups[key(host, arm)]
			cells := []string{}
			for _, category := range compareCategories {
				cells = append(cells, fmtRate(g.ByCategory[category], g.Graded))
			}
			fmt.Fprintf(&b, "| %s | %s | %s |\n", host, arm, strings.Join(cells, " | "))
		}
	}

	b.WriteString("\n## Voice fit\n\n")
	b.WriteString(renderCompareAgreement(m, records))
	b.WriteString("\n| Host | Arm | Judged | Voice score (share of yes, both judges) |\n|---|---|---|---|\n")
	for _, host := range hosts {
		for _, arm := range m.Arms {
			g := groups[key(host, arm)]
			mean, lo, hi := bootstrapMean(g.Voice, seed)
			fmt.Fprintf(&b, "| %s | %s | %d | %s |\n", host, arm, g.Voice.count(), fmtCI(mean, lo, hi, "%.2f"))
		}
	}

	b.WriteString("\n## Differences: kapi arm minus each other arm\n\nNegative violations and positive voice favour kapi. An interval that spans 0 is no measured difference.\n\n| Host | Versus | Violations per attempt | Voice score | Input tokens | Wall time (s) |\n|---|---|---|---|---|---|\n")
	for _, host := range hosts {
		k := groups[key(host, compareArmKapi)]
		if k == nil {
			continue
		}
		for _, arm := range m.Arms {
			if arm == compareArmKapi {
				continue
			}
			o := groups[key(host, arm)]
			vd, vlo, vhi := bootstrapDiff(k.Violations, o.Violations, seed)
			sd, slo, shi := bootstrapDiff(k.Voice, o.Voice, seed)
			td, tlo, thi := bootstrapDiff(k.Input, o.Input, seed)
			wd, wlo, whi := bootstrapDiff(k.Seconds, o.Seconds, seed)
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s |\n", host, arm, fmtCI(vd, vlo, vhi, "%+.2f"), fmtCI(sd, slo, shi, "%+.2f"),
				fmtCI(td/1e3, tlo/1e3, thi/1e3, "%+.0fk"), fmtCI(wd, wlo, whi, "%+.0f"))
		}
	}

	b.WriteString("\n## Cost of an attempt\n\nInput tokens include cache reads and writes. Turns are Claude's own count of model turns and Codex's count of completed turns. Cost is Claude's API-equivalent figure; the runs bill a subscription, and Codex reports no cost.\n\n")
	b.WriteString("| Host | Arm | Input tokens, mean | Input, median | Output, median | Turns, median | Tool calls, median | Wall time s, mean | Wall time s, median | Cost USD, mean |\n|---|---|---|---|---|---|---|---|---|---|\n")
	for _, host := range hosts {
		for _, arm := range m.Arms {
			g := groups[key(host, arm)]
			im, ilo, ihi := bootstrapMean(g.Input, seed)
			wm, wlo, whi := bootstrapMean(g.Seconds, seed)
			cm, clo, chi := bootstrapMean(g.Cost, seed)
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %.0f | %.0f | %s | %.0f | %s |\n", host, arm,
				fmtCI(im/1e3, ilo/1e3, ihi/1e3, "%.0fk"), fmtTokens(median(g.Input.all())), fmtTokens(median(g.Output.all())),
				median(g.Turns.all()), median(g.ToolCalls.all()), fmtCI(wm, wlo, whi, "%.0f"), median(g.Seconds.all()), fmtCI(cm, clo, chi, "%.2f"))
		}
	}

	b.WriteString("\n## What the agents did\n\n| Host | Arm | Asked kapi | Checked with kapi | Loaded the kapi skill | Records made | Read CLAUDE.md/AGENTS.md with a tool | Statuses |\n|---|---|---|---|---|---|---|---|\n")
	for _, host := range hosts {
		for _, arm := range m.Arms {
			g := groups[key(host, arm)]
			statuses := []string{}
			for status, n := range g.Statuses {
				statuses = append(statuses, fmt.Sprintf("%s %d", status, n))
			}
			slices.Sort(statuses)
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %d | %s | %s |\n", host, arm, fmtRate(g.Asked, g.Completed), fmtRate(g.Checked, g.Completed),
				fmtRate(g.Skill, g.Completed), g.Recorded, fmtRate(g.ReadRules, g.Completed), strings.Join(statuses, ", "))
		}
	}

	b.WriteString("\n## Totals\n\n")
	b.WriteString(renderCompareTotals(m, scope, records))

	b.WriteString("\n## Attempts\n\n| Attempt | Status | Violations | Broken | Task done | kapi check | Voice (claude/codex) | Input | Seconds | kapi calls |\n|---|---|---|---|---|---|---|---|---|---|\n")
	for _, r := range records {
		violations, broken, done, kcheck := "-", "", "-", "-"
		if r.Grade != nil {
			violations = strconv.Itoa(r.Grade.Violations)
			broken = strings.Join(r.Grade.Broken, ", ")
			done = strconv.FormatBool(r.Grade.TaskDone)
			kcheck = strconv.Itoa(r.Grade.KapiCheck.Failing)
		}
		voices := []string{}
		for _, host := range []string{"claude", "codex"} {
			if v, ok := r.Verdicts[host]; ok && v.complete() {
				voices = append(voices, fmt.Sprintf("%.1f", v.score()))
			} else {
				voices = append(voices, "-")
			}
		}
		calls := strings.Join(compactCalls(r.Result.KapiCalls), ", ")
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s | %s | %.0f | %s |\n", r.Result.Attempt.ID, r.Result.Status, violations, broken, done, kcheck,
			strings.Join(voices, "/"), fmtTokens(float64(r.Result.Usage.totalInput())), float64(r.Result.DurationMS)/1000, calls)
	}
	return b.String()
}

// compactCalls folds repeated calls: "context_read x2".
func compactCalls(calls []string) []string {
	counts := map[string]int{}
	order := []string{}
	for _, call := range calls {
		if counts[call] == 0 {
			order = append(order, call)
		}
		counts[call]++
	}
	out := []string{}
	for _, call := range order {
		if counts[call] > 1 {
			out = append(out, fmt.Sprintf("%s x%d", call, counts[call]))
		} else {
			out = append(out, call)
		}
	}
	return out
}

func renderCompareAgreement(m CompareManifest, records []compareRecord) string {
	if len(m.Judges) < 2 {
		return "One judge only: agreement cannot be measured, so the judged score is unvalidated.\n"
	}
	verdicts := map[string]map[string]CompareVerdict{}
	for _, r := range records {
		verdicts[r.Result.Attempt.ID] = r.Verdicts
	}
	first, second := m.Judges[0].Host, m.Judges[1].Host
	all, by := compareJudgePairs(verdicts, first, second)
	kappa := compareKappa(all[0], all[1])
	var b strings.Builder
	state := "VALIDATED"
	if math.IsNaN(kappa) || kappa < compareKappaBar || len(all[0]) < 30 {
		state = "NOT VALIDATED (needs kappa >= 0.6 over at least 30 paired answers); read the voice score as indicative only"
	}
	agree := 0
	for i := range all[0] {
		if all[0][i] == all[1][i] {
			agree++
		}
	}
	fmt.Fprintf(&b, "Judges: %s (%s) and %s (%s). Paired answers: %d, raw agreement %s, Cohen's kappa %.2f: %s.\n\n",
		first, m.Judges[0].Model, second, m.Judges[1].Model, len(all[0]), fmtRate(agree, len(all[0])), kappa, state)
	b.WriteString("| Criterion | Kappa | " + first + " yes | " + second + " yes |\n|---|---|---|---|\n")
	for _, criterion := range compareCriteria {
		pair := by[criterion.ID]
		yesA, yesB := 0, 0
		for i := range pair[0] {
			if pair[0][i] {
				yesA++
			}
			if pair[1][i] {
				yesB++
			}
		}
		fmt.Fprintf(&b, "| %s | %.2f | %s | %s |\n", criterion.ID, compareKappa(pair[0], pair[1]), fmtRate(yesA, len(pair[0])), fmtRate(yesB, len(pair[1])))
	}
	return b.String()
}

// renderCompareTotals sums what the attempts cost, and for a pilot projects
// the full run from it.
func renderCompareTotals(m CompareManifest, scope string, records []compareRecord) string {
	var b strings.Builder
	type total struct {
		attempts, completed int
		seconds, cost       float64
		input, output       int64
	}
	totals := map[string]*total{}
	for _, host := range compareHostList(m.Hosts) {
		totals[host] = &total{}
	}
	var first, last time.Time
	for _, r := range records {
		t := totals[r.Result.Attempt.Host.Host]
		if t == nil {
			continue
		}
		t.attempts++
		if r.Result.completed() {
			t.completed++
		}
		t.seconds += float64(r.Result.DurationMS) / 1000
		t.cost += r.Result.Usage.CostUSD
		t.input += r.Result.Usage.totalInput()
		t.output += r.Result.Usage.OutputTokens
		if started, err := time.Parse("20060102T150405.000000000Z", r.Result.Started); err == nil {
			if first.IsZero() || started.Before(first) {
				first = started
			}
			end := started.Add(time.Duration(r.Result.DurationMS) * time.Millisecond)
			if end.After(last) {
				last = end
			}
		}
	}
	b.WriteString("| Host | Attempts | Completed | Session time | Input tokens | Output tokens | API-equivalent cost |\n|---|---|---|---|---|---|---|\n")
	for _, host := range compareHostList(m.Hosts) {
		t := totals[host]
		cost := "not reported"
		if t.cost > 0 {
			cost = fmt.Sprintf("$%.2f", t.cost)
		}
		fmt.Fprintf(&b, "| %s | %d | %d | %s | %s | %s | %s |\n", host, t.attempts, t.completed,
			(time.Duration(t.seconds) * time.Second).Round(time.Second), fmtTokens(float64(t.input)), fmtTokens(float64(t.output)), cost)
	}
	if !first.IsZero() {
		fmt.Fprintf(&b, "\nWall clock from the first start to the last finish: %s.\n", last.Sub(first).Round(time.Second))
	}
	if scope == comparePhasePilot {
		full := len(m.Tasks) * m.Repeats * len(m.Arms) * len(m.Hosts)
		pilot := len(records)
		if pilot > 0 {
			factor := float64(full) / float64(pilot)
			fmt.Fprintf(&b, "\nProjection for the full run (%d tasks x %d repeats x %d arms x %d hosts = %d attempts, %.1fx the pilot):\n\n", len(m.Tasks), m.Repeats, len(m.Arms), len(m.Hosts), full, factor)
			for _, host := range compareHostList(m.Hosts) {
				t := totals[host]
				cost := ""
				if t.cost > 0 {
					cost = fmt.Sprintf(", about $%.0f API-equivalent", t.cost*factor)
				}
				fmt.Fprintf(&b, "- %s: about %s of session time, %s input and %s output tokens%s.\n", host,
					(time.Duration(t.seconds*factor) * time.Second).Round(time.Minute), fmtTokens(float64(t.input)*factor), fmtTokens(float64(t.output)*factor), cost)
			}
			if !first.IsZero() {
				fmt.Fprintf(&b, "- Wall clock at the pilot's concurrency: about %s.\n", (time.Duration(float64(last.Sub(first)) * factor)).Round(time.Minute))
			}
		}
	}
	return b.String()
}

func compareHostList(hosts []PairedAgentSpec) []string {
	out := []string{}
	for _, host := range hosts {
		out = append(out, host.Host)
	}
	return out
}

func compareHostNames(hosts []PairedAgentSpec) string {
	out := []string{}
	for _, host := range hosts {
		out = append(out, fmt.Sprintf("%s (%s, %s effort)", host.Host, host.Model, host.Effort))
	}
	return strings.Join(out, ", ")
}
