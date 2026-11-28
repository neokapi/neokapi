GUARDS += check-build-cache-warmer
GUARD_LANES.check-build-cache-warmer := pre-push ci
check-build-cache-warmer: ## Guard: the build cache warmer and the hit report pass their self-tests
	@./scripts/ci-warm-build-cache.sh --self-test
	@python3 ./scripts/go-cache-hits.py --self-test
