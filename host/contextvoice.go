package host

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/core/projector"
	"github.com/neokapi/neokapi/core/yamledit"
	"github.com/neokapi/neokapi/terms"
)

// Voice profiles, on the way into the store and back out of it.
//
// A recipe binds a profile by the id the store holds it under. A layout keeps
// each profile at a path: `.kapi/voice.yaml` for the project's, and
// `.kapi/profiles/<name>/voice.yaml` for a profile's own. Reading a profile in
// records the tie between the path and the id (MetaVoiceBindings), in the
// context store: a snapshot writes each profile back to the path it came from,
// and a profile that binds no voice in the recipe is answered by the one its
// directory held.

// compileVoiceSource writes one voice profile into the project store's voice
// store and records where it is authored.
//
// The upsert is by id, so reading the same file twice leaves the store as it
// was. An id is what the profile declares, falling back to a slug of its name,
// and finally to the directory it sits in, so two unnamed profiles in different
// profile directories stay two profiles.
//
// Word rules are terms, so the rules the file carries (its `terms:` list, or a
// `vocabulary:` list written before word rules moved to terms, converted) land
// in the project's terms through tb, the same batch's terms writer. It returns
// how many it read that way.
func (a *App) compileVoiceSource(ctx context.Context, db *projectdb.DB, store coreprofile.Store, tb terms.Terminology, src contextSource) (int, error) {
	if store == nil {
		return 0, projectdb.ErrNoStore
	}
	data, err := os.ReadFile(src.path)
	if err != nil {
		return 0, fmt.Errorf("open voice profile: %w", err)
	}
	file, err := coreprofile.ParseVoiceFile(data)
	if err != nil {
		return 0, fmt.Errorf("load voice profile: %w", err)
	}
	prof := file.Profile

	bindings := loadVoiceBindings(ctx, db)
	prof.ID = voiceProfileID(prof, src.rel, bindings)
	prof.Scope = LocalScope
	if err := upsertVoiceProfile(ctx, store, prof); err != nil {
		return 0, err
	}
	bindings[src.rel] = prof.ID
	if err := saveVoiceBindings(ctx, db, bindings); err != nil {
		return 0, err
	}
	if err := a.landWordRules(ctx, tb, file.Terms); err != nil {
		return 0, fmt.Errorf("move the word rules of %s into terms: %w", src.rel, err)
	}
	return len(file.Terms), nil
}

// landWordRules writes word rules into a terms store, in the
// project's source language: a rule that rejects a term records it as
// forbidden, with the form to use as the concept's preferred term, and a rule
// that names only a form to use records that form as preferred. Writing the
// same rules again changes nothing.
func (a *App) landWordRules(ctx context.Context, tb terms.Terminology, rules []coreprofile.TermRule) error {
	if len(rules) == 0 {
		return nil
	}
	if tb == nil {
		return projectdb.ErrNoStore
	}
	concepts, err := tb.Concepts(ctx)
	if err != nil {
		return fmt.Errorf("read the project's terms: %w", err)
	}
	locale := model.LocaleID(a.SourceLocale())
	if locale == "" {
		locale = "en"
	}
	touched := map[int]bool{}
	for _, rule := range rules {
		d := termDecision{
			Text:        rule.Term,
			Locale:      locale,
			Status:      model.TermForbidden,
			Replacement: rule.Replacement,
			Advisory:    rule.Advisory,
			Competitor:  rule.Competitor,
			Forms:       rule.Forms,
		}
		if strings.TrimSpace(rule.Term) == "" {
			d = termDecision{Text: rule.Replacement, Locale: locale, Status: model.TermPreferred, Forms: rule.ReplacementForms}
		}
		if strings.TrimSpace(d.Text) == "" {
			continue
		}
		// A term the store already holds keeps the standing the store gives
		// it: the store is the decided record, and a voice file only adds.
		norm := model.NormalizeLocale(locale)
		if ci := terms.IndexOfTerm(concepts, d.Text, norm); ci >= 0 {
			if ti := terms.TermIndex(&concepts[ci], d.Text, norm); concepts[ci].Terms[ti].Status != d.Status {
				continue
			}
		}
		var target int
		var changed bool
		concepts, target, changed = upsertTerm(concepts, d)
		if changed {
			if rule.Note != "" && concepts[target].Definition == "" {
				concepts[target].Definition = rule.Note
			}
			touched[target] = true
		}
	}
	for i := range concepts {
		if !touched[i] {
			continue
		}
		if err := tb.AddConcept(ctx, concepts[i]); err != nil {
			return fmt.Errorf("write concept %s: %w", concepts[i].ID, err)
		}
	}
	return nil
}

