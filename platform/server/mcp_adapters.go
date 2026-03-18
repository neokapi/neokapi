package server

import (
	"github.com/neokapi/neokapi/sievepen"
	"github.com/neokapi/neokapi/termbase"
)

// tmResolverAdapter bridges workspaceStores → MCPServer.TMResolver.
type tmResolverAdapter struct {
	ws *workspaceStores
}

func (a *tmResolverAdapter) GetTM(workspaceID string) (sievepen.TMStore, error) {
	return a.ws.getTM(workspaceID)
}

// tbResolverAdapter bridges workspaceStores → MCPServer.TBResolver.
type tbResolverAdapter struct {
	ws *workspaceStores
}

func (a *tbResolverAdapter) GetTB(workspaceID string) (termbase.TBStore, error) {
	return a.ws.getTB(workspaceID)
}
