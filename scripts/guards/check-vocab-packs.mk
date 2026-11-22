# CI runs it in reference-data-drift.yml.
GUARDS += check-vocab-packs
GUARD_LANES.check-vocab-packs := lint
check-vocab-packs: ## Guard: the vocabulary packs have exactly two homes (Go embed + one TS copy) and they agree
	@node scripts/format-ops/check-vocab-packs.mjs
