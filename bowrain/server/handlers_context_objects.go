package server

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
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
	// contextOutsideReachCode answers a push carrying a governing operation
	// at a point outside the pusher's coordinates, with status 403.
	contextOutsideReachCode = apiclient.ContextOutsideReachCode
	// contextInvalidSegmentCode answers a push with an object under log/
	// that is not a segment of operations of the project, with status 400.
	contextInvalidSegmentCode = apiclient.ContextInvalidSegmentCode
	// contextOperationConflictCode answers a push carrying an operation id
	// the project holds with other content, with status 409.
	contextOperationConflictCode = apiclient.ContextOperationConflictCode
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
	if refused, err := s.acceptContextBatch(c, cos, batch); refused || err != nil {
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

// acceptContextBatch reads the segments of a push before any of it is
// written, and binds the project's context to the project key they name on
// the push that first names one. It refuses the whole push, writing nothing
// and binding nothing, when:
//
//   - an object under log/ is not a segment of operations of one project
//     (400 invalid_segment);
//   - the segments name two keys, or a key other than the bound one (409
//     project_key_mismatch), or a key derived from one checkout (422
//     checkout_key);
//   - an operation carries the id of one the project holds with other
//     content (409 operation_conflict);
//   - a governing operation lies outside the pusher's reach (403
//     outside_reach, see checkReach).
//
// refused reports that the answer has been written.
func (s *Server) acceptContextBatch(c echo.Context, cos platstore.ContextObjectStore, batch []platstore.ContextObject) (refused bool, err error) {
	ctx := c.Request().Context()
	projectID := c.Param("id")
	held, err := cos.DigestContextObjects(ctx, projectID, workspace.RemoteLogDir)
	if err != nil {
		return true, serverErr(c, err)
	}
	// A segment the project holds already is not read again: the same bytes
	// change nothing, and other bytes are refused as the store refuses them.
	var segments []platstore.ContextObject
	for _, obj := range batch {
		if !strings.HasPrefix(obj.Name, workspace.RemoteLogDir) {
			continue
		}
		digest, ok := held[obj.Name]
		if !ok {
			segments = append(segments, obj)
			continue
		}
		if digest != contextObjectDigest(obj.Data) {
			return true, c.JSON(http.StatusConflict, ErrorResponse{
				Error: "the project already holds that context object with other bytes", Details: obj.Name, Code: contextObjectExistsCode,
			})
		}
	}
	var keys []workspace.ProjectKey
	for _, obj := range segments {
		named := segmentKeys(obj.Data)
		if len(named) == 0 {
			return true, c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: "the segment holds no operations of a project", Details: obj.Name, Code: contextInvalidSegmentCode,
			})
		}
		for _, key := range named {
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
			Message: checkoutKeyRemedy,
			Details: string(key),
			Code:    contextCheckoutKeyCode,
		})
	}
	bound, err := cos.ContextKey(ctx, projectID)
	if errors.Is(err, platstore.ErrNoContextProject) {
		return true, c.JSON(http.StatusNotFound, ErrorResponse{Error: "project not found", Details: projectID, Code: contextObjectNoProjectCode})
	}
	if err != nil {
		return true, serverErr(c, err)
	}
	if bound != "" && bound != string(key) {
		return true, keyMismatch(c, workspace.ProjectKey(bound), key)
	}

	var pushed []workspace.Op
	for _, obj := range segments {
		ops, err := workspace.DecodeSegment(obj.Data, key)
		if err != nil {
			return true, c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: fmt.Sprintf("the segment does not read as operations of project key %s: %v", key, err), Details: obj.Name, Code: contextInvalidSegmentCode,
			})
		}
		pushed = append(pushed, ops...)
	}

	var log []workspace.Op
	if bound != "" {
		if _, log, err = s.contextLogs.project(projectID).read(ctx, cos, projectID); err != nil {
			return true, serverErr(c, err)
		}
	}
	byID := make(map[string]workspace.Op, len(log)+len(pushed))
	for _, op := range log {
		byID[op.ID] = op
	}
	var fresh []workspace.Op
	for _, op := range pushed {
		if have, ok := byID[op.ID]; ok {
			if !sameOperation(have, op) {
				return true, c.JSON(http.StatusConflict, ErrorResponse{
					Error:   fmt.Sprintf("operation %s is held with other content", workspace.ShortOpID(op.ID)),
					Details: op.ID, Code: contextOperationConflictCode,
				})
			}
			continue
		}
		byID[op.ID] = op
		fresh = append(fresh, op)
	}

	member := ledgerMemberOf(c)
	if !member.Permissions.Has(governingPermissions) || !member.Reach.Unconstrained() {
		for _, op := range fresh {
			if err := checkReach(ctx, member, op, byID); err != nil {
				return true, c.JSON(http.StatusForbidden, ErrorResponse{
					Error: err.Error(), Details: op.ID, Code: contextOutsideReachCode,
				})
			}
		}
	}

	if bound == "" {
		now, err := cos.BindContextKey(ctx, projectID, string(key))
		if errors.Is(err, platstore.ErrNoContextProject) {
			return true, c.JSON(http.StatusNotFound, ErrorResponse{Error: "project not found", Details: projectID, Code: contextObjectNoProjectCode})
		}
		if err != nil {
			return true, serverErr(c, err)
		}
		if now != string(key) {
			return true, keyMismatch(c, workspace.ProjectKey(now), key)
		}
	}
	return false, nil
}

// sameOperation reports whether two readings of one operation id say the
// same thing.
func sameOperation(a, b workspace.Op) bool {
	if a.ID != b.ID || a.Kind != b.Kind || a.Project != b.Project || a.Address != b.Address ||
		a.Subject != b.Subject || !a.At.Equal(b.At) {
		return false
	}
	return compactJSON(a.Payload) == compactJSON(b.Payload)
}

// compactJSON renders a payload without insignificant whitespace, and as it
// stands when it is not JSON.
func compactJSON(raw []byte) string {
	var buf bytes.Buffer
	if json.Compact(&buf, raw) != nil {
		return string(raw)
	}
	return buf.String()
}

// keyMismatch answers a push naming a key other than the bound one.
func keyMismatch(c echo.Context, bound, pushed workspace.ProjectKey) error {
	return c.JSON(http.StatusConflict, ErrorResponse{
		Error:   keyMismatchRefusal(bound, pushed),
		Details: string(pushed),
		Code:    contextKeyMismatchCode,
	})
}

// keyMismatchRefusal says why a push naming another project key is refused.
func keyMismatchRefusal(bound, pushed workspace.ProjectKey) string {
	return fmt.Sprintf("this project's context belongs to project key %s, and the push carries project key %s", bound, pushed)
}

// checkoutKeyRefusal says why a push naming a checkout's key is refused.
func checkoutKeyRefusal(key workspace.ProjectKey) string {
	return fmt.Sprintf("the push carries project key %s, which belongs to one checkout because kapi.yaml states neither id: nor name:", key)
}

// checkoutKeyRemedy is what the person pushing a checkout's key does next.
const checkoutKeyRemedy = "Name the project in kapi.yaml, then push again."

// contextObjectDigest is the hex SHA256 digest the store keeps for an
// object's bytes.
func contextObjectDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
