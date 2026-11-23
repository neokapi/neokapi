# The documentation palettes are computed from the brand tokens, so an edit
# to either side without a regeneration ships a site painted in the previous
# brand. Runs in well under a second and links nothing, so it stays ungated.
# CI runs it in reference-data-drift.yml.
GUARDS += check-docs-palette
GUARD_LANES.check-docs-palette := lint pre-push
check-docs-palette: ## Drift gate: fail if a committed docs palette is stale vs. its brand tokens
	node --no-warnings --experimental-strip-types packages/docs-palette/cli/gen-docs-palette.ts -check
