GUARDS += check-package-licenses
GUARD_LANES.check-package-licenses := lint ci
check-package-licenses: ## Guard: every non-private package.json declares a license and ships its LICENSE
	@./scripts/check-package-licenses.sh
