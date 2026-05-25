//go:build onnx

// This file is the cgo-backed SaT engine. It is compiled only with
// `-tags onnx`, which is also when the onnxruntime and tokenizer native
// libraries must be present and linkable. The default build uses engine_stub.go
// instead, so the module (and the satproto/algo tests) build with no native
// dependency.
package sat

import (
	"fmt"
	"sync"

	"github.com/daulet/tokenizers"
	ort "github.com/yalue/onnxruntime_go"

	"github.com/neokapi/neokapi/plugins/sat/internal/model"
)

// XLM-RoBERTa special token ids (from the SaT config.json):
// bos/cls = 0, pad = 1, eos/sep = 2. Padding is unused because we run one
// block at a time (batch size 1, exact sequence length), so no padding is
// needed.
const (
	clsTokenID int64 = 0
	sepTokenID int64 = 2
)

// onnxEngine is the real Engine. It owns the onnxruntime environment and a
// cache of loaded models keyed by name. It is safe for concurrent use.
type onnxEngine struct {
	dl   *model.Downloader
	logf func(string, ...any)

	mu     sync.Mutex
	models map[string]*loadedModel
}

// loadedModel bundles a model's ONNX session and tokenizer.
type loadedModel struct {
	tk      *tokenizers.Tokenizer
	session *ort.DynamicAdvancedSession
}

// NewEngine creates the ONNX-backed engine. It initializes the onnxruntime
// environment, pointing it at the shared library named by KAPI_SAT_ORT_LIB if
// set (otherwise relying on onnxruntime_go's default discovery). cacheLogf, if
// non-nil, receives download/progress lines.
func NewEngine(cacheLogf func(string, ...any)) (Engine, error) {
	if cacheLogf == nil {
		cacheLogf = func(string, ...any) {}
	}
	if err := initORT(); err != nil {
		return nil, err
	}
	return &onnxEngine{
		dl:     &model.Downloader{Logf: cacheLogf},
		logf:   cacheLogf,
		models: map[string]*loadedModel{},
	}, nil
}

