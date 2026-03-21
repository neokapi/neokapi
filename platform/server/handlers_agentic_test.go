package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func TestHandleListAgenticExecutions_NoMCP(t *testing.T) {
	s := &Server{}
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agentic/executions", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := s.HandleListAgenticExecutions(c)
	he, ok := err.(*echo.HTTPError)
	assert.True(t, ok)
	assert.Equal(t, http.StatusServiceUnavailable, he.Code)
}

func TestHandleListAgenticEvents_NoMCP(t *testing.T) {
	s := &Server{}
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agentic/events", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := s.HandleListAgenticEvents(c)
	he, ok := err.(*echo.HTTPError)
	assert.True(t, ok)
	assert.Equal(t, http.StatusServiceUnavailable, he.Code)
}

func TestHandleGetAgenticExecutionEvents_NoMCP(t *testing.T) {
	s := &Server{}
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agentic/executions/exec_123/events", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("exec_123")

	err := s.HandleGetAgenticExecutionEvents(c)
	he, ok := err.(*echo.HTTPError)
	assert.True(t, ok)
	assert.Equal(t, http.StatusServiceUnavailable, he.Code)
}

func TestHandleAgenticEventsWebSocket_NoMCP(t *testing.T) {
	s := &Server{}
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agentic/events/ws", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := s.HandleAgenticEventsWebSocket(c)
	he, ok := err.(*echo.HTTPError)
	assert.True(t, ok)
	assert.Equal(t, http.StatusServiceUnavailable, he.Code)
}
