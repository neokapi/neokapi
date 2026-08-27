// Command conversioneval compares document converters against ground truth
// read from the documents themselves.
//
// The card this answers said the eval needs "independent ground truth, meaning
// rendered pages rather than another tool's output". OOXML gives a second kind
// of independence that is cheaper and no less real: the format spec designates
// which elements carry text, so the document states its own contents and no
// converter's output has to stand in for the answer.
//
// What is compared is text-extraction completeness — whether a converter saw
// what the file says is there — plus how long each one took. Structure and
// ordering are not compared; groundtruth.go says why.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// DefaultOut is the dataset the dashboard reads.
const DefaultOut = "web/src/pages/conversion-eval/_conversioneval.json"

// FileResult is one converter on one document.
type FileResult struct {
	Converter         string `json:"converter"`
	File              string `json:"file"`
	Ext               string `json:"ext"`
	Bytes             int64  `json:"bytes"`
	MillisecondsTaken int64  `json:"ms"`
	Recall
	Error string `json:"error,omitempty"`
}

// ConverterResult is one converter over the whole corpus.
type ConverterResult struct {
	Converter
	// Files is how many documents this converter was asked to read, which is
	// not the corpus size: each converter is asked only for the extensions it
	// claims.
	Files int `json:"files"`
	// Failed is how many it could not read at all. A failure is not scored as
	// zero recall — losing everything and refusing are different, and averaging
	// them together lets a tool that crashes on the hard files outscore one
	// that tries.
	Failed int `json:"failed"`
	// Recall is the headline: every word the corpus declares, against every one
	// the converter produced. Micro-averaged, and that is not a detail.
	//
	// Averaging per-file recalls gives each document one vote regardless of
	// size, and this corpus is full of two-word fixtures — 1200-5.docx declares
	// exactly "one" and "واحد", so missing either scores the file 0% and drags
	// the average down as hard as losing a whole report would. Every converter
	// scored 0 on it. Weighting by words asks the question a reader actually
	// has: of all the text in this corpus, how much came through.
	Recall float64 `json:"recall"`
	// TruthWords and MatchedWords are what Recall is computed from, so the
	// arithmetic can be checked rather than taken.
	TruthWords   int `json:"truthWords"`
	MatchedWords int `json:"matchedWords"`
	// MeanRecall and MedianRecall are per file, over documents big enough for a
	// per-file score to mean anything. Kept because they answer a different
	// question — the typical document rather than the typical word — and
	// because a mean far below the median names a few disasters.
	MeanRecall   float64 `json:"meanRecall"`
	MedianRecall float64 `json:"medianRecall"`
	// Tiny counts documents below the floor, excluded from the per-file stats
	// and included in the word-weighted one. Reported rather than dropped
	// quietly.
	Tiny int `json:"tiny"`
	// Perfect counts documents where nothing was lost.
	Perfect int `json:"perfect"`
	// MedianMs is per document, so converters that differ in startup cost can
	// be compared on the work rather than on the launch.
	MedianMs int64 `json:"medianMs"`
	TotalMs  int64 `json:"totalMs"`
	// Worst is the documents it lost the most on, so a number has examples.
	Worst []FileResult `json:"worst,omitempty"`
	// ByExt splits the score, because a converter can be strong on one format
	// and absent on another.
	ByExt map[string]ExtScore `json:"byExt"`
}

// ExtScore is one converter's score for one extension.
type ExtScore struct {
	Files        int     `json:"files"`
	Failed       int     `json:"failed"`
	Recall       float64 `json:"recall"`
	TruthWords   int     `json:"truthWords"`
	MatchedWords int     `json:"matchedWords"`
	MeanRecall   float64 `json:"meanRecall"`
	MedianRecall float64 `json:"medianRecall"`
	MedianMs     int64   `json:"medianMs"`
}

// CorpusInfo describes what was read.
type CorpusInfo struct {
	// Source names where the documents came from, and it is the point: they
	// were not written for this eval, and not by us.
	Source string         `json:"source"`
	Files  int            `json:"files"`
	ByExt  map[string]int `json:"byExt"`
	// Sampled says plainly when the run did not read everything. A limit that
	// is not reported reads as full coverage.
	Sampled   bool `json:"sampled"`
	Available int  `json:"available"`
}

