package projector

import (
	"context"
	"errors"

	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/terms"
	"github.com/neokapi/neokapi/voice"
)

// ErrReadOnlyView reports a write through a view that only reads.
var ErrReadOnlyView = errors.New("projector: this view of the store only reads; write through the projector")

// A view hands a projection to code written against a subsystem interface
// when the caller only reads: a lookup, a check, an export. Every write method
// answers ErrReadOnlyView, so a callee that writes after all is refused rather
// than writing past the log. Each view answers a nil interface for a build
// whose store lacks the subsystem.

// TermsView is the project's terms store, for reading.
func TermsView(db *projectdb.DB) terms.Store {
	if db == nil || db.Terms() == nil {
		return nil
	}
	return termsView{db.Terms()}
}

// MemoryView is the project's content memory, for reading.
func MemoryView(db *projectdb.DB) memory.Store {
	if db == nil || db.Memory() == nil {
		return nil
	}
	return memoryView{db.Memory()}
}

// VoiceView is the project's voice profiles, for reading.
func VoiceView(db *projectdb.DB) coreprofile.Store {
	if db == nil || db.Voice() == nil {
		return nil
	}
	return voiceView{db.Voice()}
}

type termsView struct{ *terms.SQLiteStore }

func (termsView) AddConcept(context.Context, terms.Concept) error { return ErrReadOnlyView }
func (termsView) AddConceptWithStream(context.Context, terms.Concept, string) error {
	return ErrReadOnlyView
}
func (termsView) DeleteConcept(context.Context, string) error              { return ErrReadOnlyView }
func (termsView) AddRelation(context.Context, terms.ConceptRelation) error { return ErrReadOnlyView }
func (termsView) DeleteRelation(context.Context, string) error             { return ErrReadOnlyView }
func (termsView) AddRelationWithStream(context.Context, terms.ConceptRelation, string) error {
	return ErrReadOnlyView
}

type memoryView struct{ *memory.SQLiteStore }

func (memoryView) Add(context.Context, memory.Entry) error                   { return ErrReadOnlyView }
func (memoryView) AddWithStream(context.Context, memory.Entry, string) error { return ErrReadOnlyView }
func (memoryView) BulkAddWithStream(context.Context, []memory.Entry, string) error {
	return ErrReadOnlyView
}
func (memoryView) Delete(context.Context, string) error { return ErrReadOnlyView }
func (memoryView) CreateImportSession(context.Context, memory.ImportSession) error {
	return ErrReadOnlyView
}
func (memoryView) UpdateImportSessionCount(context.Context, string, int) error {
	return ErrReadOnlyView
}
func (memoryView) DeleteImportSession(context.Context, string) error { return ErrReadOnlyView }

type voiceView struct{ *voice.SQLiteStore }

func (voiceView) CreateProfile(context.Context, *coreprofile.VoiceProfile) error {
	return ErrReadOnlyView
}
func (voiceView) UpdateProfile(context.Context, *coreprofile.VoiceProfile) error {
	return ErrReadOnlyView
}
func (voiceView) DeleteProfile(context.Context, string) error { return ErrReadOnlyView }
