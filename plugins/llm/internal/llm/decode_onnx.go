//go:build onnx

package llm

import (
	"fmt"
	"math/rand"

	ort "github.com/yalue/onnxruntime_go"
)

// genState carries the per-request decode state.
type genState struct {
	m         *loadedModel
	params    GenerateParams
	rng       *rand.Rand
	maxTokens int
	slots     []mediaSlot
}

// run executes prefill + the autoregressive decode loop and returns the decoded
// text and the number of generated tokens. promptIDs is the tokenized prompt
// (including any media placeholder tokens described by g.slots).
func (g *genState) run(promptIDs []int64) (string, int, error) {
	m := g.m

	// Prefill embeddings for the whole prompt, then overwrite placeholder rows
	// with the media encoder outputs captured while building the prompt.
	embeds, ple, err := m.embedTokens(promptIDs)
	if err != nil {
		return "", 0, err
	}
	if err := m.applyMediaSlots(embeds, g.slots); err != nil {
		embeds.Destroy()
		if ple != nil {
			ple.Destroy()
		}
		return "", 0, err
	}

	past := m.emptyPast()
	defer func() { destroyValues(past) }()

	seqLen := len(promptIDs)
	positions := rangeI64(0, seqLen)
	logits, newPast, err := m.decodeStep(embeds, ple, positions, seqLen, past)
	embeds.Destroy()
	if ple != nil {
		ple.Destroy()
	}
	if err != nil {
		return "", 0, err
	}
	destroyValues(past)
	past = newPast

	generated := make([]uint32, 0, g.maxTokens)
	totalLen := seqLen
	for step := 0; step < g.maxTokens; step++ {
		next := sample(logits, g.params.Temperature, g.params.TopP, g.rng)
		if m.cfg.isEOS(next) {
			break
		}
		generated = append(generated, uint32(next))

		// Embed the single new token and decode one step.
		stepEmbeds, stepPLE, eerr := m.embedTokens([]int64{int64(next)})
		if eerr != nil {
			return "", 0, eerr
		}
		positions = []int64{int64(totalLen)}
		var nextPast map[string]ort.Value
		logits, nextPast, err = m.decodeStep(stepEmbeds, stepPLE, positions, totalLen+1, past)
		stepEmbeds.Destroy()
		if stepPLE != nil {
			stepPLE.Destroy()
		}
		if err != nil {
			return "", 0, err
		}
		destroyValues(past)
		past = nextPast
		totalLen++
	}

	text := m.tk.Decode(generated, true)
	return text, len(generated), nil
}

// decodeStep runs decoder_model_merged once. embeds/ple are the embeddings for
// the current input tokens; positions are their position ids; totalLen is the
// attention length (past + current); past holds the KV cache keyed by
// past_key_values.* input name. It returns the last-position logits (copied out)
// and the new KV cache (the caller owns and must destroy it).
func (m *loadedModel) decodeStep(embeds, ple ort.Value, positions []int64, totalLen int, past map[string]ort.Value) ([]float32, map[string]ort.Value, error) {
	inLen := len(positions)

	attnMask, err := ort.NewTensor(ort.NewShape(1, int64(totalLen)), onesI64(totalLen))
	if err != nil {
		return nil, nil, fmt.Errorf("llm: attention_mask: %w", err)
	}
	defer attnMask.Destroy()

	posT, err := ort.NewTensor(ort.NewShape(1, int64(inLen)), positions)
	if err != nil {
		return nil, nil, fmt.Errorf("llm: position_ids: %w", err)
	}
	defer posT.Destroy()

	// num_logits_to_keep = 1 (keep only the final position's logits). The export
	// declares it as a rank-0 scalar and uses it as Neg→Unsqueeze→Slice in the
	// lm_head; a 1-D [1] tensor makes the Slice's starts the wrong rank, so it
	// must be a true 0-D scalar (ort.Scalar), not ort.NewTensor.
	numLogits, err := ort.NewScalar[int64](1)
	if err != nil {
		return nil, nil, fmt.Errorf("llm: num_logits_to_keep: %w", err)
	}
	defer numLogits.Destroy()

	// Assemble inputs in the decoder's declared input order.
	inputs := make([]ort.Value, len(m.decIn))
	for i, name := range m.decIn {
		switch m.decInRole[name] {
		case roleEmbeds:
			inputs[i] = embeds
		case rolePerLayer:
			if ple == nil {
				return nil, nil, fmt.Errorf("llm: decoder needs %q but embed produced no per_layer_inputs", name)
			}
			inputs[i] = ple
		case roleAttnMask:
			inputs[i] = attnMask
		case rolePositionIDs:
			inputs[i] = posT
		case roleNumLogits:
			inputs[i] = numLogits
		case rolePastKV:
			v, ok := past[name]
			if !ok {
				return nil, nil, fmt.Errorf("llm: missing past kv input %q", name)
			}
			inputs[i] = v
		default:
			return nil, nil, fmt.Errorf("llm: unhandled decoder input %q", name)
		}
	}

	outputs := make([]ort.Value, len(m.decOut))
	if err := m.decoder.Run(inputs, outputs); err != nil {
		return nil, nil, fmt.Errorf("llm: decoder run: %w", err)
	}

	// Extract logits (copy out) and capture present.* as the next past.
	var logits []float32
	newPast := make(map[string]ort.Value, len(m.presentToPast))
	for i, name := range m.decOut {
		v := outputs[i]
		if name == m.logitsOut {
			t, ok := v.(*ort.Tensor[float32])
			if !ok {
				return nil, nil, fmt.Errorf("llm: logits output is not float32")
			}
			data := t.GetData()
			vocab := m.cfg.VocabSize
			if vocab > 0 && len(data) > vocab {
				data = data[len(data)-vocab:] // last position only
			}
			logits = append(logits, data...)
			t.Destroy()
			continue
		}
		if past, ok := m.presentToPast[name]; ok {
			newPast[past] = v // keep alive for the next step
			continue
		}
		// Unknown extra output: free it.
		if v != nil {
			v.Destroy()
		}
	}
	if logits == nil {
		destroyValues(newPast)
		return nil, nil, fmt.Errorf("llm: decoder produced no logits")
	}
	return logits, newPast, nil
}

