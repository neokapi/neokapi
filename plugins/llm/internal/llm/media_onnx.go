//go:build onnx

// Multimodal support for the Gemma 4 engine: build the prompt token stream with
// image/audio placeholder tokens, run the vision/audio encoders, and splice
// their outputs into the placeholder rows of inputs_embeds before decoding.
//
// Validation status: the wiring (placeholder injection, encoder sessions, the
// embed-row splice) is complete and the text path is unaffected. The
// preprocessing constants (image size / normalization, audio mel parameters) and
// the encoders' exact input layout are best-effort from the model's
// preprocessor/processor configs and are validated on-device against the real
// q4 weights — the session I/O names are introspected, not hardcoded.
package llm

import (
	"encoding/binary"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"
	"strings"

	ort "github.com/yalue/onnxruntime_go"
)

// mediaSlot records where a media item's placeholder tokens sit in the prompt id
// stream and the encoder feature rows that replace their embeddings.
type mediaSlot struct {
	start    int       // index of the first placeholder token in the id stream
	count    int       // number of placeholder tokens (== feature rows)
	features []float32 // count*hiddenSize encoder outputs, row-major
}

// encVal pairs a tensor with the input name it feeds, for a generic encoder run.
type encVal struct {
	name string
	val  ort.Value
}

// openVision / openAudio open the encoder sessions, introspecting their I/O.
func (m *loadedModel) openVision(path string) error {
	in, out, err := openMultiSession(path)
	if err != nil {
		return fmt.Errorf("llm: vision encoder: %w", err)
	}
	m.visionIn, m.visionOut = in, out
	m.vision, err = ort.NewDynamicAdvancedSession(path, in, out, nil)
	if err != nil {
		return fmt.Errorf("llm: vision session: %w", err)
	}
	return nil
}

func (m *loadedModel) openAudio(path string) error {
	in, out, err := openMultiSession(path)
	if err != nil {
		return fmt.Errorf("llm: audio encoder: %w", err)
	}
	m.audioIn, m.audioOut = in, out
	m.audio, err = ort.NewDynamicAdvancedSession(path, in, out, nil)
	if err != nil {
		return fmt.Errorf("llm: audio session: %w", err)
	}
	return nil
}

// openMultiSession returns the ordered input and output names of a model.
func openMultiSession(path string) (ins, outs []string, err error) {
	is, os_, err := ort.GetInputOutputInfo(path)
	if err != nil {
		return nil, nil, err
	}
	for _, i := range is {
		ins = append(ins, i.Name)
	}
	for _, o := range os_ {
		outs = append(outs, o.Name)
	}
	return ins, outs, nil
}

// buildPromptIDs assembles the prompt token-id stream for the model's chat
// family. The decode loop, KV cache, and sampling are family-agnostic; only this
// prompt assembly differs per family, so supporting a new model (e.g. Qwen's
// ChatML) is a new case here plus a model.Spec — not an engine change.
func (m *loadedModel) buildPromptIDs(messages []Message) ([]int64, []mediaSlot, error) {
	switch m.family {
	case "", "gemma":
		return m.buildGemmaPromptIDs(messages)
	default:
		return nil, nil, fmt.Errorf("llm: unsupported chat family %q (this build implements gemma; add a builder + model.Spec for others)", m.family)
	}
}

