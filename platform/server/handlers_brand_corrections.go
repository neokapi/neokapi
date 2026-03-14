package server

import (
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	corebrand "github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/id"
)

// BrandCorrectionRequest is the request body for creating a brand voice correction.
type BrandCorrectionRequest struct {
	ProfileID     string           `json:"profile_id"`
	BlockID       string           `json:"block_id"`
	Dimension     corebrand.Dimension `json:"dimension"`
	OriginalText  string           `json:"original_text"`
	CorrectedText string           `json:"corrected_text"`
	FindingID     string           `json:"finding_id,omitempty"`
}

// HandleCreateBrandVoiceCorrection records a user correction to a brand voice finding.
func (s *Server) HandleCreateBrandVoiceCorrection(c echo.Context) error {
	if s.BrandStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "brand voice not configured"})
	}

	var req BrandCorrectionRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if req.ProfileID == "" || req.OriginalText == "" || req.CorrectedText == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "profile_id, original_text, and corrected_text are required"})
	}

	userID, _ := c.Get("user_id").(string)

	correction := &corebrand.Correction{
		ID:            id.New(),
		ProfileID:     req.ProfileID,
		BlockID:       req.BlockID,
		Dimension:     req.Dimension,
		OriginalText:  req.OriginalText,
		CorrectedText: req.CorrectedText,
		FindingID:     req.FindingID,
		CorrectedBy:   userID,
		CorrectedAt:   time.Now().UTC(),
	}

	if err := s.BrandStore.StoreCorrection(c.Request().Context(), correction); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	return c.JSON(http.StatusCreated, correction)
}

// HandleGetSuggestedRules returns vocabulary rules suggested from repeated corrections.
func (s *Server) HandleGetSuggestedRules(c echo.Context) error {
	if s.BrandStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "brand voice not configured"})
	}

	wsID, _ := c.Get("workspace_id").(string)
	minCount := 3
	if mc := c.QueryParam("min_count"); mc != "" {
		if parsed, err := strconv.Atoi(mc); err == nil && parsed > 0 {
			minCount = parsed
		}
	}

	rules, err := s.BrandStore.GetSuggestedRules(c.Request().Context(), wsID, minCount)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	return c.JSON(http.StatusOK, rules)
}
