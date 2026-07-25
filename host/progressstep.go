package host

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mattn/go-isatty"
)

// Step-progress reporting convention.
//
// Anything that can plausibly run longer than a second says what it is doing,
// on stderr, so stdout stays exactly the machine-readable result: `kapi stats
// --json 'src/**' > out.json` still writes only JSON, and `2>/dev/null` still
// yields a clean stream. (Byte-progress for model/asset downloads is the
// separate downloadProgress abstraction in progress.go; the convergence loop
// has its own richer per-locale renderer in upprogress.go. This is the plain
// "N of M items" reporter every other long loop uses.)
//
// Three properties make it safe to attach to any bounded loop:
//
//   - Silent when it does not matter. Nothing is drawn until [ProgressDelay]
//     has elapsed, so a fast run prints nothing at all and scripted output does
//     not change for small inputs.
//   - Quiet-aware. --quiet suppresses it entirely.
//   - Degrades on a pipe. On a TTY it repaints one line in place; on a pipe or
//     CI log it emits plain, append-only lines at [ProgressLogInterval] and
//     never writes an escape sequence.

// ProgressDelay is how long an operation must run before progress appears.
// Below it, a run is fast enough that a progress line is noise.
const ProgressDelay = 900 * time.Millisecond

// ProgressLogInterval is the cadence of plain progress lines on a non-TTY
// stream, where each update is a new line rather than a repaint.
const ProgressLogInterval = 5 * time.Second

// Progress is a single-line, stderr-bound progress reporter for a bounded loop.
// A nil *Progress is a working no-op, so a caller can hold one unconditionally
// and never nil-check.
type Progress struct {
	w     io.Writer
	label string
	tty   bool
	start time.Time

	mu        sync.Mutex
	total     int
	done      int
	current   string
	drawn     bool
	lastPrint time.Time
	finished  bool
}

// NewProgress builds a progress reporter for cmd writing to its stderr. label
// names the operation ("checking", "reading"); total is the number of steps, or
// 0 when unknown (the reporter then shows a bare count rather than a bar). It
// returns nil — a working no-op — when the app is quiet or stderr is missing.
func (a *App) NewProgress(cmd Command, label string, total int) *Progress {
	if a != nil && a.Quiet {
		return nil
	}
	if cmd == nil {
		return nil
	}
	w := cmd.ErrOrStderr()
	if w == nil {
		return nil
	}
	return &Progress{
		w:     w,
		label: label,
		total: total,
		tty:   writerIsTerminal(w),
		start: time.Now(),
	}
}

// Step records that one item is being worked on and repaints if due. name is
// the item's display label; pass "" when there is nothing useful to name.
func (p *Progress) Step(name string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.finished {
		return
	}
	p.current = name
	p.render()
}

// Advance records that one item finished.
func (p *Progress) Advance() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.finished {
		return
	}
	p.done++
	p.render()
}

// SetTotal revises the step count once it becomes known (a resolver that
// discovers work as it goes).
func (p *Progress) SetTotal(total int) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.total = total
}

// Done clears the live line (TTY) or writes a closing line (pipe). It is
// idempotent, so `defer prog.Done()` alongside an explicit call is fine, and it
// prints nothing at all when the run stayed inside ProgressDelay.
func (p *Progress) Done() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.finished {
		return
	}
	p.finished = true
	if !p.drawn {
		return
	}
	if p.tty {
		fmt.Fprint(p.w, "\r\x1b[2K")
		return
	}
	fmt.Fprintf(p.w, "%s: %s in %s\n", p.label, p.countLabel(), p.elapsed())
}

// render paints the current state when it is due. Callers hold p.mu.
func (p *Progress) render() {
	if time.Since(p.start) < ProgressDelay {
		return
	}
	if p.tty {
		fmt.Fprintf(p.w, "\r\x1b[2K%s", p.line())
		p.drawn = true
		return
	}
	// A pipe gets append-only lines at a slow cadence: readable in a CI log,
	// and never a wall of text.
	if !p.drawn || time.Since(p.lastPrint) >= ProgressLogInterval {
		fmt.Fprintf(p.w, "%s: %s\n", p.label, p.countLabel())
		p.drawn = true
		p.lastPrint = time.Now()
	}
}

// line renders the in-place TTY line: label, bar, counts, current item.
func (p *Progress) line() string {
	var b strings.Builder
	b.WriteString(p.label)
	b.WriteString(" ")
	if p.total > 0 {
		b.WriteString(ProgressBar(p.done, p.total, 12))
		b.WriteString(" ")
	}
	b.WriteString(p.countLabel())
	if p.current != "" {
		b.WriteString(" · ")
		b.WriteString(truncateMiddle(p.current, 44))
	}
	return b.String()
}

func (p *Progress) countLabel() string {
	if p.total > 0 {
		return fmt.Sprintf("%d/%d", p.done, p.total)
	}
	return strconv.Itoa(p.done)
}

func (p *Progress) elapsed() string {
	return time.Since(p.start).Round(100 * time.Millisecond).String()
}

// ProgressBar renders a fixed-width block bar for done/total. It is the one bar
// renderer shared by the progress reporter and the `kapi status` pipeline
// column, so a bar means the same thing wherever it appears.
func ProgressBar(done, total, width int) string {
	if width <= 0 {
		return ""
	}
	filled := 0
	if total > 0 {
		filled = min(max(done*width/total, 0), width)
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

// truncateMiddle shortens s to at most n runes, eliding the middle so both the
// leading directory and the filename stay visible.
func truncateMiddle(s string, n int) string {
	r := []rune(s)
	if len(r) <= n || n < 5 {
		return s
	}
	keep := n - 1
	head := keep / 2
	return string(r[:head]) + "…" + string(r[len(r)-(keep-head):])
}

// writerIsTerminal reports whether w is an interactive terminal. Only an
// *os.File can be, so a captured buffer (tests, embedded runs) always takes the
// plain path.
func writerIsTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fd := f.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}