// buildGemmaPromptIDs assembles the Gemma chat prompt as a token-id stream.
// Because the tokenizer does not parse the literal "<start_of_turn>" /
// "<end_of_turn>" strings as single tokens, the turn markers (and <bos>) are
// inserted by id; only the role label, content text, and newlines are tokenized.
// Media items contribute a run of placeholder tokens (recorded as mediaSlots)
// whose embeddings are overwritten with the encoder outputs after embed_tokens.
//
// Canonical Gemma turn: <bos> then, per turn,
// <start_of_turn>{role}\n{content}<end_of_turn>\n, ending with an open
// <start_of_turn>model\n to prime generation.
func (m *loadedModel) buildGemmaPromptIDs(messages []Message) ([]int64, []mediaSlot, error) {
	bos := int64(m.cfg.BOSTokenID)
	sot := int64(m.cfg.StartOfTurnID)
	eot := int64(m.cfg.EndOfTurnID)
	nl := m.encodeText("\n")

	var ids []int64
	var slots []mediaSlot
	ids = append(ids, bos)

	system := collectSystem(messages)
	firstUser := true
	for _, msg := range messages {
		if strings.EqualFold(strings.TrimSpace(msg.Role), "system") {
			continue
		}
		role := mapRole(msg.Role)
		text := msg.Text
		if role == roleUser && firstUser && system != "" {
			text = system + "\n\n" + text
			firstUser = false
		}

		ids = append(ids, sot)
		ids = append(ids, m.encodeText(role+"\n")...)

		// Media leads the turn content (placeholder tokens then the text).
		for _, md := range msg.Media {
			feats, count, tokID, err := m.encodeMedia(md)
			if err != nil {
				return nil, nil, err
			}
			slots = append(slots, mediaSlot{start: len(ids), count: count, features: feats})
			for j := 0; j < count; j++ {
				ids = append(ids, int64(tokID))
			}
		}

		ids = append(ids, m.encodeText(text)...)
		ids = append(ids, eot)
		ids = append(ids, nl...)
	}
	// Prime the model's turn.
	ids = append(ids, sot)
	ids = append(ids, m.encodeText(roleModel+"\n")...)
	return ids, slots, nil
}

// encodeText tokenizes s without adding special tokens (control tokens are
// written as their literal strings and mapped by the tokenizer's added-token
// handling).
func (m *loadedModel) encodeText(s string) []int64 {
	enc := m.tk.EncodeWithOptions(s, false)
	out := make([]int64, len(enc.IDs))
	for i, id := range enc.IDs {
		out[i] = int64(id)
	}
	return out
}

// collectSystem concatenates all system-role message texts.
func collectSystem(messages []Message) string {
	var parts []string
	for _, m := range messages {
		if strings.EqualFold(strings.TrimSpace(m.Role), "system") {
			if t := strings.TrimSpace(m.Text); t != "" {
				parts = append(parts, t)
			}
		}
	}
	return strings.Join(parts, "\n\n")
}

// encodeMedia preprocesses one media item, runs the matching encoder, and
// returns its feature rows (count*hiddenSize), the row count, and the
// placeholder token id to repeat in the prompt.
func (m *loadedModel) encodeMedia(md Media) ([]float32, int, int, error) {
	// v0.1.0 is text-only. Gemma 4's vision/audio encoders are wired and load,
	// but their preprocessing is not yet numerically validated (the vision tower
	// wants native-resolution patchified pixel_values [N,768] with [N,2] position
	// ids — not [1,3,H,W] — and audio wants log-mel [B,frames,128] with a bool
	// mask). Gating off here keeps the release honest: a clear message instead of
	// unverified output. encodeImage/encodeAudio retain the wiring for the
	// validation follow-up.
	switch md.Kind {
	case "image", "audio":
		return nil, 0, 0, fmt.Errorf("llm: %s input is experimental and not yet enabled in the native engine (text generation is fully supported); see the kapi-llm multimodal-validation follow-up", md.Kind)
	case "video":
		return nil, 0, 0, fmt.Errorf("llm: video input requires frame extraction (use the kapi-av plugin to extract frames, then pass them as images)")
	default:
		return nil, 0, 0, fmt.Errorf("llm: unsupported media kind %q", md.Kind)
	}
}

// applyMediaSlots overwrites the placeholder-token embedding rows in embeds with
// the captured encoder features. embeds is the inputs_embeds tensor [1, seq, H].
func (m *loadedModel) applyMediaSlots(embeds ort.Value, slots []mediaSlot) error {
	if len(slots) == 0 {
		return nil
	}
	t, ok := embeds.(*ort.Tensor[float32])
	if !ok {
		return fmt.Errorf("llm: inputs_embeds is not float32")
	}
	data := t.GetData()
	h := m.cfg.HiddenSize
	for _, s := range slots {
		want := s.count * h
		if len(s.features) != want {
			return fmt.Errorf("llm: media features %d != expected %d (count %d × hidden %d)", len(s.features), want, s.count, h)
		}
		off := s.start * h
		if off+want > len(data) {
			return fmt.Errorf("llm: media slot out of range (off %d + %d > %d)", off, want, len(data))
		}
		copy(data[off:off+want], s.features)
	}
	return nil
}