// embedTokens runs embed_tokens on ids and returns the inputs_embeds tensor and
// the per_layer_inputs tensor (nil if the export has none). The caller owns both
// and must Destroy them.
func (m *loadedModel) embedTokens(ids []int64) (ort.Value, ort.Value, error) {
	idsT, err := ort.NewTensor(ort.NewShape(1, int64(len(ids))), ids)
	if err != nil {
		return nil, nil, fmt.Errorf("llm: input_ids: %w", err)
	}
	defer idsT.Destroy()

	outs := make([]ort.Value, len(m.embedOut))
	if err := m.embed.Run([]ort.Value{idsT}, outs); err != nil {
		return nil, nil, fmt.Errorf("llm: embed run: %w", err)
	}

	var embeds, ple ort.Value
	for i, name := range m.embedOut {
		switch name {
		case m.embedEmbeds:
			embeds = outs[i]
		case m.embedPLE:
			ple = outs[i]
		default:
			if outs[i] != nil {
				outs[i].Destroy()
			}
		}
	}
	if embeds == nil {
		if ple != nil {
			ple.Destroy()
		}
		return nil, nil, fmt.Errorf("llm: embed produced no inputs_embeds")
	}
	return embeds, ple, nil
}

// emptyPast builds the initial (empty) KV cache: one zero-length tensor per
// past_key_values.* input, shaped from the model's DECLARED dims for that input
// (Gemma 4's hybrid attention gives sliding-window layers a wider KV head_dim
// than full-attention layers, so the shape is per-input, not one config value).
// Each dynamic dim (-1) becomes the batch size at index 0 and 0 at the sequence
// position (so the cache is empty); concrete dims (kv-heads, head_dim) are kept.
func (m *loadedModel) emptyPast() map[string]ort.Value {
	past := make(map[string]ort.Value, len(m.presentToPast))
	for _, pastName := range m.presentToPast {
		dims := m.pastDims[pastName]
		shape := emptyKVShape(dims, m.cfg.NumKVHeads, m.cfg.HeadDim)
		t, err := ort.NewTensor(shape, []float32{})
		if err != nil {
			// Extremely unlikely; leave the slot empty and let decodeStep error.
			continue
		}
		past[pastName] = t
	}
	return past
}

// emptyKVShape resolves a past_key_values declared shape to a concrete empty
// shape. KV caches are [batch, kv_heads, seq, head_dim]: batch and seq are
// dynamic (the model declares -1), so batch→1 and seq→0 (empty); kv_heads and
// head_dim are concrete and kept verbatim. When the model gives no usable dims
// (introspection unavailable), it falls back to config values.
func emptyKVShape(declared []int64, kvHeads, headDim int) ort.Shape {
	if len(declared) != 4 {
		return ort.NewShape(1, int64(kvHeads), 0, int64(headDim))
	}
	out := make([]int64, 4)
	for i, d := range declared {
		switch {
		case d > 0:
			out[i] = d // concrete (kv_heads, head_dim)
		case i == 2:
			out[i] = 0 // sequence position → empty cache
		default:
			out[i] = 1 // batch
		}
	}
	return ort.NewShape(out...)
}

// destroyValues frees every tensor in the map.
func destroyValues(m map[string]ort.Value) {
	for _, v := range m {
		if v != nil {
			v.Destroy()
		}
	}
}

// rangeI64 returns [start, start+1, ..., end-1] as int64.
func rangeI64(start, end int) []int64 {
	out := make([]int64, 0, end-start)
	for i := start; i < end; i++ {
		out = append(out, int64(i))
	}
	return out
}

// onesI64 returns a length-n slice of int64 ones (an all-attend attention mask).
func onesI64(n int) []int64 {
	out := make([]int64, n)
	for i := range out {
		out[i] = 1
	}
	return out
}
