package addin

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// errorResponse is the JSON error envelope returned by the add-in API.
type errorResponse struct {
	Error string `json:"error"`
}

// RegisterRoutes mounts the add-in REST API on the given Echo group. These are
// the endpoints the Microsoft 365 Office task pane (and any browser client)
// calls; mount the group behind the server's auth + CORS middleware:
//
//	addinSvc.RegisterRoutes(v1.Group("/addin", authMiddleware))
//
// Routes (relative to the group):
//
//	POST /check      → CheckResult
//	POST /terms      → TermsResult
//	POST /translate  → TranslateResult
func (s *Service) RegisterRoutes(g *echo.Group) {
	g.POST("/check", s.handleCheck)
	g.POST("/terms", s.handleTerms)
	g.POST("/translate", s.handleTranslate)
}

func (s *Service) handleCheck(c echo.Context) error {
	var req CheckRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Error: "invalid request: " + err.Error()})
	}
	res, err := s.Check(c.Request().Context(), req)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Error: err.Error()})
	}
	return c.JSON(http.StatusOK, res)
}

func (s *Service) handleTerms(c echo.Context) error {
	var req TermsRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Error: "invalid request: " + err.Error()})
	}
	res, err := s.Terms(c.Request().Context(), req)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Error: err.Error()})
	}
	return c.JSON(http.StatusOK, res)
}

func (s *Service) handleTranslate(c echo.Context) error {
	var req TranslateRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Error: "invalid request: " + err.Error()})
	}
	res, err := s.Translate(c.Request().Context(), req)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Error: err.Error()})
	}
	return c.JSON(http.StatusOK, res)
}
