package bridge

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/plugin/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockBridgeForAdapter creates a JavaBridge backed by pipes, returning the
// pipe ends that simulate the JVM process.
func mockBridgeForAdapter(t *testing.T) (*JavaBridge, io.ReadCloser, io.WriteCloser) {
	t.Helper()
	goStdinR, goStdinW := io.Pipe()
	javaStdoutR, javaStdoutW := io.Pipe()

	b := &JavaBridge{
		cfg: BridgeConfig{
			JARPath:        "/mock/test.jar",
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

// mockPoolForAdapter creates a BridgePool seeded with the given bridge.
// The pool has maxSize=1, which is sufficient for serial adapter tests.
func mockPoolForAdapter(b *JavaBridge) *BridgePool {
	pool := NewBridgePool(1, b.logger)
	pool.Seed(b)
	return pool
}

func respondJSONAdapter(t *testing.T, w io.Writer, resp Response) {
	t.Helper()
	data, err := json.Marshal(resp)
	require.NoError(t, err)
	data = append(data, '\n')
	_, err = w.Write(data)
	require.NoError(t, err)
}

func readCommandAdapter(t *testing.T, r io.Reader) Command {
	t.Helper()
	scanner := bufio.NewScanner(r)
	require.True(t, scanner.Scan(), "expected to read a command")
	var cmd Command
	require.NoError(t, json.Unmarshal(scanner.Bytes(), &cmd))
	return cmd
}

func TestBridgeFormatReaderInterface(t *testing.T) {
	var _ format.DataFormatReader = (*BridgeFormatReader)(nil)
}

func TestBridgeFormatWriterInterface(t *testing.T) {
	var _ format.DataFormatWriter = (*BridgeFormatWriter)(nil)
}

func TestBridgeFormatReaderSignature(t *testing.T) {
	b, stdinR, stdoutW := mockBridgeForAdapter(t)
	pool := mockPoolForAdapter(b)
	reader := NewBridgeFormatReader(pool, b.cfg, "net.sf.okapi.filters.html.HtmlFilter")

	var wg sync.WaitGroup
	wg.Add(1)
	var sig format.FormatSignature

	go func() {
		defer wg.Done()
		sig = reader.Signature()
	}()

	_ = readCommandAdapter(t, stdinR)
	infoData, _ := json.Marshal(InfoData{
		Name:        "html",
		DisplayName: "HTML Filter",
		MimeTypes:   []string{"text/html"},
		Extensions:  []string{".html"},
	})
	respondJSONAdapter(t, stdoutW, Response{Status: "ok", Data: infoData})

	wg.Wait()
	assert.Contains(t, sig.MIMETypes, "text/html")
	assert.Contains(t, sig.Extensions, ".html")
	assert.Equal(t, "html", reader.Name())
	assert.Equal(t, "HTML Filter", reader.DisplayName())
}

func TestBridgeFormatReaderOpenAndRead(t *testing.T) {
	b, stdinR, stdoutW := mockBridgeForAdapter(t)
	pool := mockPoolForAdapter(b)
	reader := NewBridgeFormatReader(pool, b.cfg, "net.sf.okapi.filters.html.HtmlFilter")

	htmlContent := []byte("<html><body>Hello</body></html>")
	doc := &model.RawDocument{
		URI:          "test.html",
		SourceLocale: "en",
		Encoding:     "UTF-8",
		MimeType:     "text/html",
		Reader:       io.NopCloser(bytes.NewReader(htmlContent)),
	}

	// Open in background.
	var wg sync.WaitGroup
	wg.Add(1)
	var openErr error
	go func() {
		defer wg.Done()
		openErr = reader.Open(context.Background(), doc)
	}()

	// Verify the open command.
	cmd := readCommandAdapter(t, stdinR)
	assert.Equal(t, "open", cmd.Command)
	respondJSONAdapter(t, stdoutW, Response{Status: "ok"})
	wg.Wait()
	require.NoError(t, openErr)

	// Read in background.
	wg.Add(1)
	var parts []*model.Part
	var readErr error
	go func() {
		defer wg.Done()
		ctx := context.Background()
		ch := reader.Read(ctx)
		for pr := range ch {
			if pr.Error != nil {
				readErr = pr.Error
				return
			}
			parts = append(parts, pr.Part)
		}
	}()

	// Verify the read command and respond.
	cmd = readCommandAdapter(t, stdinR)
	assert.Equal(t, "read", cmd.Command)

	partDTOs := []shared.PartDTO{
		{
			PartType: int(model.PartLayerStart),
			Layer:    &shared.LayerDTO{ID: "doc1", Name: "test.html", Format: "html"},
		},
		{
			PartType: int(model.PartBlock),
			Block: &shared.BlockDTO{
				ID:           "tu1",
				Translatable: true,
				Source: []shared.SegmentDTO{
					{ID: "s1", Content: shared.FragmentDTO{CodedText: "Hello"}},
				},
			},
		},
		{
			PartType: int(model.PartLayerEnd),
			Layer:    &shared.LayerDTO{ID: "doc1"},
		},
	}
	partsJSON, _ := json.Marshal(partDTOs)
	readDataJSON, _ := json.Marshal(map[string]json.RawMessage{"parts": partsJSON})
	respondJSONAdapter(t, stdoutW, Response{Status: "ok", Data: readDataJSON})

	wg.Wait()
	require.NoError(t, readErr)
	require.Len(t, parts, 3)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartBlock, parts[1].Type)
	assert.Equal(t, model.PartLayerEnd, parts[2].Type)

	// Verify block content.
	block := parts[1].Resource.(*model.Block)
	assert.Equal(t, "tu1", block.ID)
	assert.True(t, block.Translatable)
	require.Len(t, block.Source, 1)
	assert.Equal(t, "Hello", block.Source[0].Content.CodedText)
}

func TestBridgeFormatReaderClose(t *testing.T) {
	b, stdinR, stdoutW := mockBridgeForAdapter(t)
	pool := mockPoolForAdapter(b)
	reader := NewBridgeFormatReader(pool, b.cfg, "net.sf.okapi.filters.html.HtmlFilter")

	// Simulate that Open was called: acquire the bridge from the pool
	// and assign it to the reader.
	acquired, err := pool.Acquire(b.cfg)
	require.NoError(t, err)
	reader.bridge = acquired

	var wg sync.WaitGroup
	wg.Add(1)
	var closeErr error
	go func() {
		defer wg.Done()
		closeErr = reader.Close()
	}()

	cmd := readCommandAdapter(t, stdinR)
	assert.Equal(t, "close", cmd.Command)
	respondJSONAdapter(t, stdoutW, Response{Status: "ok"})

	wg.Wait()
	require.NoError(t, closeErr)
}

func TestBridgeFormatWriterWrite(t *testing.T) {
	b, stdinR, stdoutW := mockBridgeForAdapter(t)
	pool := mockPoolForAdapter(b)
	writer := NewBridgeFormatWriter(pool, b.cfg, "net.sf.okapi.filters.html.HtmlFilter")

	originalContent := []byte("<html><body>Hello</body></html>")
	writer.SetOriginalContent(originalContent)
	writer.SetLocale("fr")
	writer.SetEncoding("UTF-8")

	var outputBuf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&outputBuf))

	// Create a parts channel.
	partsCh := make(chan *model.Part, 3)
	partsCh <- &model.Part{
		Type:     model.PartLayerStart,
		Resource: &model.Layer{ID: "doc1"},
	}
	partsCh <- &model.Part{
		Type: model.PartBlock,
		Resource: &model.Block{
			ID:           "tu1",
			Translatable: true,
			Source: []*model.Segment{
				{ID: "s1", Content: model.NewFragment("Hello")},
			},
			Targets: map[model.LocaleID][]*model.Segment{
				"fr": {{ID: "s1", Content: model.NewFragment("Bonjour")}},
			},
		},
	}
	partsCh <- &model.Part{
		Type:     model.PartLayerEnd,
		Resource: &model.Layer{ID: "doc1"},
	}
	close(partsCh)

	// Write in background.
	var wg sync.WaitGroup
	wg.Add(1)
	var writeErr error
	go func() {
		defer wg.Done()
		writeErr = writer.Write(context.Background(), partsCh)
	}()

	// Read the write command.
	cmd := readCommandAdapter(t, stdinR)
	assert.Equal(t, "write", cmd.Command)

	// Respond with translated output.
	translatedHTML := "<html><body>Bonjour</body></html>"
	wd, _ := json.Marshal(WriteData{
		OutputBase64: base64.StdEncoding.EncodeToString([]byte(translatedHTML)),
	})
	respondJSONAdapter(t, stdoutW, Response{Status: "ok", Data: wd})

	wg.Wait()
	require.NoError(t, writeErr)
	assert.Equal(t, translatedHTML, outputBuf.String())
}

