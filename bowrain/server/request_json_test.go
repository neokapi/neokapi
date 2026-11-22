package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Handlers put the text of a bind error into their 400 responses, so the text
// is part of the API. The expected strings are what the server returned for
// the same bodies before this serializer existed.
func TestRequestJSONBindErrorText(t *testing.T) {
	type req struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}
	for _, tc := range []struct {
		body    string
		wantErr string
		want    req
	}{
		{
			body:    `x`,
			wantErr: "code=400, message=Syntax error: offset=1, error=invalid character 'x' looking for beginning of value, internal=invalid character 'x' looking for beginning of value",
		},
		{
			body:    `{"name": 5}`,
			wantErr: "code=400, message=Unmarshal type error: expected=string, got=number, field=name, offset=10, internal=json: cannot unmarshal number into Go struct field req.name of type string",
		},
		{
			body:    `{"name": "a"`,
			wantErr: "code=400, message=unexpected EOF, internal=unexpected EOF",
		},
		// The first JSON value is decoded and what follows it is ignored.
		{body: `{"name": "a"} trailing`, want: req{Name: "a"}},
		{body: `{"name": "a"}{"name": "b"}`, want: req{Name: "a"}},
		{body: ``},
	} {
		t.Run(tc.body, func(t *testing.T) {
			e := echo.New()
			e.JSONSerializer = requestJSONSerializer{}
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body))
			r.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			c := e.NewContext(r, httptest.NewRecorder())

			var got req
			err := c.Bind(&got)
			if tc.wantErr == "" {
				require.NoError(t, err)
				assert.Equal(t, tc.want, got)
				return
			}
			require.Error(t, err)
			assert.Equal(t, tc.wantErr, err.Error())
			assert.Equal(t, http.StatusBadRequest, echo.StatusCode(err))
		})
	}
}

// A body over the limit with no Content-Length fails while it is read, and is
// reported as the body limit's 413.
func TestRequestJSONBindOverBodyLimit(t *testing.T) {
	e := echo.New()
	e.JSONSerializer = requestJSONSerializer{}
	var bindErr error
	e.POST("/", func(c *echo.Context) error {
		var v map[string]any
		bindErr = c.Bind(&v)
		return nil
	}, middleware.BodyLimit(10))

	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name": "a long value"}`))
	r.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	r.ContentLength = -1
	e.ServeHTTP(httptest.NewRecorder(), r)

	require.Error(t, bindErr)
	assert.Equal(t, "code=413, message=Request Entity Too Large", bindErr.Error())
	assert.Equal(t, http.StatusRequestEntityTooLarge, echo.StatusCode(bindErr))
}

// A bind error returned unhandled reaches the client through httpErrorHandler
// with its message in "error" and the decoder's text in "details".
func TestRequestJSONBindErrorThroughErrorHandler(t *testing.T) {
	s := &Server{}
	e := echo.New()
	e.JSONSerializer = requestJSONSerializer{}
	e.HTTPErrorHandler = s.httpErrorHandler
	e.POST("/", func(c *echo.Context) error {
		var v map[string]any
		return c.Bind(&v)
	})

	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`x`))
	r.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, r)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.JSONEq(t, `{
		"error": "Syntax error: offset=1, error=invalid character 'x' looking for beginning of value",
		"message": "Syntax error: offset=1, error=invalid character 'x' looking for beginning of value",
		"details": "invalid character 'x' looking for beginning of value"
	}`, rec.Body.String())
}
