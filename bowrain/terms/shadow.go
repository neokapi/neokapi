package terms

import (
	"context"

	fw "github.com/neokapi/neokapi/terms"
)

// DeleteShadowConcepts clears a namespace within one workspace's reserved shadow IDs.
// Foreign keys remove the copies' terms and relations.
func (tb *PostgresStore) DeleteShadowConcepts(ctx context.Context, namespace string) error {
	prefix, err := fw.ShadowNamespacePrefix(namespace)
	if err != nil {
		return err
	}
	tx, err := tb.begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, "DELETE FROM tb_concepts WHERE workspace_id = $1 AND starts_with(id, $2)", tb.workspaceID, prefix)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count > 0 {
		if err := tb.bumpRevision(ctx, tx); err != nil {
			return err
		}
	}
	return tx.Commit()
}
