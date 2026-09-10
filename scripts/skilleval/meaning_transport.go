package main

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// meaningTransport records a narrow envelope transformation separately from
// payload validation. The agent result always retains the original final text.
type meaningTransport struct {
	Contract       string   `json:"contract"`
	Transformation string   `json:"transformation"`
	RawSHA256      string   `json:"raw_sha256"`
	PayloadSHA256  string   `json:"payload_sha256"`
	RawErrors      []string `json:"raw_errors"`
}

func validateMeaningTransport(text, protocol string, input meaningInput) meaningIntegrity {
	raw := validateMeaningPayload(text, protocol, input)
	if raw.Valid {
		return raw
	}
	payload, ok := meaningFencePayload(text)
	if !ok {
		return raw
	}
	result := validateMeaningPayload(payload, protocol, input)
	result.Transport = &meaningTransport{
		Contract: "meaning-json-envelope/v1", Transformation: "remove-single-json-fence",
		RawSHA256: meaningTextHash(text), PayloadSHA256: meaningTextHash(payload), RawErrors: raw.Errors,
	}
	return result
}

// Accept only a single complete backtick fence, with no prose outside it. The
// payload still passes the selected protocol's full validator. Do not search
// for JSON, repair syntax, or discard text surrounding an answer.
func meaningFencePayload(text string) (string, bool) {
	envelope := strings.TrimSpace(text)
	header, rest, found := strings.Cut(envelope, "\n")
	header = strings.TrimSuffix(header, "\r")
	if !found || (header != "```json" && header != "```") {
		return "", false
	}
	if !strings.HasSuffix(rest, "\n```") {
		return "", false
	}
	payload := strings.TrimSuffix(rest, "\n```")
	if strings.TrimSpace(payload) == "" {
		return "", false
	}
	return payload, true
}

func meaningTextHash(text string) string {
	hash := sha256.Sum256([]byte(text))
	return hex.EncodeToString(hash[:])
}
