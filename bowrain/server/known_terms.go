package server

import (
	"context"
	"fmt"

	"github.com/neokapi/neokapi/core/model"
)

// ServerKnownTermsLoader loads known terms from the server's workspace termbases.
type ServerKnownTermsLoader struct {
	wsStores *workspaceStores
	// workspaceSlug is needed because the termbase is workspace-scoped.
	workspaceSlug string
}

// LoadKnownTerms returns all term texts for the given locale from the workspace termbase.
func (l *ServerKnownTermsLoader) LoadKnownTerms(ctx context.Context, _ string, locale string) ([]string, error) {
	tb, err := l.wsStores.getTB(l.workspaceSlug)
	if err != nil {
		return nil, fmt.Errorf("init termbase: %w", err)
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
