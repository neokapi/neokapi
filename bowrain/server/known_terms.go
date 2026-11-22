package server

import (
	"context"
	"fmt"

	"github.com/neokapi/neokapi/core/model"
)

// ServerKnownTermsLoader loads known terms from the server's workspace terms stores.
type ServerKnownTermsLoader struct {
	wsStores *workspaceStores
	// workspaceSlug is needed because the terms store is workspace-scoped.
	workspaceSlug string
}

// LoadKnownTerms returns the texts of the terms in the locale that apply in
// the project.
func (l *ServerKnownTermsLoader) LoadKnownTerms(ctx context.Context, projectID string, locale string) ([]string, error) {
	tb, err := l.wsStores.getProjectTerms(ctx, l.workspaceSlug, projectID)
	if err != nil {
		return nil, fmt.Errorf("init terms: %w", err)
	}

	concepts, err := tb.Concepts(ctx)
	if err != nil {
		return nil, fmt.Errorf("list concepts: %w", err)
	}
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
