//go:build integration

package terms_test

import "testing"

// The import's rules hold on the store the server runs on.
func TestImportRows_Postgres(t *testing.T) {
	for _, sc := range importScenarios {
		t.Run(sc.name, func(t *testing.T) { sc.run(t, openTestPostgresTerms(t)) })
	}
}
