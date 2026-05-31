package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errAfterFirst is a test tool that reads a single part then returns an error
// without draining the rest of the input. This makes the executor cancel and
// stop its tool goroutines early, leaving the RunToolOnFiles feeder parked on
// an inCh send for every part past the channel buffer (default 64) — exactly
// the condition under which an unguarded feeder goroutine leaks.
type errAfterFirst struct {
	*tool.BaseTool
}

func (e *errAfterFirst) Process(ctx context.Context, in <-chan *model.Part, _ chan<- *model.Part) error {
	select {
	case <-in:
	case <-ctx.Done():
		return ctx.Err()
	}
	return errors.New("boom: tool failed mid-stream")
}

// TestRunToolOnFilesErroringToolDoesNotLeakFeeder verifies that when a tool
// errors after consuming only part of the stream, the feeder goroutine that
// pushes parts into the executor is cancelled and joined rather than parking
// forever. The input is sized well past the channel buffer so an unguarded
// feeder would remain blocked on `inCh <- p` after the tools stop.
func TestRunToolOnFilesErroringToolDoesNotLeakFeeder(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "big.json")

	// 200 keys → ~202 parts, comfortably above the 64-part channel buffer.
	var b []byte
	b = append(b, '{')
	for i := range 200 {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, fmt.Sprintf("%q:%q", fmt.Sprintf("k%d", i), fmt.Sprintf("value number %d", i))...)
	}
	b = append(b, '}')
	require.NoError(t, os.WriteFile(src, b, 0o644))

	a := &App{SourceLang: "en"}
	a.InitRegistries()

	run := func() {
		cfg := ToolRunConfig{
			ToolName: "err-after-first",
			Files:    []string{src},
			NewTool: func() (tool.Tool, error) {
				return &errAfterFirst{
					BaseTool: &tool.BaseTool{ToolName: "err-after-first"},
				}, nil
			},
		}
		err := a.RunToolOnFiles(context.Background(), cfg)
		require.Error(t, err, "erroring tool must surface its error")
		assert.Contains(t, err.Error(), "tool execution on")
	}

	// Warm up once so first-call lazy initialisation (registries, file caches)
	// settles, then snapshot a stable baseline.
	run()
	baseline := settledGoroutineCount()

	// A leaking feeder parks one goroutine per invocation. Run the erroring
	// pipeline many times; with the ctx-guard each feeder is cancelled and
	// joined, so the count stays bounded. Without it, ~iterations goroutines
	// would accumulate (each parked on inCh <- p past the 64-part buffer).
	const iterations = 30
	for range iterations {
		run()
	}

	require.Eventually(t, func() bool {
		// Allow a small slack for unrelated runtime/test goroutines; a real
		// leak would add ~iterations, far above this threshold.
		return settledGoroutineCount() <= baseline+5
	}, 2*time.Second, 20*time.Millisecond,
		"feeder goroutines leaked across %d runs: baseline=%d now=%d",
		iterations, baseline, runtime.NumGoroutine())
}

// settledGoroutineCount returns a goroutine count after letting transient
// goroutines from prior work wind down, so leak assertions have a steady
// reference point rather than racing against still-exiting goroutines.
func settledGoroutineCount() int {
	prev := runtime.NumGoroutine()
	for range 50 {
		runtime.Gosched()
		time.Sleep(5 * time.Millisecond)
		n := runtime.NumGoroutine()
		if n <= prev {
			return n
		}
		prev = n
	}
	return prev
}

func TestParseFormatMappings(t *testing.T) {
	tests := []struct {
		name    string
		input   []string
		want    []FormatMapping
		wantErr bool
	}{
		{
			name:  "single mapping",
			input: []string{"*.docx=okf_openxml:test"},
			want:  []FormatMapping{{Pattern: "*.docx", Format: "okf_openxml:test"}},
		},
		{
			name:  "multiple mappings",
			input: []string{"*.docx=okf_openxml:test", "*.md=okf_markdown@0.38"},
			want: []FormatMapping{
				{Pattern: "*.docx", Format: "okf_openxml:test"},
				{Pattern: "*.md", Format: "okf_markdown@0.38"},
			},
		},
		{
			name:  "version and preset",
			input: []string{"*.html=okf_html@1.46.0:wellFormed"},
			want:  []FormatMapping{{Pattern: "*.html", Format: "okf_html@1.46.0:wellFormed"}},
		},
		{
			name:  "empty input",
			input: nil,
			want:  []FormatMapping{},
		},
		{
			name:    "missing equals",
			input:   []string{"*.docx"},
			wantErr: true,
		},
		{
			name:    "empty pattern",
			input:   []string{"=okf_openxml"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseFormatMappings(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMatchFormatMapping(t *testing.T) {
	mappings := []FormatMapping{
		{Pattern: "*.docx", Format: "okf_openxml:test"},
		{Pattern: "*.md", Format: "okf_markdown@0.38"},
		{Pattern: "report-*.txt", Format: "plaintext"},
	}

	tests := []struct {
		filePath string
		want     string
	}{
		{"/home/user/doc.docx", "okf_openxml:test"},
		{"/home/user/README.md", "okf_markdown@0.38"},
		{"/tmp/report-2024.txt", "plaintext"},
		{"/tmp/notes.txt", ""}, // no match
		{"/tmp/data.json", ""}, // no match
		{"relative/path/file.docx", "okf_openxml:test"},
	}

	for _, tt := range tests {
		t.Run(tt.filePath, func(t *testing.T) {
			got := matchFormatMapping(tt.filePath, mappings)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMatchFormatMappingEmpty(t *testing.T) {
	assert.Equal(t, "", matchFormatMapping("/some/file.docx", nil))
	assert.Equal(t, "", matchFormatMapping("/some/file.docx", []FormatMapping{}))
}
