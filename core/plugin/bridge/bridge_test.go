package bridge

import (
	"bufio"
	"encoding/json"
	"io"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockBridge creates a JavaBridge with piped stdin/stdout for testing
// without a real JVM process.
func mockBridge(t *testing.T) (*JavaBridge, io.ReadCloser, io.WriteCloser) {
	t.Helper()

	// goStdinR is what the bridge writes to (its stdin pipe).
	// goStdinW is the read end we use to see what bridge sent.
	goStdinR, goStdinW := io.Pipe()

	// javaStdoutR is what the bridge reads from.
	// javaStdoutW is the write end we use to simulate JVM responses.
	javaStdoutR, javaStdoutW := io.Pipe()

	b := &JavaBridge{
		cfg: BridgeConfig{
			CommandTimeout: 5 * time.Second,
			StartupTimeout: 5 * time.Second,
		},
		stdin:   goStdinW,
		scanner: bufio.NewScanner(javaStdoutR),
		logger:  log.Default(),
		running: true,
	}
	b.scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)

	return b, goStdinR, javaStdoutW
}

// respondJSON writes a JSON response line to the mock stdout.
func respondJSON(t *testing.T, w io.Writer, resp Response) {
	t.Helper()
	data, err := json.Marshal(resp)
	require.NoError(t, err)
	data = append(data, '\n')
	_, err = w.Write(data)
	require.NoError(t, err)
}

// readCommand reads and parses a command from the bridge stdin pipe.
func readCommand(t *testing.T, r io.Reader) Command {
	t.Helper()
	scanner := bufio.NewScanner(r)
	require.True(t, scanner.Scan(), "expected to read a command")
	var cmd Command
	require.NoError(t, json.Unmarshal(scanner.Bytes(), &cmd))
	return cmd
}

func TestBridgeInfo(t *testing.T) {
	b, stdinR, stdoutW := mockBridge(t)

	var wg sync.WaitGroup
	wg.Add(1)

	var info *InfoData
	var infoErr error

	go func() {
		defer wg.Done()
		info, infoErr = b.Info("net.sf.okapi.filters.html.HtmlFilter")
	}()

	// Read the command the bridge sent.
	cmd := readCommand(t, stdinR)
	assert.Equal(t, "info", cmd.Command)

	// Respond.
	infoData, _ := json.Marshal(InfoData{
		Name:        "html",
		DisplayName: "HTML Filter",
		MimeTypes:   []string{"text/html"},
		Extensions:  []string{".html", ".htm"},
	})
	respondJSON(t, stdoutW, Response{
		Status: "ok",
		Data:   infoData,
	})

	wg.Wait()
	require.NoError(t, infoErr)
	assert.Equal(t, "html", info.Name)
	assert.Equal(t, "HTML Filter", info.DisplayName)
	assert.Contains(t, info.Extensions, ".html")
}

func TestBridgeOpen(t *testing.T) {
	b, stdinR, stdoutW := mockBridge(t)

	var wg sync.WaitGroup
	wg.Add(1)
	var openErr error

	go func() {
		defer wg.Done()
		openErr = b.Open(OpenParams{
			FilterClass:   "net.sf.okapi.filters.html.HtmlFilter",
			URI:           "test.html",
			SourceLocale:  "en",
			Encoding:      "UTF-8",
			ContentBase64: "PGh0bWw+PC9odG1sPg==",
			MimeType:      "text/html",
		})
	}()

	cmd := readCommand(t, stdinR)
	assert.Equal(t, "open", cmd.Command)

	respondJSON(t, stdoutW, Response{Status: "ok"})

	wg.Wait()
	require.NoError(t, openErr)
}

func TestBridgeRead(t *testing.T) {
	b, stdinR, stdoutW := mockBridge(t)

	var wg sync.WaitGroup
	wg.Add(1)
	var readData *ReadData
	var readErr error

	go func() {
		defer wg.Done()
		readData, readErr = b.Read()
	}()

	cmd := readCommand(t, stdinR)
	assert.Equal(t, "read", cmd.Command)

	parts := `[{"part_type":0,"layer":{"id":"doc1","name":"test.html","format":"html"}}]`
	respData, _ := json.Marshal(map[string]json.RawMessage{
		"parts": json.RawMessage(parts),
	})
	respondJSON(t, stdoutW, Response{
		Status: "ok",
		Data:   respData,
	})

	wg.Wait()
	require.NoError(t, readErr)
	require.NotNil(t, readData)
	assert.NotEmpty(t, readData.Parts)
}

