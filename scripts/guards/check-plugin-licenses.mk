GUARDS += check-plugin-licenses
GUARD_LANES.check-plugin-licenses := lint ci
check-plugin-licenses: ## Guard: every plugin tarball ships the license text of the work inside it
	@./scripts/check-plugin-licenses.sh
