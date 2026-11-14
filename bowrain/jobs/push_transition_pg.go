package jobs

import (
	"context"
	"sync"

	"github.com/neokapi/neokapi/bowrain/knowledge"
	"github.com/neokapi/neokapi/bowrain/storage"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	sqlterms "github.com/neokapi/neokapi/bowrain/terms"
	voicepg "github.com/neokapi/neokapi/bowrain/voice"
	"github.com/neokapi/neokapi/terms"
)

// NewPostgresPushTransition runs a push on one transaction of db, with every
// store the push writes bound to it: the content store, the voice store, the
// knowledge store and each workspace's terms store. All of them were built on
// db, which is what lets one transaction cover them.
//
// A workspace's terms store is opened on first use and kept; opening it
// migrates its schema on the pool, once per workspace.
func NewPostgresPushTransition(db *storage.PgDB, content *bstore.PostgresStore, voice *voicepg.PostgresVoiceStore, ks *knowledge.PostgresKnowledgeStore) PushTransitionFunc {
	var opened sync.Map // workspace slug -> *sqlterms.PostgresStore
	openTerms := func(ctx context.Context, slug string) (*sqlterms.PostgresStore, error) {
		if v, ok := opened.Load(slug); ok {
			return v.(*sqlterms.PostgresStore), nil
		}
		tb, err := sqlterms.NewPostgresStoreFromDBContext(ctx, db, slug)
		if err != nil {
			return nil, err
		}
		actual, _ := opened.LoadOrStore(slug, tb)
		return actual.(*sqlterms.PostgresStore), nil
	}
	return func(ctx context.Context, fn func(PushStores) error) error {
		return db.Transition(ctx, func(tx storage.Runner) error {
			st := PushStores{
				Content: content.Bind(tx),
				Voice:   voice.Bind(tx),
				Terms: func(ctx context.Context, slug string) (terms.Store, error) {
					tb, err := openTerms(ctx, slug)
					if err != nil {
						return nil, err
					}
					return tb.Bind(tx), nil
				},
			}
			if ks != nil {
				st.Knowledge = ks.Bind(tx)
			}
			return fn(st)
		})
	}
}
