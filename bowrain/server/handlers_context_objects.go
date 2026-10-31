package server

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/labstack/echo/v4"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/workspace"
	apiclient "github.com/neokapi/neokapi/host/venue/client"
)

// Error codes the context object routes answer with. The client remote
// (host/venue/contextremote) maps them to the workspace errors of the same
// meaning, so both sides read them from the client package.
const (
	// contextObjectExistsCode answers a put of a name the project holds with
	// other bytes (workspace.ErrObjectExists), with status 409.
	contextObjectExistsCode = apiclient.ContextObjectExistsCode
	// contextObjectMissingCode answers a get of a name the project does not
	// hold (workspace.ErrNoObject), with status 404.
	contextObjectMissingCode = apiclient.ContextObjectMissingCode
	// contextObjectInvalidCode answers a name or a directory outside the
	// context layout, or a blob whose name is not the address of its bytes,
	// with status 400.
	contextObjectInvalidCode = apiclient.ContextObjectInvalidCode
	// contextObjectNoProjectCode answers a push to a project id the store
	// does not hold, with status 404.
	contextObjectNoProjectCode = apiclient.ContextObjectNoProjectCode
	// contextKeyMismatchCode answers a push whose segments name a project key
	// other than the one the project's context is bound to, or two keys,
	// with status 409.
	contextKeyMismatchCode = apiclient.ContextKeyMismatchCode
	// contextCheckoutKeyCode answers a push whose segments name a key derived
	// from one checkout (workspace.KeyForCheckout), with status 422.
	contextCheckoutKeyCode = apiclient.ContextCheckoutKeyCode
)

// requestBodyLimit bounds every request body the server reads. A push of
// context objects travels as base64 in JSON, so the client remote splits a
// push into batches that stay under it.
const requestBodyLimit = "50M"

// contextObjectWritePermission gates a push of context objects. The objects
// carry the project's context ledger: terms writes and the keep and drop
// decisions of context review, which the server's change-set and concept
// routes gate on PermManageTerms.
const contextObjectWritePermission = platauth.PermManageTerms

// The bodies of GET and POST /:id/context/objects, the shapes the client
// sends and reads.
type (
	contextObjectList  = apiclient.ContextObjectList
	contextObjectBatch = apiclient.ContextObjectBatch
)

// registerContextObjectRoutes adds the routes a project's context objects are
// read and pushed through, under the project group /:ws.
func (s *Server) registerContextObjectRoutes(g *echo.Group) {
	g.GET("/:id/context/objects", s.HandleListContextObjects)
	g.GET("/:id/context/objects/*", s.HandleGetContextObject)
	g.POST("/:id/context/objects", s.HandlePutContextObjects)
}

// contextObjects returns the store the routes read and write, and false when
// the content store holds no context objects.
func (s *Server) contextObjects() (platstore.ContextObjectStore, bool) {
	if s.ContentStore == nil {
		return nil, false
	}
	return platstore.ContextObjects(s.ContentStore)
}

// contextObjectsUnavailable answers a route whose content store holds no
// context objects.
func contextObjectsUnavailable(c echo.Context) error {
	return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "context objects not configured"})
}

// HandleListContextObjects answers GET /:ws/:id/context/objects?dir=, the
// names under one directory of the context layout (log/, blobs/,
// checkpoints/), sorted.
func (s *Server) HandleListContextObjects(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	dir := c.QueryParam("dir")
	switch dir {
	case workspace.RemoteLogDir, workspace.RemoteBlobDir, workspace.RemoteCheckpointsDir:
	default:
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "dir must be one of log/, blobs/, checkpoints/", Code: contextObjectInvalidCode,
		})
	}
	cos, ok := s.contextObjects()
	if !ok {
		return contextObjectsUnavailable(c)
	}
	names, err := cos.ListContextObjects(c.Request().Context(), c.Param("id"), dir)
	if err != nil {
		return serverErr(c, err)
	}
	return c.JSON(http.StatusOK, contextObjectList{Names: names})
}

// HandleGetContextObject answers GET /:ws/:id/context/objects/<name> with the
// object's bytes.
func (s *Server) HandleGetContextObject(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	name := c.Param("*")
	if !workspace.ValidObjectName(name) {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "not a name in the context layout", Details: name, Code: contextObjectInvalidCode,
		})
	}
	cos, ok := s.contextObjects()
	if !ok {
		return contextObjectsUnavailable(c)
	}
	obj, err := cos.GetContextObject(c.Request().Context(), c.Param("id"), name)
	if errors.Is(err, platstore.ErrNoContextObject) {
		return c.JSON(http.StatusNotFound, ErrorResponse{
			Error: "the project holds no context object by that name", Details: name, Code: contextObjectMissingCode,
		})
	}
	if err != nil {
		return serverErr(c, err)
	}
	return c.Blob(http.StatusOK, echo.MIMEOctetStream, obj.Data)
}

