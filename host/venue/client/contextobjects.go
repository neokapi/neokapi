package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// The context object routes, /api/v1/:ws/:id/context/objects, hold the
// objects a project's context is shared through, in the core/workspace
// layout (log/, blobs/, checkpoints/). The server and this client both read
// the wire shapes and error codes below.

// Error codes the context object routes answer with, in the `code` field of
// the error body.
const (
	// ContextObjectExistsCode answers a put of a name the project holds with
	// other bytes, with status 409.
	ContextObjectExistsCode = "object_exists"
	// ContextObjectMissingCode answers a get of a name the project does not
	// hold, with status 404.
	ContextObjectMissingCode = "no_object"
	// ContextObjectInvalidCode answers a name or a directory outside the
	// context layout, with status 400.
	ContextObjectInvalidCode = "invalid_name"
)

// ContextObjectList is the body of GET /:ws/:id/context/objects?dir=.
type ContextObjectList struct {
	Names []string `json:"names"`
}

// ContextObjectBatch is the body of POST /:ws/:id/context/objects. Data
// travels as base64, as encoding/json writes a byte slice.
type ContextObjectBatch struct {
	Objects []ContextObjectWire `json:"objects"`
}

// ContextObjectWire is one object of a batch.
type ContextObjectWire struct {
	Name string `json:"name"`
	Data []byte `json:"data"`
}

// ContextObjectError is a refusal from a context object route: the status and
// the code and details of the error body.
type ContextObjectError struct {
	StatusCode int
	Code       string
	Message    string
	Details    string
}

func (e *ContextObjectError) Error() string {
	msg := e.Message
	if msg == "" {
		msg = http.StatusText(e.StatusCode)
	}
	if e.Details != "" {
		msg += ": " + e.Details
	}
	return fmt.Sprintf("context objects (HTTP %d): %s", e.StatusCode, msg)
}

// contextObjectsPath is the route prefix of the bound project's context
// objects. The routes are workspace-scoped and take no stream: a project has
// one context, whichever stream a push names.
func (c *BowrainClient) contextObjectsPath() (string, error) {
	if c.workspace == "" {
		return "", errors.New("context objects are workspace-scoped (use NewWorkspaceBowrainClient)")
	}
	return fmt.Sprintf("%s/api/v1/%s/%s/context/objects", c.baseURL, url.PathEscape(c.workspace), url.PathEscape(c.projectID)), nil
}

// ContextObjectsLocation is where the bound project's context objects are
// served: <server>/<workspace>/<project>.
func (c *BowrainClient) ContextObjectsLocation() string {
	return c.baseURL + "/" + c.workspace + "/" + c.projectID
}

// ListContextObjects returns the names under one directory of the context
// layout, sorted.
func (c *BowrainClient) ListContextObjects(ctx context.Context, dir string) ([]string, error) {
	base, err := c.contextObjectsPath()
	if err != nil {
		return nil, err
	}
	resp, err := c.contextObjectsDo(ctx, http.MethodGet, base+"?dir="+url.QueryEscape(dir), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, contextObjectError(resp)
	}
	var out ContextObjectList
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode the context object list: %w", err)
	}
	return out.Names, nil
}

// GetContextObject returns one object's bytes, reading at most limit+1 bytes
// so a caller can refuse an object larger than it takes.
func (c *BowrainClient) GetContextObject(ctx context.Context, name string, limit int64) ([]byte, error) {
	base, err := c.contextObjectsPath()
	if err != nil {
		return nil, err
	}
	resp, err := c.contextObjectsDo(ctx, http.MethodGet, base+"/"+name, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, contextObjectError(resp)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read context object %s: %w", name, err)
	}
	return data, nil
}

// PutContextObjects writes one batch of objects, which the server writes in
// one transaction.
func (c *BowrainClient) PutContextObjects(ctx context.Context, objs []ContextObjectWire) error {
	base, err := c.contextObjectsPath()
	if err != nil {
		return err
	}
	body, err := json.Marshal(ContextObjectBatch{Objects: objs})
	if err != nil {
		return fmt.Errorf("encode the context object batch: %w", err)
	}
	resp, err := c.contextObjectsDo(ctx, http.MethodPost, base, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return contextObjectError(resp)
	}
	return nil
}

// contextObjectsDo sends one request through the transport, which refreshes
// an expired access token and replays the request once.
func (c *BowrainClient) contextObjectsDo(ctx context.Context, method, target string, body []byte) (*http.Response, error) {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, rd)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.Do(req)
}

// contextObjectError reads a refusal's error body.
func contextObjectError(resp *http.Response) error {
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	var env struct {
		Error   string `json:"error"`
		Message string `json:"message"`
		Details string `json:"details"`
		Code    string `json:"code"`
	}
	e := &ContextObjectError{StatusCode: resp.StatusCode}
	if json.Unmarshal(raw, &env) == nil {
		e.Code, e.Details = env.Code, env.Details
		e.Message = env.Message
		if e.Message == "" {
			e.Message = env.Error
		}
	} else {
		e.Message = strings.TrimSpace(string(raw))
	}
	return e
}
