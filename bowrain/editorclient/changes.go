package editorclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/host/venue/client"
)

// maxChangeResultBytes bounds the result body ApplyChanges reads.
const maxChangeResultBytes = 32 << 20

// ApplyChanges sends a change set (kapi.change/v1, as JSON) to one stream of a
// project. It is how a person changes content on the server, decides on a
// translation and annotates a block: each operation names an item by its path,
// a block, an edition and the revision of the edition the sender read.
//
// The server answers every change set it considered with a change result
// (kapi.change-result/v1), a refused one included: its status is refused, each
// refused operation says why, and a stale refusal carries the edition as it
// now stands. ApplyChanges returns that result with a nil error whatever HTTP
// status carried it. An error means the change set got no result: the request
// did not reach the server, or the server answered with something other than
// a change result, which is a *client.StatusError.
//
// POST /api/v1/:ws/projects/:id/streams/:stream/changes
func (c *EditorClient) ApplyChanges(ctx context.Context, ws, projectID, stream string, set []byte) (*change.Result, error) {
	if stream == "" {
		stream = editorRef
	}
	path := fmt.Sprintf("/api/v1/%s/projects/%s/streams/%s/changes",
		url.PathEscape(ws), url.PathEscape(projectID), url.PathEscape(stream))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL()+path, bytes.NewReader(set))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request %s: %w", strings.TrimPrefix(path, "/"), err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxChangeResultBytes))
	if err != nil {
		return nil, fmt.Errorf("read %s response: %w", strings.TrimPrefix(path, "/"), err)
	}

	var res change.Result
	if json.Unmarshal(body, &res) != nil || res.Schema != change.ResultSchemaID {
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, client.NewStatusError(strings.TrimPrefix(path, "/"), resp.StatusCode, body)
		}
		return nil, fmt.Errorf("decode %s response: not a %s result", strings.TrimPrefix(path, "/"), change.ResultSchemaID)
	}
	return &res, nil
}