// Report is the dataset.
type Report struct {
	Note   string            `json:"_note"`
	Date   string            `json:"date"`
	Corpus CorpusInfo        `json:"corpus"`
	Method string            `json:"method"`
	Limits string            `json:"limits"`
	Result []ConverterResult `json:"converters"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "conversioneval:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		out     = flag.String("out", "", "dataset path (default: "+DefaultOut+" under the repo root)")
		corpus  = flag.String("corpus", "", "corpus root (default: .parity/okapi-testdata/<version>)")
		limit   = flag.Int("limit", 60, "documents per extension (0 = all)")
		jobs    = flag.Int("jobs", 4, "documents converted in parallel")
		perFile = flag.Duration("timeout", 60*time.Second, "per-conversion timeout")
		date    = flag.String("date", "", "date to stamp (default: today)")
	)
	flag.Parse()

	root, err := repoRoot()
	if err != nil {
		return err
	}
	if *date == "" {
		*date = time.Now().UTC().Format("2006-01-02")
	}
	target := *out
	if target == "" {
		target = filepath.Join(root, DefaultOut)
	}
	corpusRoot := *corpus
	if corpusRoot == "" {
		corpusRoot, err = findCorpus(root)
		if err != nil {
			return err
		}
	}

	ctx := context.Background()
	convs := available(ctx, findKapi(root))
	if len(convs) == 0 {
		return errors.New("no converters found on this machine")
	}
	fmt.Fprintf(os.Stderr, "converters: %s\n", strings.Join(ids(convs), ", "))

	docs, total, err := discover(corpusRoot, *limit)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "corpus: %d of %d documents under %s\n",
		len(docs), total, mustRel(root, corpusRoot))

	results := measure(ctx, convs, docs, *jobs, *perFile)

	rep := Report{
		Note: "GENERATED by `go run ./scripts/conversioneval`. Do not edit by hand. " +
			"Ground truth is read from each document's own XML parts, not from any converter's output.",
		Date:   *date,
		Corpus: describe(corpusRoot, docs, total),
		Method: "Multiset word recall against the text nodes each document's own parts declare, per the OOXML spec. " +
			"Words of one character are dropped from both sides. The headline is word-weighted across the corpus, not " +
			"an average of per-file scores: this corpus holds two-word fixtures, and giving those the same weight as a " +
			"full report is how the first version of this table got its numbers. Recall only — a converter that emits " +
			"extra text is doing something different from one that loses it, and this measures loss.",
		Limits: "Structure and ordering are not compared, so a converter that flattens every heading scores the same " +
			"as one that keeps the outline. Headers, footers, footnotes, comments and speaker notes are excluded, " +
			"because converters disagree about whether those belong in the output and counting them would score that " +
			"disagreement. The four tools are not the same kind of thing — see each row's note.",
		Result: results,
	}

	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(target, append(data, '\n'), 0o644); err != nil {
		return err
	}
	report(results)
	fmt.Fprintf(os.Stderr, "wrote %s\n", target)
	return nil
}

// doc is one corpus document.
type doc struct {
	path string
	ext  string
	size int64
}

// discover walks the corpus and takes up to limit documents per extension.
//
// Sorted before sampling, so "the first 60" is the same 60 on every machine and
// two runs are comparable. A random sample would be better statistics and worse
// evidence: nobody can check a number they cannot reproduce.
func discover(root string, limit int) ([]doc, int, error) {
	byExt := map[string][]doc{}
	total := 0
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil //nolint:nilerr // an unreadable corner of the corpus is not a reason to stop
		}
		ext := strings.ToLower(filepath.Ext(p))
		if _, ok := specs[ext]; !ok {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil //nolint:nilerr
		}
		total++
		byExt[ext] = append(byExt[ext], doc{path: p, ext: ext, size: info.Size()})
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	var out []doc
	for _, ext := range supportedExts() {
		list := byExt[ext]
		sort.Slice(list, func(i, j int) bool { return list[i].path < list[j].path })
		if limit > 0 && len(list) > limit {
			list = list[:limit]
		}
		out = append(out, list...)
	}
	if len(out) == 0 {
		return nil, 0, fmt.Errorf("no %s documents under %s", strings.Join(supportedExts(), "/"), root)
	}
	return out, total, nil
}

// measure runs every converter over every document it claims.
func measure(ctx context.Context, convs []Converter, docs []doc, jobs int, perFile time.Duration) []ConverterResult {
	workdir, err := os.MkdirTemp("", "conveval-")
	if err != nil {
		panic(err)
	}
	defer func() { _ = os.RemoveAll(workdir) }()

	// Ground truth once per document, shared across converters: it is the same
	// answer for all of them, and reading it four times would make the timing
	// table a measurement of the zip reader.
	truth := make(map[string][]string, len(docs))
	for _, d := range docs {
		t, err := groundTruth(d.path, d.ext)
		if err != nil {
			continue
		}
		truth[d.path] = t
	}

	out := make([]ConverterResult, len(convs))
	for i, c := range convs {
		fmt.Fprintf(os.Stderr, "  %s …", c.ID)
		var (
			mu   sync.Mutex
			list []FileResult
			wg   sync.WaitGroup
		)
		sem := make(chan struct{}, max(1, jobs))
		for _, d := range docs {
			if !c.handles(d.ext) {
				continue
			}
			t, ok := truth[d.path]
			if !ok || len(t) == 0 {
				// No ground truth means nothing to score against. Skipped
				// rather than counted, so a document whose text this eval
				// cannot read does not become every converter's failure.
				continue
			}
			wg.Add(1)
			sem <- struct{}{}
			go func(d doc, t []string) {
				defer wg.Done()
				defer func() { <-sem }()
				r := convertOne(ctx, c, d, t, workdir, perFile)
				mu.Lock()
				list = append(list, r)
				mu.Unlock()
			}(d, t)
		}
		wg.Wait()
		sort.Slice(list, func(i, j int) bool { return list[i].File < list[j].File })
		out[i] = summarize(c, list)
		fmt.Fprintf(os.Stderr, " %d files, %.1f%% of %d words, %d failed\n",
			out[i].Files, 100*out[i].Recall, out[i].TruthWords, out[i].Failed)
	}
	return out
}

func convertOne(ctx context.Context, c Converter, d doc, truth []string, workdir string, perFile time.Duration) FileResult {
	r := FileResult{Converter: c.ID, File: filepath.Base(d.path), Ext: d.ext, Bytes: d.size}
	cctx, cancel := context.WithTimeout(ctx, perFile)
	defer cancel()

	start := time.Now()
	text, err := c.run(cctx, d.path, workdir)
	r.MillisecondsTaken = time.Since(start).Milliseconds()
	if err != nil {
		r.Error = clip(err.Error(), 200)
		return r
	}
	r.Recall = score(truth, text)
	return r
}

// tinyFloor is the ground-truth word count below which a per-file recall is
// noise. A document declaring three words can only score 0, 33, 67 or 100.
const tinyFloor = 20

func summarize(c Converter, list []FileResult) ConverterResult {
	res := ConverterResult{Converter: c, ByExt: map[string]ExtScore{}}
	byExt := map[string][]FileResult{}
	var recalls []float64
	var msAll []int64

	for _, f := range list {
		res.Files++
		byExt[f.Ext] = append(byExt[f.Ext], f)
		if f.Error != "" {
			res.Failed++
			continue
		}
		res.TruthWords += f.Recall.TruthWords
		res.MatchedWords += f.Recall.Matched
		msAll = append(msAll, f.MillisecondsTaken)
		res.TotalMs += f.MillisecondsTaken
		if f.Recall.Recall >= 1 {
			res.Perfect++
		}
		if f.Recall.TruthWords < tinyFloor {
			res.Tiny++
			continue
		}
		recalls = append(recalls, f.Recall.Recall)
	}
	if res.TruthWords > 0 {
		res.Recall = float64(res.MatchedWords) / float64(res.TruthWords)
	}
	res.MeanRecall, res.MedianRecall = meanOf(recalls), medianOf(recalls)
	res.MedianMs = medianInt(msAll)

	for ext, fs := range byExt {
		var rs []float64
		var ms []int64
		failed, truth, matched := 0, 0, 0
		for _, f := range fs {
			if f.Error != "" {
				failed++
				continue
			}
			truth += f.Recall.TruthWords
			matched += f.Recall.Matched
			ms = append(ms, f.MillisecondsTaken)
			if f.Recall.TruthWords >= tinyFloor {
				rs = append(rs, f.Recall.Recall)
			}
		}
		e := ExtScore{
			Files: len(fs), Failed: failed, TruthWords: truth, MatchedWords: matched,
			MeanRecall: meanOf(rs), MedianRecall: medianOf(rs), MedianMs: medianInt(ms),
		}
		if truth > 0 {
			e.Recall = float64(matched) / float64(truth)
		}
		res.ByExt[ext] = e
	}

	// The worst files, so a mean has examples under it. Failures first — a file
	// a converter refused is more interesting than one it read badly.
	worst := append([]FileResult(nil), list...)
	sort.SliceStable(worst, func(i, j int) bool {
		fi, fj := worst[i].Error != "", worst[j].Error != ""
		if fi != fj {
			return fi
		}
		return worst[i].Recall.Recall < worst[j].Recall.Recall
	})
	for _, f := range worst {
		if len(res.Worst) >= 8 || (f.Error == "" && f.Recall.Recall >= 1) {
			break
		}
		res.Worst = append(res.Worst, f)
	}
	return res
}

func describe(root string, docs []doc, total int) CorpusInfo {
	by := map[string]int{}
	for _, d := range docs {
		by[d.ext]++
	}
	return CorpusInfo{
		Source: "The Okapi Framework's own integration-test resources (okapi-testdata " +
			filepath.Base(root) + "). Real documents, collected by another project for another purpose, " +
			"which is what makes them a fair test rather than a demonstration.",
		Files:     len(docs),
		ByExt:     by,
		Sampled:   len(docs) < total,
		Available: total,
	}
}

// report leads with the per-extension tables, because they are the only fair
// comparison here.
//
// The converters do not read the same file set: textutil takes Word documents
// only, LibreOffice has no text target for Impress, pandoc does not read
// spreadsheets. Their corpus-wide numbers are therefore computed over different
// documents, and putting them in one column would rank tools by which formats
// they declined. Within one extension every converter saw the same files.
func report(rs []ConverterResult) {
	exts := map[string]bool{}
	for _, r := range rs {
		for e := range r.ByExt {
			exts[e] = true
		}
	}
	for _, ext := range sortedKeys(exts) {
		fmt.Printf("\n%s\n", ext)
		fmt.Printf("  %-14s %6s %8s %9s %7s %7s %8s %7s %9s\n",
			"converter", "files", "recall", "words", "mean", "median", "perfect", "failed", "median ms")
		for _, r := range rs {
			e, ok := r.ByExt[ext]
			if !ok {
				continue
			}
			fmt.Printf("  %-14s %6d %7.1f%% %9d %6.1f%% %6.1f%% %8s %7d %9d\n",
				r.ID, e.Files, 100*e.Recall, e.TruthWords,
				100*e.MeanRecall, 100*e.MedianRecall, "-", e.Failed, e.MedianMs)
		}
	}
	fmt.Printf("\nacross every document each tool accepted (not a ranking: the file sets differ)\n")
	fmt.Printf("  %-14s %6s %8s %9s %8s %7s\n", "converter", "files", "recall", "words", "perfect", "failed")
	for _, r := range rs {
		fmt.Printf("  %-14s %6d %7.1f%% %9d %8d %7d\n",
			r.ID, r.Files, 100*r.Recall, r.TruthWords, r.Perfect, r.Failed)
	}
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func ids(cs []Converter) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.ID + " " + c.Version
	}
	return out
}

func repoRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", errors.New("not in a git checkout")
	}
	return strings.TrimSpace(string(out)), nil
}

func findKapi(root string) string {
	local := filepath.Join(root, "bin", "kapi")
	if _, err := os.Stat(local); err == nil {
		return local
	}
	if p, err := exec.LookPath("kapi"); err == nil {
		return p
	}
	return ""
}

// findCorpus locates the extracted okapi-testdata tree the parity harness
// already downloads, rather than adding a second copy of it.
func findCorpus(root string) (string, error) {
	base := filepath.Join(root, ".parity", "okapi-testdata")
	entries, err := os.ReadDir(base)
	if err != nil {
		return "", fmt.Errorf("no corpus at %s: run `make parity-fetch` or pass -corpus", base)
	}
	var versions []string
	for _, e := range entries {
		if e.IsDir() {
			versions = append(versions, e.Name())
		}
	}
	if len(versions) == 0 {
		return "", fmt.Errorf("no extracted corpus under %s", base)
	}
	sort.Strings(versions)
	return filepath.Join(base, versions[len(versions)-1]), nil
}

func mustRel(root, p string) string {
	if r, err := filepath.Rel(root, p); err == nil {
		return r
	}
	return p
}