func (e *onnxEngine) Loaded(name string) bool {
	if name == "" {
		name = model.DefaultModelName()
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	_, ok := e.models[name]
	return ok
}

func (e *onnxEngine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, m := range e.models {
		if m.session != nil {
			m.session.Destroy()
		}
		if m.tk != nil {
			m.tk.Close()
		}
	}
	e.models = map[string]*loadedModel{}
	ort.DestroyEnvironment()
	return nil
}

// get returns the loaded model for name, loading (and caching) it on first use.
func (e *onnxEngine) get(name string) (*loadedModel, error) {
	if name == "" {
		name = model.DefaultModelName()
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if m, ok := e.models[name]; ok {
		return m, nil
	}
	paths, err := e.dl.Ensure(name)
	if err != nil {
		return nil, err
	}
	tkBytes, err := readFile(paths.Tokenizer)
	if err != nil {
		return nil, fmt.Errorf("sat: read tokenizer: %w", err)
	}
	tk, err := tokenizers.FromBytes(tkBytes)
	if err != nil {
		return nil, fmt.Errorf("sat: load tokenizer: %w", err)
	}
	// Dynamic session: input/output tensors are supplied per Run, so we can
	// vary the batch and sequence dimensions without rebuilding the session.
	session, err := ort.NewDynamicAdvancedSession(
		paths.ONNX,
		[]string{"input_ids", "attention_mask"},
		[]string{"logits"},
		nil,
	)
	if err != nil {
		tk.Close()
		return nil, fmt.Errorf("sat: create onnx session: %w", err)
	}
	m := &loadedModel{tk: tk, session: session}
	e.models[name] = m
	e.logf("loaded model %s", name)
	return m, nil
}

// Segment runs the full SaT pipeline for text.
func (e *onnxEngine) Segment(text, name, _ string, threshold float64) ([]int, error) {
	if text == "" {
		return []int{}, nil
	}
	m, err := e.get(name)
	if err != nil {
		return nil, err
	}

	// 1) Tokenize WITHOUT special tokens; we add CLS/SEP per block. Offsets
	//    are byte ranges into text.
	enc := m.tk.EncodeWithOptions(text, false,
		tokenizers.WithReturnOffsets(),
	)
	contentIDs := make([]int64, len(enc.IDs))
	for i, id := range enc.IDs {
		contentIDs[i] = int64(id)
	}
	spans := make([]tokenSpan, len(enc.Offsets))
	for i, off := range enc.Offsets {
		spans[i] = tokenSpan{Start: int(off[0]), End: int(off[1])}
	}
	n := len(contentIDs)
	if n == 0 {
		return []int{}, nil
	}

	// 2) Plan overlapping blocks and run each through the model.
	blocks := planBlocks(n, BlockSize, Stride)
	blockLogits := make([][]float64, len(blocks))
	for bi, b := range blocks {
		bl, err := e.runBlock(m, contentIDs[b.Start:b.End])
		if err != nil {
			return nil, err
		}
		blockLogits[bi] = bl
	}

	// 3) Recombine overlapping logits, then map to rune offsets.
	combined := combineLogits(n, blocks, blockLogits)
	byteToRune := buildByteToRune(text)
	return boundaryRuneOffsets(text, spans, combined, resolveThreshold(threshold), byteToRune), nil
}

// runBlock prepends CLS, appends SEP, runs the ONNX model on a single block,
// strips the special-token logits, and returns the per-content-token boundary
// logits (length == len(content)).
//
// The SaT exports take input_ids as int64 and attention_mask as float16, and
// emit logits as float16. The Go binding has no native float16 tensor, so the
// mask and logits travel as raw bytes via CustomDataTensor and are converted
// in float16_onnx.go.
func (e *onnxEngine) runBlock(m *loadedModel, content []int64) ([]float64, error) {
	seqLen := len(content) + 2 // CLS + content + SEP
	ids := make([]int64, seqLen)
	ids[0] = clsTokenID
	for i, id := range content {
		ids[i+1] = id
	}
	ids[seqLen-1] = sepTokenID

	shape := ort.NewShape(1, int64(seqLen))
	inIDs, err := ort.NewTensor(shape, ids)
	if err != nil {
		return nil, fmt.Errorf("sat: input_ids tensor: %w", err)
	}
	defer inIDs.Destroy()

	// attention_mask: all-ones float16 (no padding — one block at a time).
	maskBytes := onesFloat16Mask(seqLen)
	inMask, err := ort.NewCustomDataTensor(shape, maskBytes, ort.TensorElementDataTypeFloat16)
	if err != nil {
		return nil, fmt.Errorf("sat: attention_mask tensor: %w", err)
	}
	defer inMask.Destroy()

	// Output logits: [batch=1, seq, num_labels=1], float16 -> 2 bytes each.
	outShape := ort.NewShape(1, int64(seqLen), 1)
	outBytes := make([]byte, seqLen*1*2)
	out, err := ort.NewCustomDataTensor(outShape, outBytes, ort.TensorElementDataTypeFloat16)
	if err != nil {
		return nil, fmt.Errorf("sat: logits tensor: %w", err)
	}
	defer out.Destroy()

	if err := m.session.Run(
		[]ort.Value{inIDs, inMask},
		[]ort.Value{out},
	); err != nil {
		return nil, fmt.Errorf("sat: onnx run: %w", err)
	}

	data := decodeFloat16Logits(outBytes) // len == seqLen * 1
	// Drop CLS (index 0) and SEP (index seqLen-1); take NewlineIndex column.
	logits := make([]float64, len(content))
	for i := range content {
		logits[i] = float64(data[(i+1)*1+NewlineIndex])
	}
	return logits, nil
}
