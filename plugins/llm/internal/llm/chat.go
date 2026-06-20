package llm

import "strings"

// Gemma chat-template special-token strings. Gemma has no dedicated system role:
// system guidance is folded into the first user turn. Turns are wrapped in
// <start_of_turn>{role}\n ... <end_of_turn>, and generation is primed with an
// open model turn.
const (
	tokBOS       = "<bos>"
	tokStartTurn = "<start_of_turn>"
	tokEndTurn   = "<end_of_turn>"
	roleUser     = "user"
	roleModel    = "model"
)

// mapRole normalizes an API role to a Gemma turn role. "assistant" and "model"
// both map to "model"; everything else is treated as a user turn (system is
// merged into the user turn by renderPrompt).
func mapRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "assistant", "model":
		return roleModel
	default:
		return roleUser
	}
}

// renderPrompt builds the Gemma chat string for a conversation, primed with an
// open model turn so the model continues as the assistant. A leading system
// message (role "system") is prepended to the first user turn separated by a
// blank line. The returned string is what the tokenizer encodes; the literal
// special-token substrings (<bos>, <start_of_turn>, <end_of_turn>) are mapped to
// their vocabulary ids by the tokenizer's added-token handling.
//
// Media placeholders are NOT emitted here — multimodal splicing happens at the
// embedding level in the ONNX engine, which inserts the right number of image/
// audio soft tokens once an encoder reports its output length.
func renderPrompt(messages []Message) string {
	var b strings.Builder
	b.WriteString(tokBOS)

	system := ""
	for _, m := range messages {
		if strings.EqualFold(strings.TrimSpace(m.Role), "system") {
			if t := strings.TrimSpace(m.Text); t != "" {
				if system != "" {
					system += "\n\n"
				}
				system += t
			}
		}
	}

	firstUserDone := false
	for _, m := range messages {
		if strings.EqualFold(strings.TrimSpace(m.Role), "system") {
			continue
		}
		role := mapRole(m.Role)
		text := m.Text
		if role == roleUser && !firstUserDone && system != "" {
			text = system + "\n\n" + text
			firstUserDone = true
		}
		b.WriteString(tokStartTurn)
		b.WriteString(role)
		b.WriteString("\n")
		b.WriteString(text)
		b.WriteString(tokEndTurn)
		b.WriteString("\n")
	}

	// Prime the model's turn.
	b.WriteString(tokStartTurn)
	b.WriteString(roleModel)
	b.WriteString("\n")
	return b.String()
}

// cleanOutput trims any trailing turn terminator the model may emit and strips a
// leading/trailing whitespace so the host gets just the assistant text.
func cleanOutput(s string) string {
	s = strings.TrimSuffix(strings.TrimSpace(s), tokEndTurn)
	// A model turn occasionally restates its opener; drop a leading one.
	s = strings.TrimPrefix(s, tokStartTurn+roleModel+"\n")
	return strings.TrimSpace(s)
}
