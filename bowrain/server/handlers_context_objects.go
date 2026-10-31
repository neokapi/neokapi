package server

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/workspace"
)

// Error codes the context object routes answer with, which a client remote
// maps to the workspace errors of the same meaning.
const (
	// contextObjectExistsCode answers a put of a name the project holds with
	// other bytes (workspace.ErrObjectExists), with status 409.
	contextObjectExistsCode = "object_exists"
	// contextObjectMissingCode answers a get of a name the project does not
	// hold (workspace.ErrNoObject), with status 404.
	contextObjectMissingCode = "no_object"
	// contextObjectInvalidCode answers a name or a directory outside the
	// context layout, or a blob whose name is not the address of its bytes,
	// with status 400.
	contextObjectInvalidCode = "invalid_name"
	// contextObjectNoProjectCode answers a push to a project id the store
	// does not hold, with status 404.
	contextObjectNoProjectCode = "no_project"
)

// contextObjectWritePermission gates a push of context objects. The objects
// carry the project's context ledger: terms writes and the keep and drop
// decisions of context review, which the server's change-set and concept
// routes gate on PermManageTerms.
const contextObjectWritePermission = platauth.PermManageTerms

// contextObjectList is the body of GET /:id/context/objects.
type contextObjectList struct {
	Names []string `json:"names"`
}

// contextObjectBatch is the body of POST /:id/context/objects. Data travels
// as base64, as encoding/json writes a byte slice.
type contextObjectBatch struct {
	Objects []contextObjectWire `json:"objects"`
}

type contextObjectWire struct {
	Name string `json:"name"`
	Data []byte `json:"data"`
}

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
