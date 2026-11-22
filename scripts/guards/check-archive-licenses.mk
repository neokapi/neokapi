GUARDS += check-archive-licenses
GUARD_LANES.check-archive-licenses := lint ci
check-archive-licenses: ## Guard: every release archive ships the license text of the work inside it
	@./scripts/check-archive-licenses.sh
