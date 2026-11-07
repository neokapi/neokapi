package host

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/contextop"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/yamledit"
)

// What a whole-profile write changed, kept on the edit operation that records
// it (contextop.VoiceEdit): the fields that differ, the profile as it stood,
// and a fingerprint of what the write left, so dropping the edit can put the
// profile back.

// voiceChangeCap bounds the changes one record keeps, and voiceValueCap the
// length of one value as a change shows it. The record keeps the previous
// profile whole, so nothing a restore needs is lost to either cap.
const (
	voiceChangeCap = 40
	voiceValueCap  = 240
)

// voiceEditOf compares the profile as it stood with the profile the store now
// holds. before is nil when the write created the profile.
func voiceEditOf(before, after *coreprofile.VoiceProfile) (*contextop.VoiceEdit, error) {
	if after == nil {
		return nil, errors.New("voice: no profile to compare")
	}
	edit := &contextop.VoiceEdit{Profile: after.ID}
	wrote, err := voiceFingerprint(after)
	if err != nil {
		return nil, err
	}
	edit.Wrote = wrote
	left, err := voiceTree(before)
	if err != nil {
		return nil, err
	}
	right, err := voiceTree(after)
	if err != nil {
		return nil, err
	}
	var changes []contextop.VoiceChange
	diffVoiceTree("", left, right, &changes)
	if len(changes) > voiceChangeCap {
		edit.Omitted = len(changes) - voiceChangeCap
		changes = changes[:voiceChangeCap]
	}
	edit.Changes = changes
	if before != nil {
		prev, err := yamledit.Marshal(nil, AuthoredVoiceProfile(before))
		if err != nil {
			return nil, fmt.Errorf("render voice profile %s: %w", before.ID, err)
		}
		edit.Previous = string(prev)
	}
	return edit, nil
}

// voiceContent is a profile as its content: what it authors, without the
// version the store increments on every write.
func voiceContent(p *coreprofile.VoiceProfile) *coreprofile.VoiceProfile {
	out := AuthoredVoiceProfile(p)
	if out != nil {
		out.Version = 0
	}
	return out
}

// voiceFingerprint identifies a profile's content.
func voiceFingerprint(p *coreprofile.VoiceProfile) (string, error) {
	doc, err := yamledit.Marshal(nil, voiceContent(p))
	if err != nil {
		return "", fmt.Errorf("fingerprint voice profile %s: %w", p.ID, err)
	}
	sum := sha256.Sum256(doc)
	return hex.EncodeToString(sum[:]), nil
}

// voiceTree is a profile's content as a generic tree, for comparing field by
// field. A nil profile is an empty tree.
func voiceTree(p *coreprofile.VoiceProfile) (map[string]any, error) {
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
func diffVoiceTree(path string, a, b any, out *[]contextop.VoiceChange) {
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
	from, to := voiceValue(a), voiceValue(b)
	if from == to {
		return
	}
	*out = append(*out, contextop.VoiceChange{Field: path, From: from, To: to})
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
		// A constraint reads as its statement.
		if st, ok := x["statement"].(string); ok && st != "" {
			s = st
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

// voiceEditPhrase names what an edit changed for the record's one line: the
// top-level fields, "tone, constraints". Empty when nothing is listed.
func voiceEditPhrase(edit *contextop.VoiceEdit) string {
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

// restoreVoiceEdit puts back the profile an edit replaced, for dropping the
// edit. It refuses when the profile has been written since, because restoring
// then would undo that later write too, and reports where it restored to.
func (s *contextOpsSession) restoreVoiceEdit(ctx context.Context, r contextop.Record, apply bool) (string, error) {
	edit := r.Voice
	if edit == nil || edit.Previous == "" {
		return "", nil
	}
	w, err := s.writer(ctx, r)
	if err != nil {
		return "", err
	}
	store := voiceWriter(w)
	if store == nil {
		return "", errors.New("voice: this project has no voice store")
	}
	current, err := store.GetProfile(ctx, edit.Profile)
	if err != nil {
		return "", fmt.Errorf("voice profile %s is no longer in the project's store, so dropping this edit cannot restore it", edit.Profile)
	}
	now, err := voiceFingerprint(current)
	if err != nil {
		return "", err
	}
	if now != edit.Wrote {
		return "", fmt.Errorf("voice profile %s was written after operation %s, so dropping it would undo that write too. Drop the later edit first", edit.Profile, contextop.ShortID(r.ID))
	}
	if !apply {
		return "", nil
	}
	prev, err := coreprofile.LoadProfileYAML(strings.NewReader(edit.Previous))
	if err != nil {
		return "", fmt.Errorf("read the earlier voice profile %s: %w", edit.Profile, err)
	}
	prev.ID, prev.Scope, prev.Version = current.ID, current.Scope, current.Version
	if prev.Constraints == nil {
		prev.Constraints = []coreprofile.Constraint{}
	}
	if err := store.UpdateProfile(ctx, prev); err != nil {
		return "", fmt.Errorf("restore voice profile %s: %w", edit.Profile, err)
	}
	s.changed = true
	return "voice profile " + edit.Profile + " restored to the version before", nil
}
