package server

import (
	"testing"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/stretchr/testify/assert"
)

func TestIsSecretConnectorKey(t *testing.T) {
	secret := []string{
		"password", "Password", "passwd", "api_key", "apiKey", "API-KEY",
		"access_token", "refresh_token", "auth_token", "client_secret",
		"clientSecret", "private_key", "access_key", "personal_access_token",
		"credential", "credentials",
	}
	for _, k := range secret {
		assert.Truef(t, isSecretConnectorKey(k), "expected %q to be secret", k)
	}
	notSecret := []string{
		"url", "base_url", "username", "user", "host", "path", "branch",
		"repo", "project", "site_id", "format", "pattern",
	}
	for _, k := range notSecret {
		assert.Falsef(t, isSecretConnectorKey(k), "expected %q NOT to be secret", k)
	}
}

func TestRedactConnectorConfig(t *testing.T) {
	cfg := map[string]string{
		"base_url": "https://wp.example.com",
		"username": "editor",
		"password": "hunter2",
		"api_token": "tok_live_abc123",
	}
	redacted, keys := redactConnectorConfig(cfg)

	// Non-secret values survive.
	assert.Equal(t, "https://wp.example.com", redacted["base_url"])
	assert.Equal(t, "editor", redacted["username"])
	// Secret values are gone from the returned config.
	_, hasPw := redacted["password"]
	_, hasTok := redacted["api_token"]
	assert.False(t, hasPw, "password must be redacted")
	assert.False(t, hasTok, "api_token must be redacted")
	// The redacted key names are reported, sorted.
	assert.Equal(t, []string{"api_token", "password"}, keys)
	// The original map is not mutated.
	assert.Equal(t, "hunter2", cfg["password"])

	// Empty/nil config yields no secret keys.
	r, k := redactConnectorConfig(nil)
	assert.Nil(t, r)
	assert.Nil(t, k)
}

func TestCollectionToResponseRedactsSecrets(t *testing.T) {
	coll := &store.Collection{
		ID:        "col_1",
		ProjectID: "proj_1",
		Name:      "WordPress",
		Kind:      store.CollectionKind("connected"),
		ConnectorConfig: map[string]string{
			"base_url": "https://wp.example.com",
			"password": "hunter2",
		},
	}
	resp := collectionToResponse(coll)
	assert.Equal(t, "https://wp.example.com", resp.ConnectorConfig["base_url"])
	_, leaked := resp.ConnectorConfig["password"]
	assert.False(t, leaked, "collection response must not echo the connector password")
	assert.Equal(t, []string{"password"}, resp.ConnectorSecretKeys)
}

func TestMergeConnectorConfigPreservesSecrets(t *testing.T) {
	existing := map[string]string{
		"base_url": "https://old.example.com",
		"username": "editor",
		"password": "stored-secret",
		"api_token": "stored-token",
	}

	// Client PUTs back a redacted config (secrets stripped): secrets are kept,
	// non-secret values follow the request.
	incoming := map[string]string{
		"base_url": "https://new.example.com",
		"username": "editor",
	}
	merged := mergeConnectorConfig(existing, incoming)
	assert.Equal(t, "https://new.example.com", merged["base_url"], "non-secret follows request")
	assert.Equal(t, "stored-secret", merged["password"], "omitted secret is carried forward")
	assert.Equal(t, "stored-token", merged["api_token"], "omitted secret is carried forward")

	// Client rotates a secret by sending a new value: it overwrites.
	rotated := mergeConnectorConfig(existing, map[string]string{"password": "new-secret"})
	assert.Equal(t, "new-secret", rotated["password"])

	// nil incoming leaves the stored config untouched.
	assert.Equal(t, existing, mergeConnectorConfig(existing, nil))
}