func TestBridgeFormatWriterSetOutput(t *testing.T) {
	b, _, _ := mockBridgeForAdapter(t)
	pool := mockPoolForAdapter(b)
	writer := NewBridgeFormatWriter(pool, b.cfg, "net.sf.okapi.filters.html.HtmlFilter")

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))
	assert.Equal(t, "net.sf.okapi.filters.html.HtmlFilter", writer.filterClass)
}

func TestBridgeFormatReaderSetFilterParams(t *testing.T) {
	b, _, _ := mockBridgeForAdapter(t)
	pool := mockPoolForAdapter(b)
	reader := NewBridgeFormatReader(pool, b.cfg, "net.sf.okapi.filters.json.JSONFilter")

	params := map[string]interface{}{
		"extractStandalone": true,
		"extractAllPairs":   false,
		"codeFinderRules": map[string]interface{}{
			"rules": []map[string]string{
				{"pattern": "<[^>]+>"},
			},
			"sample": "test <b>text</b>",
		},
	}
	reader.SetFilterParams(params)

	assert.NotNil(t, reader.filterParams)
	assert.Equal(t, true, reader.filterParams["extractStandalone"])
	assert.Equal(t, false, reader.filterParams["extractAllPairs"])
}

func TestBridgeFormatWriterSetFilterParams(t *testing.T) {
	b, _, _ := mockBridgeForAdapter(t)
	pool := mockPoolForAdapter(b)
	writer := NewBridgeFormatWriter(pool, b.cfg, "net.sf.okapi.filters.json.JSONFilter")

	params := map[string]interface{}{
		"extractStandalone": true,
		"maxDepth":          10,
	}
	writer.SetFilterParams(params)

	assert.NotNil(t, writer.filterParams)
	assert.Equal(t, true, writer.filterParams["extractStandalone"])
	assert.Equal(t, 10, writer.filterParams["maxDepth"])
}

