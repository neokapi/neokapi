GUARDS += check-plugin-release-latest
GUARD_LANES.check-plugin-release-latest := lint ci
check-plugin-release-latest: ## Guard: no plugin release claims the repo's "latest"
	@./scripts/check-plugin-release-latest.sh
