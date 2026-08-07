package server

import (
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func pageCtx(query string) echo.Context {
	e := echo.New()
	r := httptest.NewRequest(http.MethodGet, "/?"+query, nil)
	return e.NewContext(r, httptest.NewRecorder())
}

// TestPageParamsClamps pins the finding-8 contract: a client asking for
// ?limit=1000000 gets a bounded page rather than an unbounded scan, and the
// default/offset handling stays intact.
func TestPageParamsClamps(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		query                 string
		def, max              int
		wantLimit, wantOffset int
	}{
		{"default when absent", "", 50, 1000, 50, 0},
		{"clamps a huge limit to max", "limit=1000000", 50, 1000, 1000, 0},
		{"honours a valid limit", "limit=200", 50, 1000, 200, 0},
		{"non-positive falls back to default", "limit=0", 50, 1000, 50, 0},
		{"negative falls back to default", "limit=-5", 50, 1000, 50, 0},
		{"offset passes through", "limit=10&offset=30", 50, 1000, 10, 30},
		{"def 0 stays 0 when absent (store default)", "", 0, 1000, 0, 0},
		{"def 0 still clamps a huge limit", "limit=1000000", 0, 1000, 1000, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			limit, offset := pageParams(pageCtx(tc.query), tc.def, tc.max)
			assert.Equal(t, tc.wantLimit, limit, "limit")
			assert.Equal(t, tc.wantOffset, offset, "offset")
		})
	}
}

// TestReadMultipartUploadsCaps pins the finding-9 contract: the multipart
// upload reader refuses too many files, an over-large total, and an over-large
// single file before buffering unbounded content.
func TestReadMultipartUploadsCaps(t *testing.T) {
	newCtx := func() (echo.Context, *httptest.ResponseRecorder) {
		e := echo.New()
		r := httptest.NewRequest(http.MethodPost, "/", nil)
		rec := httptest.NewRecorder()
		return e.NewContext(r, rec), rec
	}
	form := func(fhs []*multipart.FileHeader) *multipart.Form {
		return &multipart.Form{File: map[string][]*multipart.FileHeader{"files": fhs}}
	}

	t.Run("no files", func(t *testing.T) {
		c, rec := newCtx()
		_, answered, _ := readMultipartUploads(c, form(nil))
		assert.True(t, answered)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("too many files", func(t *testing.T) {
		fhs := make([]*multipart.FileHeader, maxUploadFiles+1)
		for i := range fhs {
			fhs[i] = &multipart.FileHeader{Filename: "f", Size: 1}
		}
		c, rec := newCtx()
		_, answered, _ := readMultipartUploads(c, form(fhs))
		assert.True(t, answered)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("total too large", func(t *testing.T) {
		c, rec := newCtx()
		_, answered, _ := readMultipartUploads(c, form([]*multipart.FileHeader{
			{Filename: "a", Size: maxUploadTotalBytes},
			{Filename: "b", Size: 1},
		}))
		assert.True(t, answered)
		assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	})

	t.Run("single file too large", func(t *testing.T) {
		c, rec := newCtx()
		_, answered, _ := readMultipartUploads(c, form([]*multipart.FileHeader{
			{Filename: "big", Size: maxUploadFileBytes + 1},
		}))
		assert.True(t, answered)
		assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	})
}
