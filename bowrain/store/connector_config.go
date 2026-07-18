package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/neokapi/neokapi/bowrain/crypto"
	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/neokapi/neokapi/core/id"
)

// ErrConnectorConfigNotFound is returned when no connector config matches the
// requested workspace scope. Handlers map it to HTTP 404; a config belonging to
// another workspace is indistinguishable from a missing one, so one tenant never
// learns another tenant's connector ids.
var ErrConnectorConfigNotFound = errors.New("connector config not found")

// secretConfigKeys lists, per connector type, the config keys whose values are
// credentials: they are sealed at rest with crypto.Cipher and never returned to
// a client. Keep in sync with the connector constructors in bowrain/connector
// (wordpress=password, figma=token, hubspot=api_key). A type absent from the map
// has no secret fields; its whole config is stored (and returned) in plaintext.
var secretConfigKeys = map[string][]string{
	"wordpress": {"password"},
	"figma":     {"token"},
	"hubspot":   {"api_key"},
	"forge":     {"token", "webhook_secret"},
}

// secretKeySet returns the secret config keys for a connector type as a set.
func secretKeySet(connectorType string) map[string]struct{} {
	keys := secretConfigKeys[connectorType]
	set := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		set[k] = struct{}{}
	}
	return set
}

// ConnectorConfig is a workspace-scoped, persisted connector configuration. It
// is what lets a remote connector (WordPress, Figma, HubSpot, …) survive a
// server restart: the ConnectorService only holds live instances in memory, so
// without this row the config — and its credentials — would be lost on reboot.
//
// Config holds the connector's full key/value settings. Its representation
// depends on which method produced the value:
//   - Get / ListAll return the DECRYPTED config (secret values in plaintext),
//     ready to re-instantiate a connector.
//   - List and the value returned by Upsert return the REDACTED config: secret
//     values are blanked to "" (the key is kept so a UI can show the field is
//     set) and never leave the server.
//
// On Upsert an empty value for a secret key means "leave the stored secret
// unchanged" (so a rename or endpoint edit does not wipe the credential),
// mirroring the provider-config and settings-providers precedent.
type ConnectorConfig struct {
	ID          string
	WorkspaceID string
	Type        string
	Name        string
	Config      map[string]string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	// LastSyncAt is the timestamp of the last successful fetch/publish through
	// this connector, recorded server-side (TouchLastSync). Zero when the
	// connector has never synced — the status path reports it as "never synced"
	// instead of the connector's own fabricated wall-clock time.
	LastSyncAt time.Time
}

// ConnectorConfigStore persists connector configurations, sealing secret fields
// at rest. A single store serves both PostgreSQL and SQLite: SQLite-compatible
// `$N` placeholders and TEXT timestamps (RFC3339 UTC) scan identically on both
// drivers, and the config is stored as a TEXT JSON blob (as collections do), so
// no dialect-specific JSON handling is needed. Every method takes a workspace
// scope, so one tenant can never read, mutate, or delete another tenant's rows —
// except ListAll, the boot-time rehydration read that spans all workspaces.
type ConnectorConfigStore struct {
	db     *sql.DB
	cipher *crypto.Cipher // seals secret config values at rest; nil = plaintext (dev/no key)
}

// NewConnectorConfigStore wraps a *sql.DB whose schema already carries the
// connector_configs table (created by the store baseline migrations). A nil
// cipher stores secret values as plaintext (pass-through), matching the
// provider-config behavior.
func NewConnectorConfigStore(db *sql.DB, cipher *crypto.Cipher) *ConnectorConfigStore {
	return &ConnectorConfigStore{db: db, cipher: cipher}
}

