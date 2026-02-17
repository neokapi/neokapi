package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAuthFilePathDefault(t *testing.T) {
	// Ensure KAPI_CONFIG_DIR is not set.
	t.Setenv("KAPI_CONFIG_DIR", "")

	path := authFilePath()
	assert.Contains(t, path, "auth.json")
	assert.Contains(t, path, "kapi")
}

func TestAuthFilePathWithConfigDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KAPI_CONFIG_DIR", dir)

	path := authFilePath()
	assert.Equal(t, filepath.Join(dir, "auth.json"), path)
}

func TestSaveAndLoadAuth(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KAPI_CONFIG_DIR", dir)

	original := StoredAuth{
		ServerURL:   "http://localhost:8080",
		AccessToken: "test-token-123",
		User: StoredUser{
			ID:    "user-1",
			Email: "test@example.com",
			Name:  "Test User",
		},
	}

	err := saveAuth(original)
	assert.NoError(t, err)

	// Verify file was created in the custom directory.
	_, err = os.Stat(filepath.Join(dir, "auth.json"))
	assert.NoError(t, err)

	loaded, err := loadAuth()
	assert.NoError(t, err)
	assert.Equal(t, original.ServerURL, loaded.ServerURL)
	assert.Equal(t, original.AccessToken, loaded.AccessToken)
	assert.Equal(t, original.User.Email, loaded.User.Email)
}
