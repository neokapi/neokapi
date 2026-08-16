package icu

import "strings"

// Tokens is what a translation of an ICU message must carry for the program
// that formats it to keep working. Everything else in the message — the text of
// each sub-message, the words around a picker — is what a translator writes.
type Tokens struct {
	// Picker is true when the message chooses between sub-messages: a plural,
	// select or selectordinal, anywhere in the message.
	Picker bool

	// Frame counts the tokens that sit outside every sub-message — the sentence
	// a picker is embedded in. There an occurrence dropped is an occurrence
	// lost, so the comparison is by count.
	Frame map[string]int

	// Inner names the tokens that appear inside a sub-message. They are
	// compared by presence rather than by count: Norwegian writes two plural
	// branches where Polish writes four, so one message carries a different
	// number of occurrences in each language without either being wrong.
	Inner map[string]bool
}

// PlaceholderTokens parses msg as ICU MessageFormat and reports the tokens a
// translation must preserve: each argument reference in its normalized spelling
// ({name}, {name, number}, {name, number, integer}), the head of each picker
// with its argument name and keyword ({count, plural}), and the # that stands
// for the number a plural was chosen by.
//
// Branch keywords are deliberately absent. Which categories a message needs is
// a property of the language — Norwegian has one and other where Polish has
// one, few, many and other — so requiring the target's set to match the
// source's would report a correct translation as broken.
func PlaceholderTokens(msg string) (Tokens, error) {
	nodes, err := Parse(msg)
	if err != nil {
		return Tokens{}, err
	}
	t := Tokens{
		Picker: HasPicker(nodes),
		Frame:  map[string]int{},
		Inner:  map[string]bool{},
	}
	t.collect(nodes, false)
	return t, nil
}

func (t *Tokens) collect(nodes []Node, inner bool) {
	add := func(tok string) {
		if inner {
			t.Inner[tok] = true
			return
		}
		t.Frame[tok]++
	}
	for _, n := range nodes {
		switch {
		case n.Type == NodeArg:
			add(n.Text)
		case n.Type == NodeHash:
			add("#")
		case n.Type.Picker():
			add(pickerToken(n))
			for _, br := range n.Branches {
				t.collect(br.Body, true)
			}
		}
	}
}

// pickerToken spells a plural/select/selectordinal head as the part of it a
// translation must keep: the argument name, the keyword, and the offset that
// shifts the number a plural formats.
func pickerToken(n Node) string {
	var b strings.Builder
	b.WriteString("{")
	b.WriteString(n.ArgName)
	b.WriteString(", ")
	b.WriteString(strings.ToLower(n.ArgType))
	if n.Offset != "" {
		b.WriteString(", offset:")
		b.WriteString(n.Offset)
	}
	b.WriteString("}")
	return b.String()
}
