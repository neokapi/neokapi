package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The e2e suites delete every project in the workspace they use and run only
// against a server whose health answer says it is a disposable test stack.
func TestHandleHealth_ReportsADisposableStack(t *testing.T) {
	for _, disposable := range []bool{false, true} {
		s := &Server{Config: Config{DisposableStack: disposable}}
		e := echo.New()
		rec := httptest.NewRecorder()
		c := e.NewContext(httptest.NewRequest(http.MethodGet, "/api/v1/health", nil), rec)
		require.NoError(t, s.HandleHealth(c))

		var body map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		if disposable {
			assert.Equal(t, true, body["disposable"])
		} else {
			assert.NotContains(t, body, "disposable", "a server not marked disposable says nothing")
		}
	}
}
