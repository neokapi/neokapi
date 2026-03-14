package server

import (
	"net/http"

	"github.com/labstack/echo/v4"

	coreg "github.com/neokapi/neokapi/core/graph"
)

// HandleGetConceptHierarchy returns the concept hierarchy for a workspace,
// showing broader/narrower relationships and violation counts.
func (s *Server) HandleGetConceptHierarchy(c echo.Context) error {
	if s.GraphStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "graph store not configured"})
	}
	ctx := c.Request().Context()

	// Find all Concept nodes.
	nodes, err := s.GraphStore.FindNodes(ctx, "Concept", nil)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	type conceptNode struct {
		ID         string            `json:"id"`
		Label      string            `json:"label"`
		Properties map[string]string `json:"properties"`
		Children   int               `json:"children"`
		Parents    int               `json:"parents"`
	}

	var result []conceptNode
	for _, n := range nodes {
		children, _ := s.GraphStore.Neighbors(ctx, n.ID, coreg.Outgoing, coreg.LabelNarrower)
		parents, _ := s.GraphStore.Neighbors(ctx, n.ID, coreg.Outgoing, coreg.LabelBroader)
		result = append(result, conceptNode{
			ID:         n.ID,
			Label:      n.Label,
			Properties: n.Properties,
			Children:   len(children),
			Parents:    len(parents),
		})
	}

	return c.JSON(http.StatusOK, result)
}

// HandleGetGraphNeighbors returns neighbors for a node with optional label filtering.
func (s *Server) HandleGetGraphNeighbors(c echo.Context) error {
	if s.GraphStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "graph store not configured"})
	}
	ctx := c.Request().Context()
	nodeID := c.Param("nodeId")

	direction := coreg.Both
	switch c.QueryParam("direction") {
	case "outgoing":
		direction = coreg.Outgoing
	case "incoming":
		direction = coreg.Incoming
	}

	label := c.QueryParam("label")
	var labels []string
	if label != "" {
		labels = []string{label}
	}

	neighbors, err := s.GraphStore.Neighbors(ctx, nodeID, direction, labels...)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, neighbors)
}

// HandleGetGraphEdges returns edges for a node with optional direction filtering.
func (s *Server) HandleGetGraphEdges(c echo.Context) error {
	if s.GraphStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "graph store not configured"})
	}
	ctx := c.Request().Context()
	nodeID := c.Param("nodeId")

	direction := coreg.Both
	switch c.QueryParam("direction") {
	case "outgoing":
		direction = coreg.Outgoing
	case "incoming":
		direction = coreg.Incoming
	}

	edges, err := s.GraphStore.EdgesOf(ctx, nodeID, direction)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, edges)
}

// HandleGetShortestPath returns the shortest path between two nodes.
func (s *Server) HandleGetShortestPath(c echo.Context) error {
	if s.GraphStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "graph store not configured"})
	}
	ctx := c.Request().Context()
	from := c.QueryParam("from")
	to := c.QueryParam("to")
	if from == "" || to == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "from and to query params required"})
	}

	path, err := s.GraphStore.ShortestPath(ctx, from, to, 10)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, path)
}