// upsertVoiceProfile creates the profile or updates the one already carrying
// its id, and leaves the store alone when the two already say the same thing.
//
// The skip is what makes reading a profile twice idempotent. Updating a profile
// archives the version it replaces and increments its version number, so an
// update that changed nothing would still move the profile, and every read
// would make the next snapshot differ from the file it came from.
func upsertVoiceProfile(ctx context.Context, store coreprofile.Store, prof *coreprofile.VoiceProfile) error {
	existing, err := store.GetProfile(ctx, prof.ID)
	if err != nil {
		if cerr := store.CreateProfile(ctx, prof); cerr != nil {
			return fmt.Errorf("create voice profile %s: %w", prof.ID, cerr)
		}
		return nil
	}
	same, cmpErr := sameAuthoredVoice(existing, prof)
	if cmpErr != nil {
		return cmpErr
	}
	if same {
		return nil
	}
	if err := store.UpdateProfile(ctx, prof); err != nil {
		return fmt.Errorf("update voice profile %s: %w", prof.ID, err)
	}
	return nil
}

// sameAuthoredVoice reports whether two profiles say the same thing about how
// to write, comparing the serialization a snapshot would produce for each.
func sameAuthoredVoice(a, b *coreprofile.VoiceProfile) (bool, error) {
	left, err := yamledit.Marshal(nil, AuthoredVoiceProfile(a))
	if err != nil {
		return false, fmt.Errorf("compare voice profile: %w", err)
	}
	right, err := yamledit.Marshal(nil, AuthoredVoiceProfile(b))
	if err != nil {
		return false, fmt.Errorf("compare voice profile: %w", err)
	}
	return bytes.Equal(left, right), nil
}

// AuthoredVoiceProfile is a voice profile as a project authors it: what it says
// about how to write, without the bookkeeping the store keeps about the row.
//
// A snapshot and a bundle both carry this projection, because the alternative
// is a generated file whose bytes move whenever the store was written — the
// partition key of the store it came out of, the clock at the moment it was
// last touched, who touched it. None of that is context, and all of it would
// show up in a diff.
func AuthoredVoiceProfile(p *coreprofile.VoiceProfile) *coreprofile.VoiceProfile {
	if p == nil {
		return nil
	}
	out := *p
	out.Scope = ""
	out.CreatedAt = time.Time{}
	out.UpdatedAt = time.Time{}
	out.CreatedBy = ""
	out.VersionNote = ""
	return &out
}

// voiceProfileID settles the identity a profile compiled from rel is stored
// under. A profile that declares an id keeps it. One that does not takes a slug
// of its name, and where that slug already belongs to another file it takes the
// name of the directory the file sits in as well, so two profiles named the
// same way in two profile directories do not collapse into one.
func voiceProfileID(prof *coreprofile.VoiceProfile, rel string, bindings map[string]string) string {
	if prof.ID != "" {
		return prof.ID
	}
	id := slugify(prof.Name)
	if id == "" {
		id = voiceProfileScopeName(rel)
	}
	if claimedBy, taken := voiceIDOwner(bindings, id); taken && claimedBy != rel {
		if scope := voiceProfileScopeName(rel); scope != "" {
			id += "-" + scope
		}
	}
	return id
}

// voiceIDOwner reports the binding an id is already recorded for.
func voiceIDOwner(bindings map[string]string, id string) (string, bool) {
	for rel, got := range bindings {
		if got == id {
			return rel, true
		}
	}
	return "", false
}

// voiceProfileScopeName names the point a profile file belongs to: the profile
// directory holding it, or "default" for the one that sits flat in `.kapi/`.
func voiceProfileScopeName(rel string) string {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	for i, p := range parts {
		if p == project.ProfilesDirName && i+1 < len(parts)-1 {
			return slugify(parts[i+1])
		}
	}
	return "default"
}

