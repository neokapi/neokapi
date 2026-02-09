// Package service provides the shared business logic layer used by
// both the REST/gRPC server and the CLI.
package service

import (
	"github.com/gokapi/gokapi/core/connector"
	"github.com/gokapi/gokapi/core/registry"
	"github.com/gokapi/gokapi/core/store"
)

// Services aggregates all service dependencies.
type Services struct {
	Project   *ProjectService
	Connector *ConnectorService
	Flow      *FlowService
}

// NewServices creates the full service layer.
func NewServices(
	contentStore store.ContentStore,
	connectorReg *connector.Registry,
	formatReg *registry.FormatRegistry,
	toolReg *registry.ToolRegistry,
) *Services {
	ps := NewProjectService(contentStore)
	cs := NewConnectorService(contentStore, connectorReg)
	fs := NewFlowService(contentStore, formatReg, toolReg)

	return &Services{
		Project:   ps,
		Connector: cs,
		Flow:      fs,
	}
}