func TestBridgeFormatReaderOpenWithFilterParams(t *testing.T) {
	b, stdinR, stdoutW := mockBridgeForAdapter(t)
	pool := mockPoolForAdapter(b)
	reader := NewBridgeFormatReader(pool, b.cfg, "net.sf.okapi.filters.json.JSONFilter")

	// Set filter params before Open
	params := map[string]interface{}{
		"extractStandalone": true,
	}
	reader.SetFilterParams(params)

	jsonContent := []byte(`{"key": "value"}`)
	doc := &model.RawDocument{
		URI:          "test.json",
		SourceLocale: "en",
		Encoding:     "UTF-8",
		MimeType:     "application/json",
		Reader:       io.NopCloser(bytes.NewReader(jsonContent)),
	}

	// Open in background
	var wg sync.WaitGroup
	wg.Add(1)
	var openErr error
	go func() {
		defer wg.Done()
		openErr = reader.Open(context.Background(), doc)
	}()

	// Verify the open command includes filter_params
	cmd := readCommandAdapter(t, stdinR)
	assert.Equal(t, "open", cmd.Command)

	// Check that filter_params is in the params
	// cmd.Params is interface{}, need to marshal/unmarshal to get typed struct
	paramsJSON, err := json.Marshal(cmd.Params)
	require.NoError(t, err)
	var openParams OpenParams
	err = json.Unmarshal(paramsJSON, &openParams)
	require.NoError(t, err)
	assert.NotNil(t, openParams.FilterParams)
	assert.Equal(t, true, openParams.FilterParams["extractStandalone"])

	respondJSONAdapter(t, stdoutW, Response{Status: "ok"})
	wg.Wait()
	require.NoError(t, openErr)
}

