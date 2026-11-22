package contextop

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/yamledit"
)

// VoiceEdit is what a whole-profile write did to a voice profile, carried by
// the edit operation that records it. It is what a reader drills into, and what
// dropping the edit restores.
type VoiceEdit struct {
	// Profile is the id of the voice profile that was written.
	Profile string `json:"profile"`
	// Changes are the fields that differ between the profile as it stood and
	// as it was written, in the order the authored document lists them.
	// Omitted lists the changes past the cap a record keeps.
	Changes []VoiceChange `json:"changes,omitempty"`
	Omitted int           `json:"omitted,omitempty"`
	// Previous is the profile as it stood before the write, as the YAML a
	// project authors. Empty when the write created the profile.
	Previous string `json:"previous,omitempty"`
	// Wrote fingerprints the profile as the write left it, so dropping the
	// edit restores Previous only while nothing has written the profile since.
	Wrote string `json:"wrote,omitempty"`
}

// VoiceChange is one field a voice-profile write changed. From is empty for a
// field the write added, and To for one it removed.
type VoiceChange struct {
	// Field is the field's path in the authored document: "tone.formality",
	// "constraints[no-passive].statement".
	Field string `json:"field"`
	From  string `json:"from,omitempty"`
	To    string `json:"to,omitempty"`
}

// What a whole-profile write changed, kept on the edit operation that records
// it: the fields that differ, the profile as it stood, and a fingerprint of
// what the write left, so dropping the edit can put the profile back.

// voiceChangeCap bounds the changes one record keeps, and voiceValueCap the
// length of one value as a change shows it. The record keeps the previous
// profile whole, so nothing a restore needs is lost to either cap.
const (
	voiceChangeCap = 40
	voiceValueCap  = 240
)

// NewVoiceEdit compares the profile as it stood with the profile the store now
// holds. before is nil when the write created the profile, and after is nil
// when the write removed it.
func NewVoiceEdit(before, after *profile.VoiceProfile) (*VoiceEdit, error) {
	if before == nil && after == nil {
		return nil, errors.New("voice: no profile to compare")
	}
	edit := &VoiceEdit{}
	if after != nil {
		edit.Profile = after.ID
		wrote, err := VoiceFingerprint(after)
		if err != nil {
			return nil, err
		}
		edit.Wrote = wrote
	} else {
		edit.Profile = before.ID
	}
	left, err := voiceTree(before)
	if err != nil {
		return nil, err
	}
	right, err := voiceTree(after)
	if err != nil {
		return nil, err
	}
	var changes []VoiceChange
	diffVoiceTree("", left, right, &changes)
	if len(changes) > voiceChangeCap {
		edit.Omitted = len(changes) - voiceChangeCap
		changes = changes[:voiceChangeCap]
	}
	edit.Changes = changes
	if before != nil {
		prev, err := yamledit.Marshal(nil, profile.Authored(before))
		if err != nil {
			return nil, fmt.Errorf("render voice profile %s: %w", before.ID, err)
		}
		edit.Previous = string(prev)
	}
	return edit, nil
}

// voiceContent is a profile as its content: what it authors, without the
// version the store increments on every write.
func voiceContent(p *profile.VoiceProfile) *profile.VoiceProfile {
	out := profile.Authored(p)
	if out != nil {
		out.Version = 0
	}
	return out
}

// VoiceFingerprint identifies a profile's content: the same authored profile
// has the same fingerprint whatever version, timestamps or partition key the
// store gave it. A VoiceEdit's Wrote is the fingerprint of what it wrote.
func VoiceFingerprint(p *profile.VoiceProfile) (string, error) {
	doc, err := yamledit.Marshal(nil, voiceContent(p))
	if err != nil {
		return "", fmt.Errorf("fingerprint voice profile %s: %w", p.ID, err)
	}
	sum := sha256.Sum256(doc)
	return hex.EncodeToString(sum[:]), nil
}

// voiceTree is a profile's content as a generic tree, for comparing field by
// field. A nil profile is an empty tree.
func voiceTree(p *profile.VoiceProfile) (map[string]any, error) {
	out := map[string]any{}
	if p == nil {
		return out, nil
	}
	data, err := json.Marshal(voiceContent(p))
	if err != nil {
		return nil, fmt.Errorf("compare voice profile %s: %w", p.ID, err)
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("compare voice profile %s: %w", p.ID, err)
	}
	return out, nil
}

