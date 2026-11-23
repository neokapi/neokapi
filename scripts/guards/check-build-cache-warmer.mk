GUARDS += check-build-cache-warmer
GUARD_LANES.check-build-cache-warmer := pre-push ci
check-build-cache-warmer: ## Guard: the build cache warmer passes its self-test
	@./scripts/ci-warm-build-cache.sh --self-test
