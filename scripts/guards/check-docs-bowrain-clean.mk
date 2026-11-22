GUARDS += check-docs-bowrain-clean
GUARD_LANES.check-docs-bowrain-clean := pre-push ci
check-docs-bowrain-clean: ## Guard: the neokapi docs stay bowrain-clean
	@./scripts/check-docs-bowrain-clean.sh
