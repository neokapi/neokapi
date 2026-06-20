//go:build onnx

package llm

import (
	"encoding/json"
	"fmt"
	"os"
)

// modelConfig holds the architecture + special-token facts the decode loop needs,
// parsed from the model's config.json and generation_config.json.
type modelConfig struct {
	HiddenSize    int
	NumLayers     int
	NumKVHeads    int
	HeadDim       int
	SlidingWindow int
	VocabSize     int

	ImageTokenID int
	AudioTokenID int
	BOSTokenID   int
	EOSTokenIDs  []int

	// MMTokensPerImage is the fixed number of soft tokens one image expands to
	// in the prompt (Gemma uses a constant). Zero means "derive from the vision
	// encoder output length".
	MMTokensPerImage int
}

// isEOS reports whether id is a stop token.
func (c modelConfig) isEOS(id int) bool {
	for _, e := range c.EOSTokenIDs {
		if id == e {
			return true
		}
	}
	return false
}

// rawConfig mirrors the subset of config.json we read. Gemma 4 nests the text
// hyper-parameters under text_config; the multimodal token ids sit at the top
// level.
type rawConfig struct {
	ImageTokenID     *int `json:"image_token_id"`
	AudioTokenID     *int `json:"audio_token_id"`
	BOSTokenID       *int `json:"bos_token_id"`
	MMTokensPerImage *int `json:"mm_tokens_per_image"`

	TextConfig struct {
		HiddenSize       *int `json:"hidden_size"`
		NumHiddenLayers  *int `json:"num_hidden_layers"`
		NumAttnHeads     *int `json:"num_attention_heads"`
		NumKeyValueHeads *int `json:"num_key_value_heads"`
		HeadDim          *int `json:"head_dim"`
		SlidingWindow    *int `json:"sliding_window"`
		VocabSize        *int `json:"vocab_size"`
	} `json:"text_config"`

	// Some exports also place these at the top level; used as a fallback.
	HiddenSize       *int            `json:"hidden_size"`
	NumHiddenLayers  *int            `json:"num_hidden_layers"`
	NumKeyValueHeads *int            `json:"num_key_value_heads"`
	HeadDim          *int            `json:"head_dim"`
	VocabSize        *int            `json:"vocab_size"`
	EOSTokenID       json.RawMessage `json:"eos_token_id"`
}

// rawGenConfig mirrors generation_config.json (the authoritative stop tokens).
type rawGenConfig struct {
	EOSTokenID json.RawMessage `json:"eos_token_id"`
	BOSTokenID *int            `json:"bos_token_id"`
}

// parseIntOrList decodes a JSON value that is either an int or a list of ints.
func parseIntOrList(raw json.RawMessage) []int {
	if len(raw) == 0 {
		return nil
	}
	var one int
	if err := json.Unmarshal(raw, &one); err == nil {
		return []int{one}
	}
	var many []int
	if err := json.Unmarshal(raw, &many); err == nil {
		return many
	}
	return nil
}

// loadConfig reads config.json and generation_config.json, applying sane Gemma 4
// E2B defaults for any field the files omit.
func loadConfig(configPath, genPath string) (modelConfig, error) {
	cfg := modelConfig{
		// Gemma 4 E2B defaults (used only when the file omits a field).
		HiddenSize:    1536,
		NumLayers:     35,
		NumKVHeads:    1,
		HeadDim:       256,
		SlidingWindow: 512,
		VocabSize:     262144,
		BOSTokenID:    2,
		EOSTokenIDs:   []int{1, 106}, // <eos>, <end_of_turn>
	}

	if configPath != "" {
		b, err := os.ReadFile(configPath)
		if err != nil {
			return modelConfig{}, fmt.Errorf("llm: read config.json: %w", err)
		}
		var rc rawConfig
		if err := json.Unmarshal(b, &rc); err != nil {
			return modelConfig{}, fmt.Errorf("llm: parse config.json: %w", err)
		}
		setInt(&cfg.HiddenSize, rc.TextConfig.HiddenSize, rc.HiddenSize)
		setInt(&cfg.NumLayers, rc.TextConfig.NumHiddenLayers, rc.NumHiddenLayers)
		setInt(&cfg.NumKVHeads, rc.TextConfig.NumKeyValueHeads, rc.NumKeyValueHeads)
		setInt(&cfg.HeadDim, rc.TextConfig.HeadDim, rc.HeadDim)
		setInt(&cfg.SlidingWindow, rc.TextConfig.SlidingWindow, nil)
		setInt(&cfg.VocabSize, rc.TextConfig.VocabSize, rc.VocabSize)
		setInt(&cfg.ImageTokenID, rc.ImageTokenID, nil)
		setInt(&cfg.AudioTokenID, rc.AudioTokenID, nil)
		setInt(&cfg.BOSTokenID, rc.BOSTokenID, nil)
		setInt(&cfg.MMTokensPerImage, rc.MMTokensPerImage, nil)
		if eos := parseIntOrList(rc.EOSTokenID); len(eos) > 0 {
			cfg.EOSTokenIDs = eos
		}
	}

	if genPath != "" {
		if b, err := os.ReadFile(genPath); err == nil {
			var gc rawGenConfig
			if err := json.Unmarshal(b, &gc); err == nil {
				if eos := parseIntOrList(gc.EOSTokenID); len(eos) > 0 {
					cfg.EOSTokenIDs = eos
				}
				setInt(&cfg.BOSTokenID, gc.BOSTokenID, nil)
			}
		}
	}

	return cfg, nil
}

// setInt assigns the first non-nil candidate to *dst (leaving the default when
// all candidates are nil).
func setInt(dst *int, candidates ...*int) {
	for _, c := range candidates {
		if c != nil {
			*dst = *c
			return
		}
	}
}