func TestBridgeFormatWriterWriteWithFilterParams(t *testing.T) {
	b, stdinR, stdoutW := mockBridgeForAdapter(t)
	pool := mockPoolForAdapter(b)
	writer := NewBridgeFormatWriter(pool, b.cfg, "net.sf.okapi.filters.json.JSONFilter")

	// Set filter params before Write
	params := map[string]interface{}{
		"extractStandalone": false,
	}
	writer.SetFilterParams(params)

	originalContent := []byte(`{"key": "value"}`)
	writer.SetOriginalContent(originalContent)
	writer.SetLocale("fr")
	writer.SetEncoding("UTF-8")

	var outputBuf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&outputBuf))

	// Create a parts channel
	partsCh := make(chan *model.Part, 3)
	partsCh <- &model.Part{
		Type:     model.PartLayerStart,
		Resource: &model.Layer{ID: "doc1"},
	}
	partsCh <- &model.Part{
		Type: model.PartBlock,
		Resource: &model.Block{
			ID:           "tu1",
			Translatable: true,
			Source:       []*model.Segment{{ID: "s1", Content: model.NewFragment("value")}},
			Targets: map[model.LocaleID][]*model.Segment{
				"fr": {{ID: "s1", Content: model.NewFragment("valeur")}},
			},
		},
	}
	partsCh <- &model.Part{
		Type:     model.PartLayerEnd,
		Resource: &model.Layer{ID: "doc1"},
	}
	close(partsCh)

	// Write in background
	var wg sync.WaitGroup
	wg.Add(1)
	var writeErr error
	go func() {
		defer wg.Done()
		writeErr = writer.Write(context.Background(), partsCh)
	}()

	// Read the write command
	cmd := readCommandAdapter(t, stdinR)
	assert.Equal(t, "write", cmd.Command)

	// Verify filter_params is in the params
	// cmd.Params is interface{}, need to marshal/unmarshal to get typed struct
	paramsJSON, err := json.Marshal(cmd.Params)
	require.NoError(t, err)
	var writeParams WriteParams
	err = json.Unmarshal(paramsJSON, &writeParams)
	require.NoError(t, err)
	assert.NotNil(t, writeParams.FilterParams)
	assert.Equal(t, false, writeParams.FilterParams["extractStandalone"])

	// Respond
	translatedJSON := `{"key": "valeur"}`
	wd, _ := json.Marshal(WriteData{
		OutputBase64: base64.StdEncoding.EncodeToString([]byte(translatedJSON)),
	})
	respondJSONAdapter(t, stdoutW, Response{Status: "ok", Data: wd})

	wg.Wait()
	require.NoError(t, writeErr)
	assert.Equal(t, translatedJSON, outputBuf.String())
}

func TestBridgeFormatReaderReadContextCancel(t *testing.T) {
	b, stdinR, stdoutW := mockBridgeForAdapter(t)
	pool := mockPoolForAdapter(b)
	reader := NewBridgeFormatReader(pool, b.cfg, "net.sf.okapi.filters.html.HtmlFilter")
	// Simulate an acquired bridge (as if Open was called).
	reader.bridge = b

	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	wg.Add(1)
	var results []model.PartResult
	go func() {
		defer wg.Done()
		ch := reader.Read(ctx)
		for pr := range ch {
			results = append(results, pr)
		}
	}()

	// Read command.
	cmd := readCommandAdapter(t, stdinR)
	assert.Equal(t, "read", cmd.Command)

	// Return many parts but cancel after sending.
	cancel()

	// Still need to send a response for the bridge.Read() call to complete.
	manyParts := make([]shared.PartDTO, 100)
	for i := range manyParts {
		manyParts[i] = shared.PartDTO{
			PartType: int(model.PartData),
			Data:     &shared.DataDTO{ID: "d"},
		}
	}
	partsJSON, _ := json.Marshal(manyParts)
	readDataJSON, _ := json.Marshal(map[string]json.RawMessage{"parts": partsJSON})
	respondJSONAdapter(t, stdoutW, Response{Status: "ok", Data: readDataJSON})

	wg.Wait()

	// Should have received a context canceled error at some point.
	hasError := false
	for _, r := range results {
		if r.Error != nil {
			hasError = true
			break
		}
	}
	assert.True(t, hasError || len(results) < 100, "context cancellation should stop part emission")
}
