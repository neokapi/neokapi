# `make audit-vite-alias` runs the self-test and then the audit of the
# installed tree, which needs `vp install`. This is the self-test alone.
GUARDS += check-vite-alias-audit
GUARD_LANES.check-vite-alias-audit := ci
check-vite-alias-audit: ## Guard: the vite alias audit fails on drift and passes on both core layouts
	@bash scripts/audit-vite-alias.sh --self-test
