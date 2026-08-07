package server

import (
	"fmt"
	"io"
	"mime/multipart"
	"net/http"

	"github.com/labstack/echo/v4"
)

// Upload caps for the editor/collection multipart handlers. Without them a
// request buffers every part into memory with io.ReadAll before any format
// reader runs, bounded only by the global 50 MiB BodyLimit — so one large or
// many small parts pin memory. These mirror the brand-scan upload contract:
// a count cap, a per-file cap, a total cap, and a LimitReader on each read.
const (
	maxUploadFiles      = 200
	maxUploadFileBytes  = 20 * 1024 * 1024 // per file
	maxUploadTotalBytes = 50 * 1024 * 1024 // per request
)

// readMultipartUploads reads the multipart "files" field into memory under the
// count, per-file and total-size caps above. It returns (files, true, err) when
// it has already answered the request (empty, too many, or too large), matching
// the (refused, err) shape the other request guards use.
func readMultipartUploads(c echo.Context, form *multipart.Form) (files map[string][]byte, answered bool, err error) {
	fhs := form.File["files"]
	if len(fhs) == 0 {
		return nil, true, c.JSON(http.StatusBadRequest, ErrorResponse{Error: "no files uploaded"})
	}
	if len(fhs) > maxUploadFiles {
		return nil, true, c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: fmt.Sprintf("too many files: at most %d per upload", maxUploadFiles),
		})
	}
	var total int64
	for _, fh := range fhs {
		total += fh.Size
	}
	if total > maxUploadTotalBytes {
		return nil, true, c.JSON(http.StatusRequestEntityTooLarge, ErrorResponse{
			Error: "upload too large: at most 50 MiB per request",
		})
	}

	files = make(map[string][]byte, len(fhs))
	for _, fh := range fhs {
		if fh.Size > maxUploadFileBytes {
			return nil, true, c.JSON(http.StatusRequestEntityTooLarge, ErrorResponse{
				Error: fmt.Sprintf("%q too large: at most 20 MiB per file", fh.Filename),
			})
		}
		f, err := fh.Open()
		if err != nil {
			return nil, true, c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: fmt.Sprintf("open %q: %s", fh.Filename, err),
			})
		}
		// LimitReader (+1) so a part whose declared Size lied still cannot exceed
		// the cap; the length check below catches the overflow.
		data, readErr := io.ReadAll(io.LimitReader(f, maxUploadFileBytes+1))
		f.Close()
		if readErr != nil {
			return nil, true, c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: fmt.Sprintf("read %q: %s", fh.Filename, readErr),
			})
		}
		if int64(len(data)) > maxUploadFileBytes {
			return nil, true, c.JSON(http.StatusRequestEntityTooLarge, ErrorResponse{
				Error: fmt.Sprintf("%q too large: at most 20 MiB per file", fh.Filename),
			})
		}
		files[fh.Filename] = data
	}
	return files, false, nil
}