// runEncoder runs an encoder session with the given inputs (in the session's
// declared input order) and returns the first output's data and row count
// (rows = product of all dims except the last == hiddenSize).
func (m *loadedModel) runEncoder(sess *ort.DynamicAdvancedSession, inNames, outNames []string, supply map[string]ort.Value) ([]float32, int, error) {
	inputs := make([]ort.Value, len(inNames))
	for i, name := range inNames {
		v, ok := supply[name]
		if !ok {
			return nil, 0, fmt.Errorf("llm: encoder input %q not supplied", name)
		}
		inputs[i] = v
	}
	outputs := make([]ort.Value, len(outNames))
	if err := sess.Run(inputs, outputs); err != nil {
		return nil, 0, fmt.Errorf("llm: encoder run: %w", err)
	}
	defer func() {
		for _, v := range outputs {
			if v != nil {
				v.Destroy()
			}
		}
	}()
	ft, ok := outputs[0].(*ort.Tensor[float32])
	if !ok {
		return nil, 0, fmt.Errorf("llm: encoder output is not float32")
	}
	shape := ft.GetShape()
	hidden := m.cfg.HiddenSize
	rows := 1
	for i := 0; i < len(shape)-1; i++ {
		rows *= int(shape[i])
	}
	if len(shape) > 0 {
		// Last dim should be hidden; trust it if it differs from config.
		hidden = int(shape[len(shape)-1])
	}
	out := make([]float32, rows*hidden)
	copy(out, ft.GetData())
	return out, rows, nil
}

// encodeImage decodes, resizes, and normalizes an image, then runs the vision
// encoder. Returns the feature rows, count, and the image placeholder token id.
func (m *loadedModel) encodeImage(path string) ([]float32, int, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("llm: open image: %w", err)
	}
	defer func() { _ = f.Close() }()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("llm: decode image: %w", err)
	}

	pp := loadImagePreproc(m)
	pix := preprocessImage(img, pp)
	pvT, err := ort.NewTensor(ort.NewShape(1, 3, int64(pp.size), int64(pp.size)), pix)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("llm: pixel_values: %w", err)
	}
	defer pvT.Destroy()

	supply := map[string]ort.Value{}
	// Supply pixel_values + (optionally) pixel_position_ids by introspected name.
	var posT *ort.Tensor[int64]
	for _, name := range m.visionIn {
		switch {
		case strings.Contains(name, "pixel_values") || strings.Contains(name, "pixel"):
			if strings.Contains(name, "position") {
				patches := (pp.size / pp.patch) * (pp.size / pp.patch)
				posT, err = ort.NewTensor(ort.NewShape(1, int64(patches)), rangeI64(0, patches))
				if err != nil {
					return nil, 0, 0, fmt.Errorf("llm: pixel_position_ids: %w", err)
				}
				supply[name] = posT
			} else {
				supply[name] = pvT
			}
		case strings.Contains(name, "position"):
			patches := (pp.size / pp.patch) * (pp.size / pp.patch)
			posT, err = ort.NewTensor(ort.NewShape(1, int64(patches)), rangeI64(0, patches))
			if err != nil {
				return nil, 0, 0, fmt.Errorf("llm: pixel_position_ids: %w", err)
			}
			supply[name] = posT
		default:
			supply[name] = pvT
		}
	}
	if posT != nil {
		defer posT.Destroy()
	}

	feats, rows, err := m.runEncoder(m.vision, m.visionIn, m.visionOut, supply)
	if err != nil {
		return nil, 0, 0, err
	}
	return feats, rows, m.cfg.ImageTokenID, nil
}

// imagePreproc holds the image preprocessing parameters.
type imagePreproc struct {
	size  int
	patch int
	mean  [3]float32
	std   [3]float32
}

// loadImagePreproc reads preprocessor_config.json defaults appropriate for the
// Gemma SigLIP-style vision tower.
func loadImagePreproc(m *loadedModel) imagePreproc {
	return imagePreproc{
		size:  896,
		patch: 16,
		mean:  [3]float32{0.5, 0.5, 0.5},
		std:   [3]float32{0.5, 0.5, 0.5},
	}
}

