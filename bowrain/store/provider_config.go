package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/neokapi/neokapi/bowrain/crypto"
	"github.com/neokapi/neokapi/core/id"
)

// ErrProviderConfigNotFound is returned when no provider config matches the
// requested workspace scope. Handlers map it to HTTP 404 and the worker to a
// resolve error; either way one tenant never learns another tenant's ids.
var ErrProviderConfigNotFound = errors.New("provider config not found")

// ProviderConfig is a workspace-scoped AI provider configuration persisted in
// Postgres (Epic 004 hybrid AI). Unlike the machine-global, keychain-backed
// core/credentials store, these configs are multi-tenant: every row is owned by
// exactly one workspace and the API key is sealed at rest with crypto.Cipher
// (BOWRAIN_SECRETS_KEY) — the same mechanism used for connector secrets.
//
// APIKey holds the decrypted key and is populated only by Get and Resolve —
// never by List, and it is never serialized back to a client. On Upsert an empty
// APIKey means "leave the stored key unchanged" (so a rename/model edit does not
// wipe the key).
type ProviderConfig struct {
	ID            string
	WorkspaceID   string
	WorkspaceSlug string
	Name          string
	Type          string // aiprovider ID: "anthropic", "openai", "gemini", "ollama", ...
	Model         string
	BaseURL       string
	APIKey        string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// ProviderConfigStore is a workspace-scoped CRUD store for AI provider configs.
// Every method takes a workspace scope, so one tenant can never list, read,
// resolve, or mutate another tenant's configuration.
type ProviderConfigStore struct {
	db     *sql.DB
	cipher *crypto.Cipher // seals api_key at rest; nil = plaintext (dev/no key)
}

// NewProviderConfigStore wraps a *sql.DB whose schema already carries the
// provider_configs table (created by the store baseline migrations). The cipher
// seals and opens the API key; a nil cipher stores keys as plaintext
// (pass-through), matching the connector-secret behavior.
func NewProviderConfigStore(db *sql.DB, cipher *crypto.Cipher) *ProviderConfigStore {
	return &ProviderConfigStore{db: db, cipher: cipher}
}

// Upsert creates or updates a provider config, keyed by (workspace_id, name).
// The returned config never carries the API key. When cfg.APIKey is non-empty
// it is sealed and stored; when empty, any existing sealed key is preserved.
func (s *ProviderConfigStore) Upsert(ctx context.Context, cfg *ProviderConfig) (ProviderConfig, error) {
	if cfg.WorkspaceID == "" {
		return ProviderConfig{}, errors.New("provider config: workspace_id required")
	}
	if cfg.Name == "" {
		return ProviderConfig{}, errors.New("provider config: name required")
	}
	if cfg.ID == "" {
		cfg.ID = id.New()
	}

	// Seal the key only when a new one is supplied. A typed nil []byte encodes as
	// SQL NULL, so COALESCE below keeps the previously stored key on update.
	var sealedArg []byte
	if cfg.APIKey != "" {
		sealed, err := s.cipher.Seal(cfg.APIKey)
		if err != nil {
			return ProviderConfig{}, fmt.Errorf("seal api key: %w", err)
		}
		sealedArg = []byte(sealed)
	}

	now := time.Now().UTC()
	var out ProviderConfig
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO provider_configs
			(id, workspace_id, workspace_slug, name, type, model, base_url, api_key_sealed, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9)
		 ON CONFLICT (workspace_id, name) DO UPDATE SET
			workspace_slug = EXCLUDED.workspace_slug,
			type           = EXCLUDED.type,
			model          = EXCLUDED.model,
			base_url       = EXCLUDED.base_url,
			api_key_sealed = COALESCE(EXCLUDED.api_key_sealed, provider_configs.api_key_sealed),
			updated_at     = EXCLUDED.updated_at
		 RETURNING id, workspace_id, workspace_slug, name, type, model, base_url, created_at, updated_at`,
		cfg.ID, cfg.WorkspaceID, cfg.WorkspaceSlug, cfg.Name, cfg.Type, cfg.Model, cfg.BaseURL, sealedArg, now,
	).Scan(&out.ID, &out.WorkspaceID, &out.WorkspaceSlug, &out.Name, &out.Type, &out.Model, &out.BaseURL, &out.CreatedAt, &out.UpdatedAt)
	if err != nil {
		return ProviderConfig{}, fmt.Errorf("upsert provider config: %w", err)
	}
	return out, nil
}

// List returns all provider configs for a workspace, without API keys.
func (s *ProviderConfigStore) List(ctx context.Context, workspaceID string) ([]ProviderConfig, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, workspace_id, workspace_slug, name, type, model, base_url, created_at, updated_at
		 FROM provider_configs WHERE workspace_id=$1 ORDER BY name`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list provider configs: %w", err)
	}
	defer rows.Close()

	out := make([]ProviderConfig, 0)
	for rows.Next() {
		var c ProviderConfig
		if err := rows.Scan(&c.ID, &c.WorkspaceID, &c.WorkspaceSlug, &c.Name,
			&c.Type, &c.Model, &c.BaseURL, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan provider config: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Get returns one provider config, scoped to the workspace, with its API key
// decrypted. Returns ErrProviderConfigNotFound when the id does not belong to
// this workspace (cross-tenant reads are indistinguishable from "not found").
func (s *ProviderConfigStore) Get(ctx context.Context, workspaceID, configID string) (ProviderConfig, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, workspace_id, workspace_slug, name, type, model, base_url, api_key_sealed, created_at, updated_at
		 FROM provider_configs WHERE workspace_id=$1 AND id=$2`, workspaceID, configID)
	return s.scanWithKey(row)
}

// Resolve loads a saved config for the worker's translation path, with the API
// key decrypted. It is scoped strictly by the DURABLE workspace_id — never by
// workspace_slug. The persisted slug is a mutable, reusable label: a workspace
// rename would break resolution, and worse, a slug freed by one tenant and later
// acquired by another could authorize retrieval of the first tenant's decrypted
// key. Since the translation job now persists the billing workspace_id, that id
// is the only resolution key here. An empty workspace_id never resolves (guards a
// blank/anon job from leaking a config).
func (s *ProviderConfigStore) Resolve(ctx context.Context, workspaceID, configID string) (ProviderConfig, error) {
	if workspaceID == "" {
		return ProviderConfig{}, ErrProviderConfigNotFound
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT id, workspace_id, workspace_slug, name, type, model, base_url, api_key_sealed, created_at, updated_at
		 FROM provider_configs
		 WHERE id=$1 AND workspace_id=$2`,
		configID, workspaceID)
	return s.scanWithKey(row)
}

// Delete removes a provider config scoped to the workspace. Returns
// ErrProviderConfigNotFound when nothing matched (missing or cross-tenant).
func (s *ProviderConfigStore) Delete(ctx context.Context, workspaceID, configID string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM provider_configs WHERE workspace_id=$1 AND id=$2`, workspaceID, configID)
	if err != nil {
		return fmt.Errorf("delete provider config: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrProviderConfigNotFound
	}
	return nil
}

// scanWithKey scans a single row and unseals the API key.
func (s *ProviderConfigStore) scanWithKey(row *sql.Row) (ProviderConfig, error) {
	var c ProviderConfig
	var sealed []byte
	err := row.Scan(&c.ID, &c.WorkspaceID, &c.WorkspaceSlug, &c.Name,
		&c.Type, &c.Model, &c.BaseURL, &sealed, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ProviderConfig{}, ErrProviderConfigNotFound
	}
	if err != nil {
		return ProviderConfig{}, fmt.Errorf("get provider config: %w", err)
	}
	if len(sealed) > 0 {
		key, err := s.cipher.Open(string(sealed))
		if err != nil {
			return ProviderConfig{}, fmt.Errorf("open api key: %w", err)
		}
		c.APIKey = key
	}
	return c, nil
}
