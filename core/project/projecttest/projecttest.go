// Package projecttest provides test support for core/project, in the spirit
// of net/http/httptest: helpers that tests need but that don't belong on the
// production API surface.
package projecttest

import "github.com/neokapi/neokapi/core/project/internal/extregistry"

// ResetExtensions clears the process-wide extension registry
// (project.RegisterExtension state) so a test that registers extensions
// starts from a clean slate. Call it in setup and defer it for cleanup.
// Test-only: production code registers extensions once, at init, and never
// unregisters.
func ResetExtensions() {
	extregistry.Reset()
}