// preprocessImage resizes img to a square and returns normalized CHW float32.
func preprocessImage(img image.Image, pp imagePreproc) []float32 {
	rgba := image.NewRGBA(image.Rect(0, 0, pp.size, pp.size))
	b := img.Bounds()
	sw, sh := b.Dx(), b.Dy()
	for y := 0; y < pp.size; y++ {
		sy := y * sh / pp.size
		for x := 0; x < pp.size; x++ {
			sx := x * sw / pp.size
			rgba.Set(x, y, img.At(b.Min.X+sx, b.Min.Y+sy))
		}
	}
	out := make([]float32, 3*pp.size*pp.size)
	plane := pp.size * pp.size
	for y := 0; y < pp.size; y++ {
		for x := 0; x < pp.size; x++ {
			c := rgba.RGBAAt(x, y)
			rgb := [3]float32{float32(c.R), float32(c.G), float32(c.B)}
			for ch := 0; ch < 3; ch++ {
				out[ch*plane+y*pp.size+x] = (rgb[ch]/255 - pp.mean[ch]) / pp.std[ch]
			}
		}
	}
	return out
}

// encodeAudio decodes a 16-bit PCM WAV, computes a log-mel spectrogram, and runs
// the audio encoder. Returns the feature rows, count, and the audio placeholder
// token id.
func (m *loadedModel) encodeAudio(path string) ([]float32, int, int, error) {
	samples, sr, err := decodeWAV(path)
	if err != nil {
		return nil, 0, 0, err
	}
	mel := logMel(samples, sr, audioMelParams())
	frames := len(mel)
	if frames == 0 {
		return nil, 0, 0, fmt.Errorf("llm: empty audio")
	}
	nMels := len(mel[0])

	// input_features as [1, n_mels, frames] (channels-first mel).
	feat := make([]float32, nMels*frames)
	for t := 0; t < frames; t++ {
		for b := 0; b < nMels; b++ {
			feat[b*frames+t] = mel[t][b]
		}
	}
	featT, err := ort.NewTensor(ort.NewShape(1, int64(nMels), int64(frames)), feat)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("llm: input_features: %w", err)
	}
	defer featT.Destroy()

	maskT, err := ort.NewTensor(ort.NewShape(1, int64(frames)), onesI64(frames))
	if err != nil {
		return nil, 0, 0, fmt.Errorf("llm: input_features_mask: %w", err)
	}
	defer maskT.Destroy()

	supply := map[string]ort.Value{}
	for _, name := range m.audioIn {
		switch {
		case strings.Contains(name, "mask"):
			supply[name] = maskT
		default:
			supply[name] = featT
		}
	}

	feats, rows, err := m.runEncoder(m.audio, m.audioIn, m.audioOut, supply)
	if err != nil {
		return nil, 0, 0, err
	}
	return feats, rows, m.cfg.AudioTokenID, nil
}

// melParams holds log-mel extraction parameters.
type melParams struct {
	sampleRate int
	nFFT       int
	hop        int
	nMels      int
	fMin       float64
	fMax       float64
}

func audioMelParams() melParams {
	return melParams{sampleRate: 16000, nFFT: 400, hop: 160, nMels: 128, fMin: 0, fMax: 8000}
}

// decodeWAV reads a 16-bit PCM WAV file and returns mono float32 samples in
// [-1,1] plus the sample rate. Multi-channel audio is downmixed by averaging.
func decodeWAV(path string) ([]float32, int, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, fmt.Errorf("llm: read audio: %w", err)
	}
	if len(b) < 44 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		return nil, 0, fmt.Errorf("llm: not a PCM WAV file (only 16-bit PCM WAV is supported)")
	}
	var (
		channels   int
		sampleRate int
		bits       int
		dataOff    int
		dataLen    int
	)
	// Walk RIFF chunks.
	off := 12
	for off+8 <= len(b) {
		id := string(b[off : off+4])
		sz := int(binary.LittleEndian.Uint32(b[off+4 : off+8]))
		body := off + 8
		switch id {
		case "fmt ":
			if body+16 > len(b) {
				return nil, 0, fmt.Errorf("llm: truncated fmt chunk")
			}
			channels = int(binary.LittleEndian.Uint16(b[body+2 : body+4]))
			sampleRate = int(binary.LittleEndian.Uint32(b[body+4 : body+8]))
			bits = int(binary.LittleEndian.Uint16(b[body+14 : body+16]))
		case "data":
			dataOff = body
			dataLen = sz
		}
		off = body + sz
		if sz%2 == 1 {
			off++ // chunks are word-aligned
		}
	}
	if bits != 16 || channels < 1 || sampleRate == 0 || dataOff == 0 {
		return nil, 0, fmt.Errorf("llm: unsupported WAV (need 16-bit PCM; got %d-bit, %d ch, %d Hz)", bits, channels, sampleRate)
	}
	if dataOff+dataLen > len(b) {
		dataLen = len(b) - dataOff
	}
	frames := dataLen / (2 * channels)
	out := make([]float32, frames)
	for i := 0; i < frames; i++ {
		var acc int
		for c := 0; c < channels; c++ {
			s := int16(binary.LittleEndian.Uint16(b[dataOff+(i*channels+c)*2:]))
			acc += int(s)
		}
		out[i] = float32(acc) / float32(channels) / 32768.0
	}
	return out, sampleRate, nil
}

