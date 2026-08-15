package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchUser(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/api/v1/auth/me", r.URL.Path)
			assert.Equal(t, http.MethodGet, r.Method)
			assert.Equal(t, "Bearer tok_123", r.Header.Get("Authorization"))

			_ = json.NewEncoder(w).Encode(map[string]string{
				"id":    "usr_1",
				"email": "alice@example.com",
				"name":  "Alice",
			})
		}))
		defer srv.Close()

		user, err := FetchUser(context.Background(), srv.URL+"/", "tok_123")
		require.NoError(t, err)
		assert.Equal(t, "usr_1", user.ID)
		assert.Equal(t, "alice@example.com", user.Email)
		assert.Equal(t, "Alice", user.Name)
	})

	t.Run("unauthorized", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "invalid token", http.StatusUnauthorized)
		}))
		defer srv.Close()

		user, err := FetchUser(context.Background(), srv.URL, "bad")
		require.Error(t, err)
		assert.Nil(t, user)
		assert.Contains(t, err.Error(), "HTTP 401")
	})

	t.Run("cancelled context", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "usr_1"})
		}))
		defer srv.Close()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		user, err := FetchUser(ctx, srv.URL, "tok")
		require.Error(t, err)
		assert.Nil(t, user)
	})
}