// diffVoiceTree appends a change for each leaf that differs between a and b.
// Objects are compared key by key; a list whose every item carries an id (the
// constraints) is compared item by item under that id; anything else is
// compared whole.
func diffVoiceTree(path string, a, b any, out *[]VoiceChange) {
	if reflect.DeepEqual(a, b) {
		return
	}
	am, aok := a.(map[string]any)
	bm, bok := b.(map[string]any)
	// An object one side lacks is one change, shown whole, except at the top
	// of a profile the write created, where each field is listed.
	whole := path != "" && (a == nil || b == nil)
	if !whole && (aok || a == nil) && (bok || b == nil) && (aok || bok) {
		for _, k := range unionKeys(am, bm) {
			diffVoiceTree(joinField(path, k), am[k], bm[k], out)
		}
		return
	}
	if al, ok := voiceIDList(a); ok {
		if bl, ok := voiceIDList(b); ok {
			for _, id := range unionKeys(al, bl) {
				diffVoiceTree(path+"["+id+"]", al[id], bl[id], out)
			}
			return
		}
	}
	// A list of objects with no ids (patterns, examples) is compared by
	// position, so an edited description is one field rather than the list.
	if al, ok := objectList(a); ok {
		if bl, ok := objectList(b); ok {
			for i := range max(len(al), len(bl)) {
				var x, y any
				if i < len(al) {
					x = al[i]
				}
				if i < len(bl) {
					y = bl[i]
				}
				diffVoiceTree(fmt.Sprintf("%s[%d]", path, i+1), x, y, out)
			}
			return
		}
	}
	from, to := voiceValue(a), voiceValue(b)
	if from == to {
		return
	}
	*out = append(*out, VoiceChange{Field: path, From: from, To: to})
}

// voiceIDList indexes a list by the id each item carries. ok is false for
// anything else. A nil list indexes as empty, so a list added whole is
// compared item by item against nothing.
func voiceIDList(v any) (map[string]any, bool) {
	list, ok := v.([]any)
	if !ok && v != nil {
		return nil, false
	}
	out := make(map[string]any, len(list))
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, false
		}
		id, ok := m["id"].(string)
		if !ok || id == "" {
			return nil, false
		}
		out[id] = item
	}
	return out, true
}

// objectList reports a list whose every item is an object. A nil list is an
// empty one.
func objectList(v any) ([]any, bool) {
	list, ok := v.([]any)
	if !ok {
		return nil, v == nil
	}
	for _, item := range list {
		if _, ok := item.(map[string]any); !ok {
			return nil, false
		}
	}
	return list, true
}

func unionKeys(a, b map[string]any) []string {
	keys := make([]string, 0, len(a)+len(b))
	for k := range a {
		keys = append(keys, k)
	}
	for k := range b {
		if _, ok := a[k]; !ok {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	return keys
}

func joinField(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

// voiceValue renders one value the way a change shows it: text as itself, a
// list of words joined, anything larger as compact JSON. An empty value
// renders empty, so an absent field and an empty one compare equal.
func voiceValue(v any) string {
	var s string
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		s = x
	case bool, float64:
		s = fmt.Sprint(x)
	case []any:
		words := make([]string, 0, len(x))
		for _, item := range x {
			w, ok := item.(string)
			if !ok {
				words = nil
				break
			}
			words = append(words, w)
		}
		if words != nil {
			s = strings.Join(words, ", ")
			break
		}
		data, _ := json.Marshal(x)
		s = string(data)
	case map[string]any:
		if len(x) == 0 {
			return ""
		}
		// An item reads as its own words: a constraint's statement, a
		// pattern's description, an example's corrected text.
		if label := itemLabel(x); label != "" {
			s = label
			break
		}
		data, _ := json.Marshal(x)
		s = string(data)
	default:
		s = fmt.Sprint(x)
	}
	if r := []rune(s); len(r) > voiceValueCap {
		s = string(r[:voiceValueCap-1]) + "…"
	}
	return s
}

// itemLabel is the field an object in a profile list reads as, empty when it
// carries none of them.
func itemLabel(m map[string]any) string {
	for _, key := range []string{"statement", "description", "after", "name", "text"} {
		if v, ok := m[key].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// Phrase names what an edit changed for the record's one line: the top-level
// fields, "tone, constraints". Empty when nothing is listed.
func (edit *VoiceEdit) Phrase() string {
	if edit == nil {
		return ""
	}
	var fields []string
	for _, c := range edit.Changes {
		top, _, _ := strings.Cut(c.Field, ".")
		top, _, _ = strings.Cut(top, "[")
		if !slices.Contains(fields, top) {
			fields = append(fields, top)
		}
	}
	return strings.Join(fields, ", ")
}