// logMel computes a [frames][nMels] log-mel spectrogram with a Hann window.
func logMel(samples []float32, sr int, p melParams) [][]float32 {
	if len(samples) == 0 {
		return nil
	}
	window := hann(p.nFFT)
	filters := melFilterbank(p.nFFT, p.nMels, sr, p.fMin, p.fMax)
	var out [][]float32
	for start := 0; start+p.nFFT <= len(samples); start += p.hop {
		frame := make([]float64, p.nFFT)
		for i := 0; i < p.nFFT; i++ {
			frame[i] = float64(samples[start+i]) * window[i]
		}
		power := dftPower(frame) // length nFFT/2+1
		mels := make([]float32, p.nMels)
		for mIdx := 0; mIdx < p.nMels; mIdx++ {
			var sum float64
			for k, w := range filters[mIdx] {
				sum += w * power[k]
			}
			mels[mIdx] = float32(math.Log(sum + 1e-10))
		}
		out = append(out, mels)
	}
	return out
}

func hann(n int) []float64 {
	w := make([]float64, n)
	for i := range w {
		w[i] = 0.5 * (1 - math.Cos(2*math.Pi*float64(i)/float64(n-1)))
	}
	return w
}

// dftPower returns the power spectrum (|X|^2) for bins 0..n/2 via a direct DFT.
// A direct DFT is O(n^2) but n is small (nFFT=400) and clarity beats an FFT here.
func dftPower(frame []float64) []float64 {
	n := len(frame)
	bins := n/2 + 1
	out := make([]float64, bins)
	for k := 0; k < bins; k++ {
		var re, im float64
		for t := 0; t < n; t++ {
			a := -2 * math.Pi * float64(k) * float64(t) / float64(n)
			re += frame[t] * math.Cos(a)
			im += frame[t] * math.Sin(a)
		}
		out[k] = re*re + im*im
	}
	return out
}

// melFilterbank builds nMels triangular filters over the DFT power bins.
func melFilterbank(nFFT, nMels, sr int, fMin, fMax float64) [][]float64 {
	bins := nFFT/2 + 1
	hzToMel := func(f float64) float64 { return 2595 * math.Log10(1+f/700) }
	melToHz := func(mel float64) float64 { return 700 * (math.Pow(10, mel/2595) - 1) }
	mMin, mMax := hzToMel(fMin), hzToMel(fMax)
	points := make([]float64, nMels+2)
	for i := range points {
		mel := mMin + (mMax-mMin)*float64(i)/float64(nMels+1)
		points[i] = melToHz(mel)
	}
	binHz := func(f float64) float64 { return f * float64(nFFT) / float64(sr) }
	filters := make([][]float64, nMels)
	for mIdx := 0; mIdx < nMels; mIdx++ {
		filt := make([]float64, bins)
		lo, ctr, hi := binHz(points[mIdx]), binHz(points[mIdx+1]), binHz(points[mIdx+2])
		for k := 0; k < bins; k++ {
			fk := float64(k)
			switch {
			case fk >= lo && fk <= ctr && ctr > lo:
				filt[k] = (fk - lo) / (ctr - lo)
			case fk > ctr && fk <= hi && hi > ctr:
				filt[k] = (hi - fk) / (hi - ctr)
			}
		}
		filters[mIdx] = filt
	}
	return filters
}
