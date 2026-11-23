package jobs

import (
	"context"

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
// voice and ks may be nil when their stores could not be opened; a push that
// needs one then fails rather than writing outside the transaction.
//
// The terms schema, which every workspace shares, is migrated here, before
// any push opens a transaction. A push then opens a workspace's terms store
// without a statement of its own, so nothing it does inside its transaction
// reaches the pool.
func NewPostgresPushTransition(ctx context.Context, db *storage.PgDB, content *bstore.PostgresStore, voice *voicepg.PostgresVoiceStore, ks *knowledge.PostgresKnowledgeStore) (PushTransitionFunc, error) {
	if err := sqlterms.MigratePostgres(ctx, db); err != nil {
		return nil, err
	}
	return func(ctx context.Context, fn func(PushStores) error) error {
		return db.Transition(ctx, func(tx storage.Runner) error {
			st := PushStores{
				Content: content.Bind(tx),
				Terms: func(_ context.Context, slug string) (terms.Store, error) {
					return sqlterms.ForWorkspace(db, slug).Bind(tx), nil
				},
			}
			if voice != nil {
				st.Voice = voice.Bind(tx)
			}
			if ks != nil {
				st.Knowledge = ks.Bind(tx)
			}
			return fn(st)
		})
	}, nil
}
