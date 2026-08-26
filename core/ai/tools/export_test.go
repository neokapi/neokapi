package tools

import (
	"context"

	"github.com/neokapi/neokapi/core/model"
)

// ExportCacheFingerprint exposes the session cache key for tests in the
// external test package. It is the one piece of this tool that must be
// asserted from outside and cannot be observed through a prompt.
func ExportCacheFingerprint(t *AITranslateTool, ctx context.Context, b *model.Block) string {
	return t.cacheFingerprint(ctx, b)
}
