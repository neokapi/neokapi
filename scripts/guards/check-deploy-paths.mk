GUARDS += check-deploy-paths
GUARD_LANES.check-deploy-paths := lint ci
check-deploy-paths: ## Guard: the deploy workflow triggers on every framework dir compiled into the deployed binaries
	@./scripts/check-deploy-paths.sh
