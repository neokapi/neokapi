package server

import (
	"context"

	"github.com/gokapi/gokapi/core/model"
)

// ServerKnownTermsLoader loads known terms from the server's workspace termbases.
type ServerKnownTermsLoader struct {
	wsStores *workspaceStores
	// workspaceSlug is needed because the termbase is workspace-scoped.
	workspaceSlug string
}

// LoadKnownTerms returns all term texts for the given locale from the workspace termbase.
func (l *ServerKnownTermsLoader) LoadKnownTerms(_ context.Context, _ string, locale string) ([]string, error) {
	tb := l.wsStores.getTB(l.workspaceSlug)

	concepts := tb.Concepts()
	seen := make(map[string]struct{})
	var terms []string

	for _, c := range concepts {
		for _, t := range c.Terms {
			if string(t.Locale) == locale || t.Locale == model.LocaleID(locale) {
				text := t.Text
				if _, ok := seen[text]; !ok {
					seen[text] = struct{}{}
					terms = append(terms, text)
				}
			}
		}
	}

	return terms, nil
}

// newKnownTermsLoader creates a ServerKnownTermsLoader for the given workspace.
func newKnownTermsLoader(ws *workspaceStores, wsSlug string) *ServerKnownTermsLoader {
	return &ServerKnownTermsLoader{wsStores: ws, workspaceSlug: wsSlug}
}