// HandlePutContextObjects answers POST /:ws/:id/context/objects. The batch is
// written in one transaction and every new object is stamped with the
// signed-in member. A name the project holds with other bytes refuses the
// whole batch with 409.
func (s *Server) HandlePutContextObjects(c echo.Context) error {
	if err := s.requirePermission(c, contextObjectWritePermission); err != nil {
		return err
	}
	var body contextObjectBatch
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
	}
	batch := make([]platstore.ContextObject, 0, len(body.Objects))
	for _, obj := range body.Objects {
		if !workspace.ValidObjectName(obj.Name) {
			return c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: "not a name in the context layout", Details: obj.Name, Code: contextObjectInvalidCode,
			})
		}
		if strings.HasPrefix(obj.Name, workspace.RemoteBlobDir) &&
			workspace.BlobObjectName(workspace.BlobAddress(obj.Data)) != obj.Name {
			return c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: "the blob's name is not the address of its bytes", Details: obj.Name, Code: contextObjectInvalidCode,
			})
		}
		batch = append(batch, platstore.ContextObject{Name: obj.Name, Data: obj.Data})
	}
	cos, ok := s.contextObjects()
	if !ok {
		return contextObjectsUnavailable(c)
	}
	ctx := c.Request().Context()
	projectID := c.Param("id")
	if _, err := s.ContentStore.GetProject(ctx, projectID); errors.Is(err, sql.ErrNoRows) {
		return c.JSON(http.StatusNotFound, ErrorResponse{
			Error: "project not found", Details: projectID, Code: contextObjectNoProjectCode,
		})
	} else if err != nil {
		return serverErr(c, err)
	}
	if refused, err := bindContextKey(c, cos, batch); refused || err != nil {
		return err
	}
	pushedBy, _ := c.Get("user_id").(string)
	err := cos.PutContextObjects(ctx, projectID, pushedBy, batch)
	if exists, ok := errors.AsType[*platstore.ContextObjectExistsError](err); ok {
		return c.JSON(http.StatusConflict, ErrorResponse{
			Error: "the project already holds that context object with other bytes", Details: exists.Name, Code: contextObjectExistsCode,
		})
	}
	if err != nil {
		return serverErr(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// bindContextKey binds the project's context to the project key the batch's
// segments name, on the push that first names one, and refuses a batch that
// names another key, two keys, or a key derived from one checkout. refused
// reports that the answer has been written. A batch whose objects name no key
// (blobs, checkpoints) binds nothing.
func bindContextKey(c echo.Context, cos platstore.ContextObjectStore, batch []platstore.ContextObject) (refused bool, err error) {
	var keys []workspace.ProjectKey
	for _, obj := range batch {
		if !strings.HasPrefix(obj.Name, workspace.RemoteLogDir) {
			continue
		}
		for _, key := range segmentKeys(obj.Data) {
			if !slices.Contains(keys, key) {
				keys = append(keys, key)
			}
		}
	}
	if len(keys) == 0 {
		return false, nil
	}
	if len(keys) > 1 {
		return true, c.JSON(http.StatusConflict, ErrorResponse{
			Error: fmt.Sprintf("the push carries the context of two projects, project key %s and project key %s", keys[0], keys[1]),
			Code:  contextKeyMismatchCode,
		})
	}
	key := keys[0]
	if workspace.IsCheckoutKey(key) {
		return true, c.JSON(http.StatusUnprocessableEntity, ErrorResponse{
			Error:   checkoutKeyRefusal(key),
			Details: string(key),
			Code:    contextCheckoutKeyCode,
		})
	}
	held, err := cos.BindContextKey(c.Request().Context(), c.Param("id"), string(key))
	if errors.Is(err, platstore.ErrNoContextProject) {
		return true, c.JSON(http.StatusNotFound, ErrorResponse{Error: "project not found", Code: contextObjectNoProjectCode})
	}
	if err != nil {
		return true, serverErr(c, err)
	}
	if held != string(key) {
		return true, c.JSON(http.StatusConflict, ErrorResponse{
			Error:   keyMismatchRefusal(workspace.ProjectKey(held), key),
			Details: string(key),
			Code:    contextKeyMismatchCode,
		})
	}
	return false, nil
}

// keyMismatchRefusal says why a push naming another project key is refused.
func keyMismatchRefusal(bound, pushed workspace.ProjectKey) string {
	return fmt.Sprintf("this project's context belongs to project key %s, and the push carries project key %s", bound, pushed)
}

// checkoutKeyRefusal says why a push naming a checkout's key is refused.
func checkoutKeyRefusal(key workspace.ProjectKey) string {
	return fmt.Sprintf("the push carries project key %s, which belongs to one checkout because kapi.yaml states neither id: nor name:. Name the project in kapi.yaml, then push again", key)
}