// Upsert creates or updates a connector config, keyed by (workspace_id, id).
// Secret config values are sealed; an empty secret value preserves the stored
// one. The returned config carries the REDACTED config (secrets blanked), never
// the sealed or plaintext credential.
func (s *ConnectorConfigStore) Upsert(ctx context.Context, cfg *ConnectorConfig) (ConnectorConfig, error) {
	if cfg.WorkspaceID == "" {
		return ConnectorConfig{}, errors.New("connector config: workspace_id required")
	}
	if cfg.Type == "" {
		return ConnectorConfig{}, errors.New("connector config: type required")
	}
	if cfg.ID == "" {
		cfg.ID = id.New()
	}

	// Load the existing stored config (secret values still sealed) so a blank
	// secret in the new config preserves the previously stored credential. A
	// nil error means the row exists (drives INSERT vs UPDATE below).
	existingSealed, _, rawErr := s.rawConfig(ctx, cfg.WorkspaceID, cfg.ID)
	rowExists := rawErr == nil
	if rawErr != nil && !errors.Is(rawErr, ErrConnectorConfigNotFound) {
		return ConnectorConfig{}, rawErr
	}

	secrets := secretKeySet(cfg.Type)
	toStore := make(map[string]string, len(cfg.Config))
	for k, v := range cfg.Config {
		if _, isSecret := secrets[k]; isSecret {
			if v == "" {
				// Blank secret: keep the previously stored (sealed) value, if any.
				if prev, ok := existingSealed[k]; ok && prev != "" {
					toStore[k] = prev
				}
				continue
			}
			sealed, sErr := s.cipher.Seal(v)
			if sErr != nil {
				return ConnectorConfig{}, fmt.Errorf("seal connector secret %q: %w", k, sErr)
			}
			toStore[k] = sealed
			continue
		}
		toStore[k] = v
	}
	// Preserve any stored secret whose key was omitted entirely from the new
	// config (an omitted secret is treated the same as a blank one).
	for k, prev := range existingSealed {
		if _, isSecret := secrets[k]; !isSecret || prev == "" {
			continue
		}
		if _, present := toStore[k]; !present {
			toStore[k] = prev
		}
	}

	configJSON, err := json.Marshal(toStore)
	if err != nil {
		return ConnectorConfig{}, fmt.Errorf("marshal connector config: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	// UPSERT via explicit INSERT-vs-UPDATE keeps the SQL dialect-neutral (no
	// ON CONFLICT / RETURNING differences between PostgreSQL and SQLite).
	if rowExists {
		if _, err := s.db.ExecContext(ctx,
			`UPDATE connector_configs SET type=$1, name=$2, config=$3, updated_at=$4
			 WHERE workspace_id=$5 AND id=$6`,
			cfg.Type, cfg.Name, string(configJSON), now, cfg.WorkspaceID, cfg.ID); err != nil {
			return ConnectorConfig{}, fmt.Errorf("update connector config: %w", err)
		}
	} else {
		if _, err := s.db.ExecContext(ctx,
			`INSERT INTO connector_configs (id, workspace_id, type, name, config, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $6)`,
			cfg.ID, cfg.WorkspaceID, cfg.Type, cfg.Name, string(configJSON), now); err != nil {
			return ConnectorConfig{}, fmt.Errorf("insert connector config: %w", err)
		}
	}

	return s.Get(ctx, cfg.WorkspaceID, cfg.ID, WithRedactedSecrets())
}

// getOptions controls how a read projects secret config values.
type getOptions struct{ redact bool }

// GetOption configures Get.
type GetOption func(*getOptions)

// WithRedactedSecrets makes Get blank secret values instead of decrypting them,
// yielding a config safe to return to a client.
func WithRedactedSecrets() GetOption { return func(o *getOptions) { o.redact = true } }

// Get returns one connector config, scoped to the workspace. By default secret
// values are decrypted (ready to re-instantiate a connector); pass
// WithRedactedSecrets to blank them instead. Returns ErrConnectorConfigNotFound
// when the id does not belong to this workspace.
func (s *ConnectorConfigStore) Get(ctx context.Context, workspaceID, configID string, opts ...GetOption) (ConnectorConfig, error) {
	var o getOptions
	for _, fn := range opts {
		fn(&o)
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT id, workspace_id, type, name, config, created_at, updated_at, last_sync_at
		 FROM connector_configs WHERE workspace_id=$1 AND id=$2`, workspaceID, configID)
	return s.scan(row, o.redact)
}

// List returns all connector configs for a workspace with secret values
// REDACTED — the read that backs the connectors listing. Never returns secrets.
func (s *ConnectorConfigStore) List(ctx context.Context, workspaceID string) ([]ConnectorConfig, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, workspace_id, type, name, config, created_at, updated_at, last_sync_at
		 FROM connector_configs WHERE workspace_id=$1 ORDER BY name, id`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list connector configs: %w", err)
	}
	return s.scanRows(rows, true)
}

// ListAll returns every persisted connector config across all workspaces with
// secret values DECRYPTED. It exists solely for boot-time rehydration, where the
// server re-instantiates each connector; it is never exposed to a client.
func (s *ConnectorConfigStore) ListAll(ctx context.Context) ([]ConnectorConfig, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, workspace_id, type, name, config, created_at, updated_at, last_sync_at
		 FROM connector_configs ORDER BY workspace_id, id`)
	if err != nil {
		return nil, fmt.Errorf("list all connector configs: %w", err)
	}
	return s.scanRows(rows, false)
}

// TouchLastSync stamps a connector's last successful sync time, scoped to the
// workspace. Called after a successful fetch/publish so the status path can
// report a real timestamp instead of the connector's fabricated wall-clock time.
// A no-op (nil) when nothing matched — a missing row is not worth failing a sync.
func (s *ConnectorConfigStore) TouchLastSync(ctx context.Context, workspaceID, configID string, t time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE connector_configs SET last_sync_at=$1 WHERE workspace_id=$2 AND id=$3`,
		t.UTC().Format(time.RFC3339), workspaceID, configID)
	if err != nil {
		return fmt.Errorf("touch connector last_sync_at: %w", err)
	}
	return nil
}

// Delete removes a connector config scoped to the workspace. Returns
// ErrConnectorConfigNotFound when nothing matched (missing or cross-tenant).
func (s *ConnectorConfigStore) Delete(ctx context.Context, workspaceID, configID string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM connector_configs WHERE workspace_id=$1 AND id=$2`, workspaceID, configID)
	if err != nil {
		return fmt.Errorf("delete connector config: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrConnectorConfigNotFound
	}
	return nil
}

// rawConfig loads the stored config map with secret values left AS STORED
// (sealed). Used by Upsert to preserve blank secrets without decrypting them.
func (s *ConnectorConfigStore) rawConfig(ctx context.Context, workspaceID, configID string) (map[string]string, string, error) {
	var configJSON, connType string
	err := s.db.QueryRowContext(ctx,
		`SELECT config, type FROM connector_configs WHERE workspace_id=$1 AND id=$2`,
		workspaceID, configID).Scan(&configJSON, &connType)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", ErrConnectorConfigNotFound
	}
	if err != nil {
		return nil, "", fmt.Errorf("load connector config: %w", err)
	}
	m := map[string]string{}
	if err := json.Unmarshal([]byte(configJSON), &m); err != nil {
		return nil, "", fmt.Errorf("unmarshal connector config: %w", err)
	}
	return m, connType, nil
}

// scanRows scans a result set of connector-config rows.
func (s *ConnectorConfigStore) scanRows(rows *sql.Rows, redact bool) ([]ConnectorConfig, error) {
	defer rows.Close()
	out := make([]ConnectorConfig, 0)
	for rows.Next() {
		cfg, err := s.scan(rows, redact)
		if err != nil {
			return nil, err
		}
		out = append(out, cfg)
	}
	return out, rows.Err()
}

// scan reads one row and projects its config either redacted (secrets blanked)
// or decrypted (secrets in plaintext), per redact.
func (s *ConnectorConfigStore) scan(sc storage.Scanner, redact bool) (ConnectorConfig, error) {
	var cfg ConnectorConfig
	var configJSON, createdStr, updatedStr, lastSyncStr string
	err := sc.Scan(&cfg.ID, &cfg.WorkspaceID, &cfg.Type, &cfg.Name, &configJSON, &createdStr, &updatedStr, &lastSyncStr)
	if errors.Is(err, sql.ErrNoRows) {
		return ConnectorConfig{}, ErrConnectorConfigNotFound
	}
	if err != nil {
		return ConnectorConfig{}, fmt.Errorf("scan connector config: %w", err)
	}
	cfg.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	cfg.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
	// last_sync_at is "" until the first successful sync; leave the zero value.
	if lastSyncStr != "" {
		cfg.LastSyncAt, _ = time.Parse(time.RFC3339, lastSyncStr)
	}

	stored := map[string]string{}
	if err := json.Unmarshal([]byte(configJSON), &stored); err != nil {
		return ConnectorConfig{}, fmt.Errorf("unmarshal connector config: %w", err)
	}
	secrets := secretKeySet(cfg.Type)
	cfg.Config = make(map[string]string, len(stored))
	for k, v := range stored {
		if _, isSecret := secrets[k]; isSecret {
			if redact {
				cfg.Config[k] = "" // key kept, value hidden
				continue
			}
			opened, oErr := s.cipher.Open(v)
			if oErr != nil {
				return ConnectorConfig{}, fmt.Errorf("open connector secret %q: %w", k, oErr)
			}
			cfg.Config[k] = opened
			continue
		}
		cfg.Config[k] = v
	}
	return cfg, nil
}
