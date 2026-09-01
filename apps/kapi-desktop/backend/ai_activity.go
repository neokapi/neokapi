package backend

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// AI activity: what this session sent to a model, and what came back.
//
// Every AI surface in the app produces a target the user is asked to accept:
// a proposed translation, a corrected one, a review verdict, a whole
// convergence pass. Judging any of them means knowing what the model was
// actually told, and until now nothing carried it. `kapi --explain-prompts`
// answers the same question at the command line; this is the desktop's copy of
// that answer, kept for the life of the session so it is there when the user
// thinks to look rather than only when they asked in advance.
//
// It is read-only observation. A recorder never changes a call, and an entry is
// a record of one that already happened.

// AIActivityScope names the work an LLM call was made for. It travels on the
// call's context, so an exchange can be attributed to the unit it was about
// instead of arriving in an undifferentiated list.
type AIActivityScope struct {
	// Surface is the part of the app that made the call: "review", "pre-review",
	// "convergence", "tool".
	Surface string `json:"surface"`
	// Action is the specific operation within the surface, when there is one
	// ("fix-findings", "retranslate", "explain").
	Action string `json:"action,omitempty"`
	// Locale, File and Key address the unit the call was about, when it was
	// about one. A convergence pass fills only Locale.
	Locale string `json:"locale,omitempty"`
	File   string `json:"file,omitempty"`
	Key    string `json:"key,omitempty"`
}

type aiScopeKey struct{}

// withAIScope stamps the scope on a context so the recorder can read it back.
func withAIScope(ctx context.Context, scope AIActivityScope) context.Context {
	return context.WithValue(ctx, aiScopeKey{}, scope)
}

// aiScopeFrom reads the scope a call was made under, if any.
func aiScopeFrom(ctx context.Context) (AIActivityScope, bool) {
	s, ok := ctx.Value(aiScopeKey{}).(AIActivityScope)
	return s, ok
}

// AIActivityEntry is one recorded LLM call: the scope it was made for, when it
// happened, and the exchange itself (the messages sent, the schema constraining
// the output, the reply, the token usage).
type AIActivityEntry struct {
	ID    int64           `json:"id"`
	At    time.Time       `json:"at"`
	Scope AIActivityScope `json:"scope"`
	// Duplicated out of Exchange so a list view can render a row without
	// unpacking the messages.
	Provider string `json:"provider"`
	Model    string `json:"model,omitempty"`
	Prompt   string `json:"prompt,omitempty"`
	Version  string `json:"prompt_version,omitempty"`
	Error    string `json:"error,omitempty"`

	Exchange aiprovider.Exchange `json:"exchange"`
}

// aiActivityCap is how many exchanges a session keeps. A convergence run over a
// large project makes thousands of calls whose messages carry the full source
// text, so the log is bounded and drops its oldest entries rather than growing
// until the app is killed. The cap is generous enough to cover a review session
// and a run or two.
const aiActivityCap = 500

// aiActivityLog is the session's rolling record of LLM calls. Reads and writes
// come from different goroutines (a Wails method call and a provider call in a
// run), so every access takes the lock.
type aiActivityLog struct {
	mu      sync.RWMutex
	entries []AIActivityEntry
	nextID  atomic.Int64
	// dropped counts entries evicted by the cap, so the UI can say the log is
	// not the whole story rather than implying it is.
	dropped int
}

func newAIActivityLog() *aiActivityLog { return &aiActivityLog{} }

// add records one exchange under the scope its context carries.
func (l *aiActivityLog) add(ctx context.Context, ex aiprovider.Exchange) {
	scope, _ := aiScopeFrom(ctx)
	entry := AIActivityEntry{
		ID:       l.nextID.Add(1),
		At:       time.Now(),
		Scope:    scope,
		Provider: ex.Provider,
		Model:    ex.Model,
		Prompt:   ex.Prompt,
		Version:  ex.Version,
		Error:    ex.Err,
		Exchange: ex,
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, entry)
	if over := len(l.entries) - aiActivityCap; over > 0 {
		l.entries = append(l.entries[:0], l.entries[over:]...)
		l.dropped += over
	}
}

// recent returns up to limit entries, newest first. A limit of 0 or less
// returns everything the log holds.
func (l *aiActivityLog) recent(limit int) ([]AIActivityEntry, int) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	n := len(l.entries)
	if limit > 0 && limit < n {
		n = limit
	}
	out := make([]AIActivityEntry, 0, n)
	for i := len(l.entries) - 1; i >= 0 && len(out) < n; i-- {
		out = append(out, l.entries[i])
	}
	return out, l.dropped
}

// since returns every entry recorded after the given id, oldest first. It is
// how one action collects its own calls: take the id before the call, ask for
// what followed.
func (l *aiActivityLog) since(id int64) []AIActivityEntry {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var out []AIActivityEntry
	for _, e := range l.entries {
		if e.ID > id {
			out = append(out, e)
		}
	}
	return out
}

// lastID is the id of the most recent entry, or 0 when the log is empty.
func (l *aiActivityLog) lastID() int64 { return l.nextID.Load() }

func (l *aiActivityLog) clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = nil
	l.dropped = 0
}

// AIActivityResult is the log as the frontend reads it.
type AIActivityResult struct {
	Entries []AIActivityEntry `json:"entries"`
	// Dropped is how many older entries the cap evicted. Non-zero means the list
	// is a window, not the session's whole history.
	Dropped int `json:"dropped"`
	// Cap is the window size, so the UI can say what the limit is.
	Cap int `json:"cap"`
}

// GetAIActivity returns this session's recorded LLM calls, newest first. A limit
// of 0 or less returns every entry the log holds.
func (a *App) GetAIActivity(limit int) AIActivityResult {
	entries, dropped := a.aiActivity.recent(limit)
	if entries == nil {
		entries = []AIActivityEntry{}
	}
	return AIActivityResult{Entries: entries, Dropped: dropped, Cap: aiActivityCap}
}

// ClearAIActivity empties the log.
func (a *App) ClearAIActivity() { a.aiActivity.clear() }