// voiceProfileIDForBinding answers which profile in the project's voice store
// was read from a layout path, or "" when none was.
//
// The tie is the binding a read of that file recorded (MetaVoiceBindings). A
// store filled by a restore rather than by a read has no binding recorded, and
// the conventional path answers instead: `.kapi/profiles/<id>/voice.yaml` is
// where a profile of that id is written, which is the same rule
// storedVoiceProfiles writes by.
func (a *App) voiceProfileIDForBinding(ctx context.Context, root, profileFile string) string {
	path := profileFile
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	rel := relSlash(root, path)
	if db, err := a.ProjectDB(ctx, root); err == nil {
		if id := loadVoiceBindings(ctx, db)[rel]; id != "" {
			return id
		}
	}
	return profileIDFromConventionalPath(rel)
}

// profileIDFromConventionalPath reads a profile id out of
// `.kapi/profiles/<id>/voice.yaml`, or returns "" for any other path.
func profileIDFromConventionalPath(rel string) string {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) != 4 || parts[0] != project.StateDirName ||
		parts[1] != project.ProfilesDirName || parts[3] != VoiceConventionalName {
		return ""
	}
	return parts[2]
}

// loadVoiceBindings reads where each voice profile in the store is authored.
// Any uncertainty yields an empty map, which reads as "nothing recorded" and
// costs one re-record.
func loadVoiceBindings(ctx context.Context, db *projectdb.DB) map[string]string {
	v, ok, err := db.ContextMeta(ctx, MetaVoiceBindings)
	if err != nil || !ok {
		return map[string]string{}
	}
	var bindings map[string]string
	if json.Unmarshal([]byte(v), &bindings) != nil || bindings == nil {
		return map[string]string{}
	}
	return bindings
}

// saveVoiceBindings records where each voice profile in the store is authored.
func saveVoiceBindings(ctx context.Context, db *projectdb.DB, bindings map[string]string) error {
	data, err := json.Marshal(bindings)
	if err != nil {
		return fmt.Errorf("encode voice bindings: %w", err)
	}
	if err := db.PutContextMeta(ctx, MetaVoiceBindings, string(data)); err != nil && !errors.Is(err, projectdb.ErrNoStore) {
		return err
	}
	return nil
}

// boundVoiceProfile pairs a profile in the store with the project-relative path
// it is authored at.
type boundVoiceProfile struct {
	// binding is the project-relative slash path the profile is written to.
	binding string
	profile *coreprofile.VoiceProfile
}

// storedVoiceProfiles lists every voice profile the project store holds,
// each paired with the path it is authored at, in binding order.
//
// A profile with no recorded binding takes the conventional place for a profile
// of its id, `.kapi/profiles/<id>/voice.yaml`, which is exactly where
// governance looks for a profile that binds no file of its own. That is what
// lets a store populated by a restore, rather than by a compile, still be
// written back out somewhere a clean clone resolves it from.
func storedVoiceProfiles(ctx context.Context, db *projectdb.DB) ([]boundVoiceProfile, error) {
	store := projector.VoiceView(db)
	if store == nil {
		return nil, nil
	}
	return boundVoiceProfiles(ctx, store, loadVoiceBindings(ctx, db))
}

// boundVoiceProfiles is storedVoiceProfiles over a voice store and the bindings
// recorded for it, for a caller that reaches the store without a project
// handle: a whole-workspace export reads a project's context store out of the
// workspace, where there is no checkout to record bindings in.
func boundVoiceProfiles(ctx context.Context, store coreprofile.Store, bindings map[string]string) ([]boundVoiceProfile, error) {
	if store == nil {
		return nil, nil
	}
	profiles, err := store.ListProfiles(ctx, LocalScope)
	if err != nil {
		return nil, fmt.Errorf("list voice profiles: %w", err)
	}
	byID := make(map[string]string, len(bindings))
	for rel, id := range bindings {
		// A binding that lost its file still names where the profile belongs.
		// Where two bindings claim one id the earlier path wins, so the answer
		// does not depend on map order.
		if prev, ok := byID[id]; !ok || rel < prev {
			byID[id] = rel
		}
	}

	out := make([]boundVoiceProfile, 0, len(profiles))
	for _, p := range profiles {
		binding, ok := byID[p.ID]
		if !ok {
			binding = filepath.ToSlash(project.RelStatePath(
				project.ProfilesDirName, p.ID, VoiceConventionalName))
		}
		out = append(out, boundVoiceProfile{binding: binding, profile: p})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].binding < out[j].binding })
	return out, nil
}