func TestBridgeWrite(t *testing.T) {
	b, stdinR, stdoutW := mockBridge(t)

	var wg sync.WaitGroup
	wg.Add(1)
	var writeData *WriteData
	var writeErr error

	go func() {
		defer wg.Done()
		writeData, writeErr = b.Write(WriteParams{
			FilterClass:           "net.sf.okapi.filters.html.HtmlFilter",
			Parts:                 []map[string]any{{"part_type": 0}},
			Locale:                "fr",
			Encoding:              "UTF-8",
			OriginalContentBase64: "PGh0bWw+PC9odG1sPg==",
		})
	}()

	cmd := readCommand(t, stdinR)
	assert.Equal(t, "write", cmd.Command)

	wd, _ := json.Marshal(WriteData{OutputBase64: "dHJhbnNsYXRlZA=="})
	respondJSON(t, stdoutW, Response{
		Status: "ok",
		Data:   wd,
	})

	wg.Wait()
	require.NoError(t, writeErr)
	assert.Equal(t, "dHJhbnNsYXRlZA==", writeData.OutputBase64)
}

func TestBridgeCloseFilter(t *testing.T) {
	b, stdinR, stdoutW := mockBridge(t)

	var wg sync.WaitGroup
	wg.Add(1)
	var closeErr error

	go func() {
		defer wg.Done()
		closeErr = b.CloseFilter()
	}()

	cmd := readCommand(t, stdinR)
	assert.Equal(t, "close", cmd.Command)
	respondJSON(t, stdoutW, Response{Status: "ok"})

	wg.Wait()
	require.NoError(t, closeErr)
}

func TestBridgeListFilters(t *testing.T) {
	b, stdinR, stdoutW := mockBridge(t)

	var wg sync.WaitGroup
	wg.Add(1)
	var lf *ListFiltersData
	var lfErr error

	go func() {
		defer wg.Done()
		lf, lfErr = b.ListFilters()
	}()

	cmd := readCommand(t, stdinR)
	assert.Equal(t, "list_filters", cmd.Command)

	lfData, _ := json.Marshal(ListFiltersData{
		Filters: []FilterEntry{
			{FilterClass: "net.sf.okapi.filters.html.HtmlFilter", Name: "html", DisplayName: "HTML"},
		},
	})
	respondJSON(t, stdoutW, Response{Status: "ok", Data: lfData})

	wg.Wait()
	require.NoError(t, lfErr)
	require.Len(t, lf.Filters, 1)
	assert.Equal(t, "html", lf.Filters[0].Name)
}

func TestBridgeErrorResponse(t *testing.T) {
	b, stdinR, stdoutW := mockBridge(t)

	var wg sync.WaitGroup
	wg.Add(1)
	var infoErr error

	go func() {
		defer wg.Done()
		_, infoErr = b.Info("nonexistent.Filter")
	}()

	_ = readCommand(t, stdinR)
	respondJSON(t, stdoutW, Response{
		Status: "error",
		Error:  "filter class not found: nonexistent.Filter",
	})

	wg.Wait()
	require.Error(t, infoErr)
	assert.Contains(t, infoErr.Error(), "filter class not found")
}

func TestBridgeTimeout(t *testing.T) {
	// Use a very short timeout.
	b, stdinR, stdoutW := mockBridge(t)
	b.cfg.CommandTimeout = 50 * time.Millisecond

	// Drain stdin so the write doesn't block.
	go func() {
		scanner := bufio.NewScanner(stdinR)
		for scanner.Scan() {
		}
	}()

	// No response will be sent, so this should time out.
	_, err := b.Info("some.Filter")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timed out")

	// Close pipes to unblock any leaked goroutines.
	stdinR.Close()
	stdoutW.Close()
}

func TestConfigWithDefaults(t *testing.T) {
	cfg := BridgeConfig{}
	cfg = cfg.withDefaults()
	assert.Equal(t, "java", cfg.Command)
	assert.Equal(t, DefaultStartupTimeout, cfg.StartupTimeout)
	assert.Equal(t, DefaultCommandTimeout, cfg.CommandTimeout)
}

func TestConfigPreservesValues(t *testing.T) {
	cfg := BridgeConfig{
		Command:        "/usr/local/bin/java",
		StartupTimeout: 10 * time.Second,
		CommandTimeout: 30 * time.Second,
	}
	cfg = cfg.withDefaults()
	assert.Equal(t, "/usr/local/bin/java", cfg.Command)
	assert.Equal(t, 10*time.Second, cfg.StartupTimeout)
	assert.Equal(t, 30*time.Second, cfg.CommandTimeout)
}
